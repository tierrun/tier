package control

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/golang/groupcache/singleflight"
	"golang.org/x/sync/errgroup"
	"tier.run/refs"
	"tier.run/stripe"
	"tier.run/types/tax"
	"tier.run/values"
)

// Errors
var (
	ErrFeatureExists     = errors.New("feature already exists")
	ErrFeatureNotFound   = errors.New("feature not found")
	ErrNoFeatures        = errors.New("no features")
	ErrFeatureNotMetered = errors.New("feature is not metered")
	ErrPlanExists        = errors.New("plan already exists")
	ErrInvalidEmail      = errors.New("invalid email")
	ErrTooManyItems      = errors.New("too many subscription items")
	ErrInvalidPrice      = errors.New("invalid price")
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
	if fs == nil {
		return nil // preserve nil
	}
	ns := make([]refs.FeaturePlan, len(fs))
	for i, f := range fs {
		ns[i] = f.FeaturePlan
	}
	return ns
}

type Feature struct {
	refs.FeaturePlan // the feature name prefixed with ("feature:")

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
	Base float64

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

	TransformDenominator int  // the denominator for transforming usage
	TransformRoundUp     bool // whether to round up transformed usage; otherwise round down

	Tax tax.Settings
}

// TODO(bmizerany): remove FQN and replace with simply adding the version to
// the Name.

// IsMetered reports if the feature is metered.
func (f *Feature) IsMetered() bool {
	// checking the aggregate is more reliable than checking the existence
	// of tiers because not all responses from stripe containing prices
	// include tiers, but they all include the mode, which is empty for
	// license prices.
	return f.Aggregate != ""
}

func (f *Feature) ID() string {
	return stripe.MakeID(f.String())
}

func (f *Feature) Limit() int {
	if len(f.Tiers) == 0 {
		return Inf
	}
	return f.Tiers[len(f.Tiers)-1].Upto
}

// Tier holds the pricing information for a single tier.
type Tier struct {
	Upto  int     // the upper limit of the tier
	Price float64 // the price of the tier
	Base  int     // the base price of the tier
}

type Client struct {
	Logf      func(format string, args ...any)
	Stripe    *stripe.Client
	KeySource string // the source of the API key

	cache memo
}

// Live reports if APIKey is set to a "live" key.
func (c *Client) Live() bool { return c.Stripe.Live() }

// PushReportFunc is called for each feature pushed to Stripe. Implementations
// must be safe to use accross goroutines.
type PushReportFunc func(Feature, error)

// Push pushes each feature in fs to Stripe as a product and price combination.
// A new price and product are created in Stripe if one does not already exist.
//
// All features intended to be in the same plan must all be pushed in a single
// call to Push. Any subsequent calls attempting to push a feature in a plan
// that has already been pushed, will result in ErrPlanExists, and no
// attempt to push any feature in fs will be made. This constraint keeps plan
// immutable.
//
// Each call to push is subject to rate limiting via the clients shared rate
// limit.
//
// It returns the first error encountered if any.
func (c *Client) Push(ctx context.Context, fs []Feature, cb PushReportFunc) error {
	plans := map[refs.Plan][]Feature{}
	for _, f := range fs {
		for _, t := range f.Tiers {
			// Check the price has less than or equal to 12 decimal
			// places as required by stripe.
			//
			// We do the pre-flight check here because we don't
			// want to push a sentinel product if we can't push the
			// prices; otherwise we'll have to delete the product
			// manaully, which leads to crummy UX.
			if countDecimals(t.Price) > 12 {
				err := fmt.Errorf("%w: %.13f; tier prices must not exceed 12 decimal places", ErrInvalidPrice, t.Price)
				cb(f, err)
				return err
			}
		}
		plans[f.Plan()] = append(plans[f.Plan()], f)
	}

	var fg singleflight.Group
	var mu sync.Mutex
	pushed := map[refs.Plan]error{}
	var g errgroup.Group
	g.SetLimit(c.maxWorkers())
	for p, fs := range plans {
		p, fs := p, fs
		for _, f := range fs {
			f := f
			g.Go(func() error {
				_, err := fg.Do(f.String(), func() (any, error) {
					mu.Lock()
					defer mu.Unlock()
					if err, ok := pushed[p]; ok {
						return nil, err
					}
					err := c.pushSentinelPlan(ctx, p)
					pushed[p] = err
					return nil, err
				})
				if err != nil {
					cb(f, err) // error out all features in the plan
					return err
				}

				pid, err := c.pushFeature(ctx, f)
				if err != nil {
					cb(f, err)
					return err
				}
				f.ProviderID = pid
				cb(f, nil)
				return nil
			})
		}
	}
	return g.Wait()
}

func (c *Client) pushSentinelPlan(ctx context.Context, p refs.Plan) error {
	if p.IsZero() {
		return nil
	}
	var data stripe.Form
	data.Set("id", stripe.MakeID(p.String()))
	data.Set("name", p)

	// prevent sentinel products from being visible or
	// usable in the dashboard
	data.Set("active", false)

	err := c.Stripe.Do(ctx, "POST", "/v1/products", data, nil)
	if isExists(err) {
		err = ErrPlanExists
	}
	return err
}

func (c *Client) maxWorkers() int {
	if c.Stripe.Live() {
		return 50
	}
	return 20 // a little under the max concurrent requests in test mode
}

func (c *Client) pushFeature(ctx context.Context, f Feature) (providerID string, err error) {
	// https://stripe.com/docs/api/prices/create
	var data stripe.Form
	data.Set("metadata", "tier.plan_title", f.PlanTitle)
	data.Set("metadata", "tier.title", f.Title)
	data.Set("metadata", "tier.feature", f.FeaturePlan)

	c.Logf("tier: pushing feature %q", f.ID())
	data.Set("lookup_key", f.ID())
	data.Set("product_data", "id", f.ID())

	// This will appear as the line item description in the Stripe dashboard
	// and customer invoices.
	data.Set("product_data", "name", fmt.Sprintf("%s - %s",
		values.Coalesce(f.PlanTitle, f.String()),
		values.Coalesce(f.Title, f.String()),
	))

	// secondary composite key in schedules:
	data.Set("currency", f.Currency)

	interval := intervalToStripe[f.Interval]
	if interval == "" {
		return "", fmt.Errorf("unknown interval: %q", f.Interval)
	}
	data.Set("recurring", "interval", interval)
	data.Set("recurring", "interval_count", 1) // TODO: support user-defined interval count

	if f.TransformDenominator != 0 {
		round := "down"
		if f.TransformRoundUp {
			round = "up"
		}
		data.Set("transform_quantity", "divide_by", f.TransformDenominator)
		data.Set("transform_quantity", "round", round)
	}

	numTiers := len(f.Tiers)
	switch {
	case numTiers == 0:
		data.Set("recurring", "usage_type", "licensed")
		data.Set("billing_scheme", "per_unit")
		data.Set("unit_amount_decimal", f.Base)
	case numTiers == 1 && f.Tiers[0].Base == 0:
		t := f.Tiers[0]
		data.Set("recurring", "usage_type", "metered")
		data.Set("billing_scheme", "per_unit")
		aggregate := aggregateToStripe[f.Aggregate]
		if aggregate == "" {
			return "", fmt.Errorf("unknown aggregate: %q", f.Aggregate)
		}
		data.Set("recurring", "aggregate_usage", aggregate)
		data.Set("unit_amount_decimal", t.Price)
		data.Set("metadata", "tier.limit", t.Upto)
	default:
		data.Set("recurring", "usage_type", "metered")
		data.Set("billing_scheme", "tiered")
		data.Set("tiers_mode", f.Mode)
		aggregate := aggregateToStripe[f.Aggregate]
		if aggregate == "" {
			return "", fmt.Errorf("unknown aggregate: %q", f.Aggregate)
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
			data.Set("tiers", i, "unit_amount_decimal", t.Price)
			data.Set("tiers", i, "flat_amount", t.Base)
		}
		data.Set("metadata", "tier.limit", limit)
	}

	if f.Tax.Included {
		data.Set("tax_behavior", "inclusive")
	} else {
		data.Set("tax_behavior", "exclusive")
	}

	// TODO(bmizerany): data.Set("active", ?)
	// TODO(bmizerany): data.Set("currency_options", "?")

	var v struct {
		ID string
	}
	err = c.Stripe.Do(ctx, "POST", "/v1/prices", data, &v)
	if isExists(err) {
		return "", ErrFeatureExists
	}
	return v.ID, err
}

type stripePrice struct {
	stripe.ID
	LookupKey string `json:"lookup_key"`
	Metadata  struct {
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
	BillingScheme string  `json:"billing_scheme"`
	TiersMode     string  `json:"tiers_mode"`
	UnitAmount    float64 `json:"unit_amount_decimal,string"`
	Tiers         []struct {
		Upto         int     `json:"up_to"`
		Price        float64 `json:"unit_amount"`
		PriceDecimal float64 `json:"unit_amount_decimal,string"`
		Base         int     `json:"flat_amount"`
	}
	Currency          string
	TransformQuantity struct {
		DivideBy int    `json:"divide_by"`
		Round    string `json:"round"`
	} `json:"transform_quantity"`
	TaxBehavior string `json:"tax_behavior"`
}

func stripePriceToFeature(p stripePrice) Feature {
	f := Feature{
		ProviderID:           p.ProviderID(),
		PlanTitle:            p.Metadata.PlanTitle,
		FeaturePlan:          p.Metadata.Feature,
		Title:                p.Metadata.Title,
		Currency:             p.Currency,
		Interval:             intervalFromStripe[p.Recurring.Interval],
		Mode:                 p.TiersMode,
		Aggregate:            aggregateFromStripe[p.Recurring.AggregateUsage],
		TransformDenominator: p.TransformQuantity.DivideBy,
		TransformRoundUp:     p.TransformQuantity.Round == "up",
		Tax:                  tax.Settings{Included: p.TaxBehavior == "inclusive"},
	}

	if len(p.Tiers) == 0 && p.Recurring.UsageType == "metered" {
		f.Tiers = append(f.Tiers, Tier{
			Upto:  parseLimit(p.Metadata.Limit),
			Price: p.UnitAmount,
		})
	} else {
		f.Base = p.UnitAmount
	}

	for i, t := range p.Tiers {
		f.Tiers = append(f.Tiers, Tier{
			Upto:  t.Upto,
			Price: values.Coalesce(t.PriceDecimal, t.Price),
			Base:  t.Base,
		})
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

func Expand(m []Feature, names ...string) ([]refs.FeaturePlan, error) {
	fs, err := ExpandPlans(m, names...)
	if err != nil {
		return nil, err
	}
	return FeaturePlans(fs), nil
}

// ExpandPlans parses each ref in refs and adds it to the result. If the ref is a
// plan ref, Expand will append all features in fs for that plan to the result.
// returns an error if any ref is invalid or not availabe in the
//
// The parameter fs is assumed to have no two features with the same FeaturePlan.
//
// It returns an error if any.
func ExpandPlans(fs []Feature, names ...string) ([]Feature, error) {
	var out []Feature
	for _, name := range names {
		fp, err := refs.ParseFeaturePlan(name)
		if err != nil {
			p, err := refs.ParsePlan(name)
			if err != nil {
				return nil, err
			}
			n := len(out)
			for _, f := range fs {
				if f.InPlan(p) {
					out = append(out, f)
				}
			}
			if len(out) == n {
				return nil, fmt.Errorf("%w found for plan %q", ErrNoFeatures, p)
			}
		} else {
			var found bool
			for _, f := range fs {
				if f.FeaturePlan == fp {
					found = true
					out = append(out, f)
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("%w found named %q", ErrNoFeatures, fp)
			}
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

func countDecimals(f float64) int {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	_, dec, _ := strings.Cut(s, ".")
	return len(dec)
}
