package convert

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/stripe/stripe-go/v72"
	"golang.org/x/exp/slices"
	"kr.dev/errorfmt"
	"tailscale.com/util/multierr"
	"tier.run/pricing/schema"
	"tier.run/values"
)

var (
	ErrMissingMetadata = fmt.Errorf("missing metadata")
)

func Expand(fp *schema.Feature) *schema.Feature {
	// TODO: use tailscale/cmd/cloner
	newFP := new(schema.Feature)
	*newFP = *fp
	values.MaybeSet(&newFP.Interval, "@monthly")
	values.MaybeSet(&newFP.Currency, "usd")
	values.MaybeSet(&newFP.Mode, "graduated")
	values.MaybeSet(&newFP.Aggregate, "sum")
	return newFP
}

var intervalLookup = map[schema.Interval]stripe.PriceRecurringInterval{
	"@daily":   stripe.PriceRecurringIntervalDay,
	"@weekly":  stripe.PriceRecurringIntervalWeek,
	"@monthly": stripe.PriceRecurringIntervalMonth,
	"@yearly":  stripe.PriceRecurringIntervalYear,
}

var aggregateLookup = map[schema.Aggregate]stripe.PriceRecurringAggregateUsage{
	"sum":       stripe.PriceRecurringAggregateUsageSum,
	"max":       stripe.PriceRecurringAggregateUsageMax,
	"last":      stripe.PriceRecurringAggregateUsageLastDuringPeriod,
	"perpetual": stripe.PriceRecurringAggregateUsageLastEver,
}

func ToPriceParams(ctx context.Context, planID string, v *schema.Feature) (*stripe.PriceParams, error) {
	interval := stripe.PriceRecurringIntervalMonth
	if v.Interval != "" {
		interval = intervalLookup[v.Interval]
		if interval == "" {
			return nil, fmt.Errorf("invalid interval %q", v.Interval)
		}
	}

	aggregate := stripe.PriceRecurringAggregateUsageSum
	if v.Aggregate != "" {
		aggregate = aggregateLookup[v.Aggregate]
		if aggregate == "" {
			return nil, fmt.Errorf("invalid aggregate %q", v.Aggregate)
		}
	}

	pp := &stripe.PriceParams{
		Params: stripe.Params{
			Context: ctx,
			Metadata: map[string]string{
				"tier.plan":    planID,
				"tier.feature": v.ID,
			},
		},

		Product:   ptr(MakeID(planID)),
		Currency:  ptr(v.Currency),
		LookupKey: ptr(MakeID(planID, v.ID)),
		Nickname:  ptr(v.ID),

		BillingScheme: ptr("per_unit"),
		UnitAmount:    ptr(v.Base),
		Recurring: &stripe.PriceRecurringParams{
			Interval:       ptr(string(interval)),
			UsageType:      ptr("licensed"),
			AggregateUsage: ptr(string(aggregate)),
		},
	}

	hasTiers := len(v.Tiers) > 0

	if hasTiers {
		if v.Base > 0 {
			return nil, fmt.Errorf("a non zero base is not supported for tiered pricing")
		}

		slices.SortFunc(v.Tiers, func(a, b schema.Tier) bool {
			return a.Upto < b.Upto
		})

		pp.UnitAmount = nil
		pp.BillingScheme = ptr("tiered")
		pp.Recurring.UsageType = ptr("metered")
		pp.Recurring.AggregateUsage = ptr(string(aggregate))
		pp.TiersMode = ptr(string(v.Mode))
	}

	for _, t := range v.Tiers {
		pt := &stripe.PriceTierParams{
			UnitAmount: ptr(t.Price),
			FlatAmount: ptr(t.Base),
		}

		switch t.Upto {
		case 0:
			return nil, fmt.Errorf("invalid tier %v; zero upto reserved for future use", t)
		case schema.Inf:
			pt.UpToInf = ptr(true)
		default:
			pt.UpTo = ptr(t.Upto)
		}

		pp.Tiers = append(pp.Tiers, pt)
	}

	if hasTiers {
		limit := v.Tiers[len(v.Tiers)-1].Upto
		if limit == schema.Inf {
			pp.Metadata["tier.limit"] = "inf"
		} else {
			pp.Metadata["tier.limit"] = strconv.FormatInt(limit, 10)
		}

		catch := pp.Tiers[len(v.Tiers)-1]
		catch.UpToInf = ptr(true)
		catch.UpTo = nil
	}

	return pp, nil
}

var intervalConvert = values.Invert(intervalLookup)

func fromPriceInterval(v stripe.PriceRecurringInterval) (schema.Interval, error) {
	in, ok := intervalConvert[v]
	if !ok {
		return "", fmt.Errorf("invalid interval %q", v)
	}
	return in, nil
}

var aggregateConvert = values.Invert(aggregateLookup)

func fromPriceAggregate(v stripe.PriceRecurringAggregateUsage) (schema.Aggregate, error) {
	in, ok := aggregateConvert[v]
	if !ok {
		return "", fmt.Errorf("invalid aggregate %q", v)
	}
	return in, nil
}

func ToFeature(p *stripe.Price) (*schema.Feature, error) {
	v := &schema.Feature{
		ProviderID: p.ID,
		Base:       p.UnitAmount,
		Mode:       schema.Mode(p.TiersMode),
		Currency:   string(p.Currency),
		Tiers:      nil,
	}

	var errs []error
	var err error
	v.Plan, err = getMeta(p, "tier.plan")
	if err != nil {
		errs = append(errs, err)
	}

	v.ID, err = getMeta(p, "tier.feature")
	if err != nil {
		errs = append(errs, err)
	}

	if p.Recurring != nil {
		s, err := fromPriceInterval(p.Recurring.Interval)
		if err != nil {
			errs = append(errs, err)
		}
		v.Interval = s

		if p.Recurring.UsageType == "metered" {
			aggregate, err := fromPriceAggregate(p.Recurring.AggregateUsage)
			if err != nil {
				errs = append(errs, err)
			}
			v.Aggregate = aggregate
		}
	}

	limit, err := getLimit(p)
	if err != nil {
		errs = append(errs, err)
	}

	for i, t := range p.Tiers {
		if i == len(p.Tiers)-1 {
			t.UpTo = limit
		}

		v.Tiers = append(v.Tiers, schema.Tier{
			Upto:  t.UpTo,
			Price: t.UnitAmount,
			Base:  t.FlatAmount,
		})
	}

	v.Err = multierr.New(errs...)

	return v, nil
}

func getMeta(p *stripe.Price, key string) (string, error) {
	v, ok := p.Metadata[key]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrMissingMetadata, key)
	}
	return v, nil
}

func getLimit(p *stripe.Price) (n int64, err error) {
	if len(p.Tiers) == 0 {
		return 0, nil
	}
	errorfmt.Handlef("error parsing tier.limit: %w", &err)
	v, err := getMeta(p, "tier.limit")
	if err != nil {
		return 0, err
	}
	if v == "inf" {
		return schema.Inf, nil
	}
	return strconv.ParseInt(v, 10, 64)
}

func ptr[T any](v T) *T {
	return &v
}

func p64(i int64) *int64 {
	return &i
}

func MakeID(ids ...string) string {
	id := []rune(strings.Join(ids, "__"))
	for i, r := range id {
		if !unicode.IsDigit(r) && !unicode.IsLetter(r) {
			id[i] = '_'
		}
	}
	return "tier_" + string(id)
}
