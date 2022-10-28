package tier

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"golang.org/x/sync/errgroup"
	"tier.run/refs"
	"tier.run/stripe"
	"tier.run/values"
)

// Errors
var (
	ErrFeatureExists     = errors.New("feature already exists")
	ErrFeatureNotFound   = errors.New("feature not found")
	ErrFeatureNotMetered = errors.New("feature is not metered")
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

func FeaturePlans(fs []Feature) []refs.FeaturePlan {
	ns := make([]refs.FeaturePlan, len(fs))
	for i, f := range fs {
		ns[i] = f.Name
	}
	return ns
}

type Feature struct {
	Name refs.FeaturePlan // the feature name prefixed with ("feature:")

	ProviderID string // identifier set by the billing engine provider
	PlanTitle  string // a human readable title for the plan
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

	// ReportID is the ID for reporting usage to the billing provider.
	ReportID string
}

// TODO(bmizerany): remove FQN and replace with simply adding the version to
// the Name.

// IsMetered reports if the feature is metered.
func (f *Feature) IsMetered() bool {
	// checking the mode is more reliable than checking the existence of
	// tiers becauase not all responses from stripe containing prices
	// include tiers, but they all include the mode, which is empty for
	// license prices.
	return f.Mode != ""
}

func (f *Feature) ID() string {
	return stripe.MakeID(f.Name.String())
}

func (f *Feature) Limit() int {
	if len(f.Tiers) == 0 {
		return Inf
	}
	return f.Tiers[len(f.Tiers)-1].Upto
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
	Clock  string

	cache memo
}

// Live reports if APIKey is set to a "live" key.
func (c *Client) Live() bool { return c.Stripe.Live() }

// Push pushes each feature in fs to Stripe as a product and price combination.
// A new price and product are created in Stripe if one does not already exist.
//
// Each call to push is subject to rate limiting via the clients shared rate
// limit.
//
// It returns the first error encountered if any.
func (c *Client) Push(ctx context.Context, fs []Feature, cb func(f Feature, err error)) error {
	var g errgroup.Group
	g.SetLimit(c.maxWorkers())
	for _, f := range fs {
		f := f
		g.Go(func() error {
			err := c.pushFeature(ctx, f)
			cb(f, err)
			return err
		})
	}
	return g.Wait()
}

func (c *Client) maxWorkers() int {
	if c.Stripe.Live() {
		return 50
	}
	return 20 // a little under the max concurrent requests in test mode
}

func (c *Client) pushFeature(ctx context.Context, f Feature) error {
	// https://stripe.com/docs/api/prices/create
	var data stripe.Form
	data.Set("metadata", "tier.plan_title", f.PlanTitle)
	data.Set("metadata", "tier.title", f.Title)
	data.Set("metadata", "tier.feature", f.Name)

	c.Logf("tier: pushing feature %q", f.ID())
	data.Set("lookup_key", f.ID())
	data.Set("product_data", "id", f.ID())

	// This will appear as the line item description in the Stripe dashboard
	// and customer invoices.
	data.Set("product_data", "name", fmt.Sprintf("%s - %s",
		values.Coalesce(f.PlanTitle, f.Name.String()),
		values.Coalesce(f.Title, f.Name.String()),
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

type stripePrice struct {
	stripe.ID
	LookupKey string `json:"lookup_key"`
	Metadata  struct {
		Plan      string           `json:"tier.plan"`
		PlanTitle string           `json:"tier.plan_title"`
		Feature   refs.FeaturePlan `json:"tier.feature"`
		Limit     string           `json:"tier.limit"`
		Title     string           `json:"tier.title"`
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

func stripePriceToFeature(p stripePrice) Feature {
	f := Feature{
		ProviderID: p.ProviderID(),
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
	return f
}

// Pull retrieves the feature from Stripe.
func (c *Client) Pull(ctx context.Context, limit int) ([]Feature, error) {
	// https://stripe.com/docs/api/prices/list
	var f stripe.Form
	f.Add("expand[]", "data.product")
	f.Add("expand[]", "data.tiers")
	prices, err := stripe.Slurp[stripePrice](ctx, c.Stripe, "GET", "/v1/prices", f)
	if err != nil {
		return nil, err
	}
	var fs []Feature
	for _, p := range prices {
		if p.Metadata.Feature.IsZero() {
			continue
		}
		fs = append(fs, stripePriceToFeature(p))
	}
	return fs, nil
}

// Expand parses each ref in refs and adds it to the result. If the ref is a
// plan ref, Expand will append all features in fs for that plan to the result.
// returns an error if any ref is invalid or not availabe in the
//
// The parameter fs is assumed to have no two features with the same FeaturePlan.
//
// It returns an error if any.
func Expand(fs []Feature, names ...string) ([]refs.FeaturePlan, error) {
	var out []refs.FeaturePlan

	for _, name := range names {
		fp, err := refs.ParseFeaturePlan(name)
		if err != nil {
			p, err := refs.ParsePlan(name)
			if err != nil {
				return nil, err
			}
			n := len(out)
			for _, f := range fs {
				if f.Name.Plan() == p {
					out = append(out, f.Name)
				}
			}
			if len(out) == n {
				return nil, fmt.Errorf("no features found for plan %q", p)
			}
		} else {
			out = append(out, fp)
		}
	}

	return out, nil
}

type Org struct {
	ProviderID string
	ID         string
	Email      string
}

// ListOrgs returns a list of all known customers in Stripe.
func (c *Client) ListOrgs(ctx context.Context) ([]Org, error) {
	// https://stripe.com/docs/api/customers/list
	var f stripe.Form
	f.Add("limit", 100)
	type T struct {
		stripe.ID
		Email    string
		Metadata struct {
			Org string `json:"tier.org"`
		}
	}
	customers, err := stripe.Slurp[T](ctx, c.Stripe, "GET", "/v1/customers", f)
	if err != nil {
		return nil, err
	}
	var cs []Org
	for _, c := range customers {
		cs = append(cs, Org{
			ProviderID: c.ProviderID(),
			ID:         c.Metadata.Org,
			Email:      c.Email,
		})
	}
	return cs, nil
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
