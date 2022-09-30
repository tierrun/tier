package tier

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"tier.run/stripe"
	"tier.run/values"
)

// Errors
var (
	ErrFeatureExists = errors.New("feature already exists")
)

const Inf = 1<<63 - 1

var (
	intervalToStripe = map[string]string{
		"@daily":   "day",
		"@weekly":  "week",
		"@monthly": "month",
		"@yearly":  "year",
	}

	intervalFromStripe = values.Invert(intervalToStripe)
)

var (
	aggregateToStripe = map[string]string{
		"sum":       "sum",
		"max":       "max",
		"last":      "last_during_period",
		"perpetual": "last_ever",
	}

	aggregateFromStripe = values.Invert(aggregateToStripe)
)

// Feature holds identifying, pricing, and billing information for a feature.
type Feature struct {
	ProviderID string // identifier set by the billing engine provider
	Plan       string // the plan ID prefixed with ("plan:")
	PlanTitle  string // a human readable title for the plan
	Name       string // the feature name prefixed with ("feature:")
	Title      string // a human readable title for the feature

	// Interval specifies the billing interval for the feature.
	//
	// Known intervals are "@daily", "@weekly", "@monthly", and "@yearly".
	Interval string

	// Currency is the ISO 4217 currency code for the feature.
	//
	// Known currencies look like "usd", "eur", "gbp", "cad", "aud", "jpy", "chf",
	// etc. Please see your billing engine provider for a complete list.
	Currency string

	// Base is the base price for the feature. If Tiers is not empty, then Base
	// is ignored.
	Base int

	// Mode specifies the billing mode for use with Tiers.
	//
	// Known modes are "graduated" and "volume".
	Mode string

	// Aggregate specifies the usage aggregation method for use with Tiers.
	//
	// Known aggregates are "sum", "max", "last", and "perpetual".
	Aggregate string

	// Tiers optionally specifies the pricing tiers for this feature. If
	// empty, feature is billed at the beginning of each billing period at
	// the flat rate specified by Base. If non-empty, the feature is billed
	// at the end of each billing period based on usage, and at a price
	// determined by Tiers, Mode, and Aggregate.
	Tiers []Tier
}

func (f *Feature) ID() string {
	return makeID(f.Name, f.Plan)
}

func (f *Feature) Version() string {
	_, version, _ := strings.Cut(f.Plan, "@")
	return version
}

// Tier holds the pricing information for a single tier.
type Tier struct {
	Upto  int // the upper limit of the tier
	Price int // the price of the tier
	Base  int // the base price of the tier
}

type Client struct {
	Logf   func(format string, args ...any)
	Stripe *stripe.Client
}

// Live reports if APIKey is set to a "live" key.
func (c *Client) Live() bool { return c.Stripe.Live() }

// Push pushes f to Stripe as a product and price combination. A new price and
// product are created in Stripe if one does not already exist.
//
// Each call to push is subject to rate limiting via the clients shared rate
// limit.
func (c *Client) Push(ctx context.Context, f Feature) error {
	// https://stripe.com/docs/api/prices/create
	var data stripe.Form
	data.Set("metadata", "tier.plan", f.Plan)
	data.Set("metadata", "tier.plan_title", f.PlanTitle)
	data.Set("metadata", "tier.title", f.Title)
	data.Set("metadata", "tier.feature", f.Name)

	c.Logf("tier: pushing feature %q", f.ID())
	data.Set("lookup_key", f.ID())
	data.Set("product_data", "id", f.ID())

	// This will appear as the line item description in the Stripe dashboard
	// and customer invoices.
	data.Set("product_data", "name", fmt.Sprintf("%s - %s",
		values.Coalesce(f.PlanTitle, f.Plan),
		values.Coalesce(f.Title, f.Name),
	))

	// secondary composite key in schedules:
	data.Set("currency", f.Currency)

	interval := intervalToStripe[f.Interval]
	if interval == "" {
		return fmt.Errorf("unknown interval: %q", f.Interval)
	}
	data.Set("recurring", "interval", interval)
	data.Set("recurring", "interval_count", 1) // TODO: support user-defined interval count

	if len(f.Tiers) == 0 {
		data.Set("recurring", "usage_type", "licensed")
		data.Set("billing_scheme", "per_unit")
		data.Set("unit_amount", f.Base)
	} else {
		data.Set("recurring", "usage_type", "metered")
		data.Set("billing_scheme", "tiered")
		data.Set("tiers_mode", f.Mode)
		aggregate := aggregateToStripe[f.Aggregate]
		if aggregate == "" {
			return fmt.Errorf("unknown aggregate: %q", f.Aggregate)
		}
		data.Set("recurring", "aggregate_usage", aggregate)
		var limit int
		for i, t := range f.Tiers {
			if i == len(f.Tiers)-1 {
				data.Set("tiers", i, "up_to", "inf")
			} else {
				data.Set("tiers", i, "up_to", t.Upto)
			}
			if limit < t.Upto {
				limit = t.Upto
			}
			data.Set("tiers", i, "unit_amount", t.Price)
			data.Set("tiers", i, "flat_amount", t.Base)
		}
		data.Set("metadata", "tier.limit", limit)
	}

	// TODO(bmizerany): data.Set("active", ?)
	// TODO(bmizerany): data.Set("tax_behavior", "?")
	// TODO(bmizerany): data.Set("transform_quantity", "?")
	// TODO(bmizerany): data.Set("currency_options", "?")

	err := c.Stripe.Do(ctx, "POST", "/v1/prices", data, nil)
	if isExists(err) {
		return ErrFeatureExists
	}
	return err
}

// Pull retrieves the feature from Stripe.
func (c *Client) Pull(ctx context.Context, limit int) ([]Feature, error) {
	var f stripe.Form
	f.Add("expand[]", "data.product")
	f.Add("expand[]", "data.tiers")

	// https://stripe.com/docs/api/prices/list
	type T struct {
		stripe.ID
		Metadata struct {
			Plan      string `json:"tier.plan"`
			PlanTitle string `json:"tier.plan_title"`
			Feature   string `json:"tier.feature"`
			Limit     string `json:"tier.limit"`
			Title     string `json:"tier.title"`
		}
		Recurring struct {
			Interval       string
			IntervalCount  int    `json:"interval_count"`
			UsageType      string `json:"usage_type"`
			AggregateUsage string `json:"aggregate_usage"`
		}
		BillingScheme string `json:"billing_scheme"`
		TiersMode     string `json:"tiers_mode"`
		UnitAmount    int    `json:"unit_amount"`
		Tiers         []struct {
			Upto  int `json:"up_to"`
			Price int `json:"unit_amount"`
			Base  int `json:"flat_amount"`
		}
		Currency string
	}

	prices, err := stripe.Slurp[T](ctx, c.Stripe, "GET", "/v1/prices", f)
	if err != nil {
		return nil, err
	}

	var fs []Feature
	for _, p := range prices {
		if p.Metadata.Feature == "" {
			continue
		}
		f := Feature{
			ProviderID: p.ProviderID(),
			Plan:       p.Metadata.Plan,
			PlanTitle:  p.Metadata.PlanTitle,
			Name:       p.Metadata.Feature,
			Title:      p.Metadata.Title,
			Currency:   p.Currency,
			Interval:   intervalFromStripe[p.Recurring.Interval],
			Mode:       p.TiersMode,
			Aggregate:  aggregateFromStripe[p.Recurring.AggregateUsage],
			Base:       p.UnitAmount,
		}
		for i, t := range p.Tiers {
			f.Tiers = append(f.Tiers, Tier(t))
			if i == len(p.Tiers)-1 {
				f.Tiers[i].Upto = parseLimit(p.Metadata.Limit)
			}
		}
		fs = append(fs, f)
	}

	return fs, nil
}

func makeID(ids ...string) string {
	id := []rune(strings.Join(ids, "__"))
	for i, r := range id {
		if r != '_' && !unicode.IsDigit(r) && !unicode.IsLetter(r) {
			id[i] = '-'
		}
	}
	return "tier__" + string(id)
}

func parseLimit(s string) int {
	if s == "inf" || s == "" {
		return Inf
	}
	n, _ := strconv.Atoi(s)
	return n
}

func isExists(err error) bool {
	var e *stripe.Error
	return errors.As(err, &e) && e.Code == "resource_already_exists"
}
