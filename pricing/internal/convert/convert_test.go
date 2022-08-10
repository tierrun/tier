package convert

import (
	"context"
	"fmt"
	"testing"

	"github.com/stripe/stripe-go/v72"
	"kr.dev/diff"
	"tier.run/pricing/schema"
)

func TestToStripePriceParamsMetadata(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		plan string
		fp   *schema.Feature
		want *stripe.PriceParams
	}{
		{
			plan: "plan:test@0",
			fp:   &schema.Feature{ID: "feature:a"},
			want: &stripe.PriceParams{
				Params: stripe.Params{
					Context: ctx,
					Metadata: map[string]string{
						"tier.plan":    "plan:test@0",
						"tier.feature": "feature:a",
					},
				},
				Product: ptr("tier_plan_test_0"),
			},
		},
		{
			plan: "plan:x@1",
			fp:   &schema.Feature{ID: "feature:b"},
			want: &stripe.PriceParams{
				Params: stripe.Params{
					Context: ctx,
					Metadata: map[string]string{
						"tier.plan":    "plan:x@1",
						"tier.feature": "feature:b",
					},
				},
				Product: ptr("tier_plan_x_1"),
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.fp.ID, func(t *testing.T) {
			fp := Expand(tt.fp)
			got, err := ToPriceParams(ctx, tt.plan, fp)
			if err != nil {
				t.Fatal(err)
			}
			diff.Test(t, t.Errorf, got, tt.want,
				diff.KeepFields[stripe.PriceParams]("Product", "Params"))
		})
	}
}

func TestToStripePriceParamsInvalidBaseWithTiered(t *testing.T) {
	_, err := ToPriceParams(context.Background(), "plan:test@0", &schema.Feature{
		ID:    "feature:tiered_with_base",
		Base:  100,
		Tiers: []schema.Tier{{Upto: 10}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestToStripePriceParamsTierWithZero(t *testing.T) {
	// TODO: delete this test when we support zero.
	_, err := ToPriceParams(context.Background(), "plan:test@0", &schema.Feature{
		ID:    "feature:tiered_with_base",
		Tiers: []schema.Tier{{Upto: 0}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestToStripePriceParams(t *testing.T) {
	defaultRecurring := &stripe.PriceRecurringParams{
		Interval:  ptr("month"),
		UsageType: ptr("licensed"),
	}

	defaultTieredRecurring := &stripe.PriceRecurringParams{
		AggregateUsage: ptr("sum"),
		Interval:       ptr("month"),
		UsageType:      ptr("metered"),
	}

	ctx := context.Background()
	cases := []struct {
		fp   *schema.Feature
		want *stripe.PriceParams
	}{
		{
			fp: &schema.Feature{ID: "feature:blank"},
			want: &stripe.PriceParams{
				BillingScheme: ptr("per_unit"),
				Currency:      ptr("usd"),
				Recurring:     defaultRecurring,
				UnitAmount:    p64(0),
			},
		},
		{
			fp: &schema.Feature{ID: "feature:base", Base: 10},
			want: &stripe.PriceParams{
				BillingScheme: ptr("per_unit"),
				Currency:      ptr("usd"),
				Recurring:     defaultRecurring,
				UnitAmount:    p64(10),
			},
		},
		{
			fp: &schema.Feature{
				ID:       "feature:interval",
				Interval: "@yearly",
			},
			want: &stripe.PriceParams{
				BillingScheme: ptr("per_unit"),
				Currency:      ptr("usd"),
				Recurring: &stripe.PriceRecurringParams{
					Interval:  ptr("year"),
					UsageType: ptr("licensed"),
				},
				UnitAmount: p64(0),
			},
		},
		{
			fp: &schema.Feature{ID: "feature:single:tiered", Tiers: []schema.Tier{
				{Upto: 10, Price: 00, Base: 00},
			}},
			want: &stripe.PriceParams{
				BillingScheme: ptr("tiered"),
				Currency:      ptr("usd"),
				Recurring:     defaultTieredRecurring,
				TiersMode:     ptr("graduated"),
				Tiers: []*stripe.PriceTierParams{
					{UpToInf: ptr(true), FlatAmount: p64(0), UnitAmount: p64(0)},
				},
			},
		},
		{
			fp: &schema.Feature{ID: "feature:multi:tiered:catch-all", Tiers: []schema.Tier{
				{Upto: 10, Base: 2},
				{Upto: 20, Base: 1},
			}},
			want: &stripe.PriceParams{
				BillingScheme: ptr("tiered"),
				Currency:      ptr("usd"),
				Recurring:     defaultTieredRecurring,
				TiersMode:     ptr("graduated"),
				Tiers: []*stripe.PriceTierParams{
					{UpTo: p64(10), FlatAmount: p64(2), UnitAmount: p64(0)},
					{UpToInf: ptr(true), FlatAmount: p64(1), UnitAmount: p64(0)},
				},
			},
		},
		{
			fp: &schema.Feature{
				ID:    "feature:tiered_volume",
				Mode:  "volume",
				Tiers: []schema.Tier{{Upto: 10}},
			},
			want: &stripe.PriceParams{
				BillingScheme: ptr("tiered"),
				Currency:      ptr("usd"),
				Recurring:     defaultTieredRecurring,
				TiersMode:     ptr("volume"),
				Tiers: []*stripe.PriceTierParams{
					{UpToInf: ptr(true), FlatAmount: p64(0), UnitAmount: p64(0)},
				},
			},
		},
		{
			fp: &schema.Feature{
				ID:        "feature:aggregate",
				Aggregate: "max",
				Tiers:     []schema.Tier{{Upto: 10}},
			},
			want: &stripe.PriceParams{
				BillingScheme: ptr("tiered"),
				Currency:      ptr("usd"),
				Recurring: &stripe.PriceRecurringParams{
					AggregateUsage: ptr("max"),
					Interval:       ptr("month"),
					UsageType:      ptr("metered"),
				},
				TiersMode: ptr("graduated"),
				Tiers: []*stripe.PriceTierParams{
					{UpToInf: ptr(true), FlatAmount: p64(0), UnitAmount: p64(0)},
				},
			},
		},
		{
			fp: &schema.Feature{
				ID:       "feature:currency",
				Currency: "eur",
			},
			want: &stripe.PriceParams{
				BillingScheme: ptr("per_unit"),
				Currency:      ptr("eur"),
				Recurring:     defaultRecurring,
				UnitAmount:    p64(0),
			},
		},
		{
			fp: &schema.Feature{
				ID:    "feature:tiered:upto:inf",
				Tiers: []schema.Tier{{Upto: schema.Inf}},
			},
			want: &stripe.PriceParams{
				BillingScheme: ptr("tiered"),
				Currency:      ptr("usd"),
				Recurring:     defaultTieredRecurring,
				TiersMode:     ptr("graduated"),
				Tiers: []*stripe.PriceTierParams{
					{UpToInf: ptr(true), FlatAmount: p64(0), UnitAmount: p64(0)},
				},
			},
		},
		{
			fp: &schema.Feature{
				ID:    "feature:tiered:non-zero:upto_base_and_price",
				Tiers: []schema.Tier{{Upto: 1, Base: 2, Price: 3}},
			},
			want: &stripe.PriceParams{
				BillingScheme: ptr("tiered"),
				Currency:      ptr("usd"),
				Recurring:     defaultTieredRecurring,
				TiersMode:     ptr("graduated"),
				Tiers: []*stripe.PriceTierParams{
					{UpToInf: ptr(true), FlatAmount: p64(2), UnitAmount: p64(3)},
				},
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.fp.ID, func(t *testing.T) {
			fp := Expand(tt.fp)
			const plan = "plan:test@0"
			got, err := ToPriceParams(ctx, plan, fp)
			if err != nil {
				t.Fatal(err)
			}

			want := tt.want
			want.Nickname = ptr(tt.fp.ID)
			want.LookupKey = ptr(MakeID(plan, tt.fp.ID))
			diff.Test(t, t.Errorf, got, tt.want,
				diff.ZeroFields[stripe.PriceParams]("Params", "Product"))
		})
	}
}

func TestToStripePriceParamsLookupKeys(t *testing.T) {
	cases := []struct {
		plan, feature string
		want          string // lookup key
	}{
		{"plan:foo@0", "feature:bar", "tier_plan_foo_0__feature_bar"},
		{"plan:zoo@0", "feature:baz", "tier_plan_zoo_0__feature_baz"},
	}

	for i, tt := range cases {
		pp, err := ToPriceParams(context.Background(), tt.plan, Expand(&schema.Feature{
			ID: tt.feature,
		}))
		if err != nil {
			t.Fatal(err)
		}
		if got := *pp.LookupKey; got != tt.want {
			t.Errorf("[%d]: LookupKey = %q, want %q", i, got, tt.want)
		}
	}
}

func TestToStripePriceParamsLimit(t *testing.T) {
	cases := []struct {
		tiers    []schema.Tier
		wantMeta string // metadata value
	}{
		{tiers: []schema.Tier{{Upto: 10}}, wantMeta: "10"},
		{tiers: []schema.Tier{{Upto: 10}, {Upto: 20}}, wantMeta: "20"},
		{tiers: []schema.Tier{{Upto: schema.Inf}, {Upto: 20}}, wantMeta: "inf"},
		{tiers: []schema.Tier{{Upto: schema.Inf}}, wantMeta: "inf"},
	}

	for i, tt := range cases {
		pp, err := ToPriceParams(context.Background(), "plan:test@0", Expand(&schema.Feature{
			ID:    fmt.Sprintf("feature:%d", i),
			Tiers: tt.tiers,
		}))
		if err != nil {
			t.Fatal(err)
		}
		if got := pp.Metadata["tier.limit"]; got != tt.wantMeta {
			t.Errorf("[%d]: tier.limit = %q, want %q", i, got, tt.wantMeta)
		}
	}
}

func TestToFeature(t *testing.T) {
	recurringLicensed := &stripe.PriceRecurring{
		Interval:  "month",
		UsageType: "licensed",
	}

	recurringMetered := &stripe.PriceRecurring{
		Interval:       "month",
		AggregateUsage: "sum",
		UsageType:      "metered",
	}

	cases := []struct {
		price *stripe.Price
		want  *schema.Feature
	}{
		{
			price: &stripe.Price{
				ID: "pr_123",
				Metadata: map[string]string{
					"tier.feature": "feature:test:providerID",
					"tier.plan":    "plan:test@0",
					"tier.limit":   "inf",
				},
				Recurring: &stripe.PriceRecurring{Interval: "day"},
			},
			want: &schema.Feature{
				ID:         "feature:test:providerID",
				ProviderID: "pr_123",
				Plan:       "plan:test@0",
				Interval:   "@daily",
			},
		},
		{
			price: &stripe.Price{
				Metadata: map[string]string{
					"tier.feature": "feature:test:interval",
					"tier.plan":    "plan:test@0",
					"tier.limit":   "inf",
				},
				Recurring: &stripe.PriceRecurring{Interval: "day"},
			},
			want: &schema.Feature{
				ID:       "feature:test:interval",
				Plan:     "plan:test@0",
				Interval: "@daily",
			},
		},
		{
			price: &stripe.Price{
				Metadata: map[string]string{
					"tier.feature": "feature:test:currency",
					"tier.plan":    "plan:test@0",
					"tier.limit":   "inf",
				},
				Currency:  "eur",
				Recurring: recurringLicensed,
			},
			want: &schema.Feature{
				ID:       "feature:test:currency",
				Plan:     "plan:test@0",
				Interval: "@monthly",
				Currency: "eur",
			},
		},
		{
			price: &stripe.Price{
				Metadata: map[string]string{
					"tier.feature": "feature:test:tiers",
					"tier.plan":    "plan:test@0",
					"tier.limit":   "inf",
				},
				Recurring: recurringMetered,
				TiersMode: "graduated",
				Tiers: []*stripe.PriceTier{
					{UpTo: 1, FlatAmount: 2, UnitAmount: 3},
					{UpTo: 0, FlatAmount: 4, UnitAmount: 5},
				},
			},
			want: &schema.Feature{
				ID:        "feature:test:tiers",
				Plan:      "plan:test@0",
				Interval:  "@monthly",
				Mode:      "graduated",
				Aggregate: "sum",
				Tiers: []schema.Tier{
					{Upto: 1, Base: 2, Price: 3},
					{Upto: schema.Inf, Base: 4, Price: 5},
				},
			},
		},
		{
			price: &stripe.Price{
				Metadata: map[string]string{
					"tier.feature": "feature:test:limit",
					"tier.plan":    "plan:test@0",
					"tier.limit":   "1",
				},
				Recurring: recurringMetered,
				TiersMode: "graduated",
				Tiers: []*stripe.PriceTier{
					{UpTo: 0, FlatAmount: 2, UnitAmount: 3},
				},
			},
			want: &schema.Feature{
				ID:        "feature:test:limit",
				Plan:      "plan:test@0",
				Mode:      "graduated",
				Interval:  "@monthly",
				Aggregate: "sum",
				Tiers: []schema.Tier{
					{Upto: 1, Base: 2, Price: 3},
				},
			},
		},
	}

	for _, tt := range cases {
		name := tt.price.Metadata["tier.feature"]
		t.Run(name, func(t *testing.T) {
			got, err := ToFeature(tt.price)
			if err != nil {
				t.Fatal(err)
			}
			diff.Test(t, t.Errorf, got, tt.want)

			// RoundTrip check
			pp, err := ToPriceParams(context.Background(), "plan:test@0", got)
			if err != nil {
				t.Fatal(err)
			}

			sp := priceParamsToPrice(tt.price.ID, pp)
			got, err = ToFeature(sp)
			if err != nil {
				t.Fatal(err)
			}
			diff.Test(t, t.Errorf, got, tt.want)
		})
	}
}

func priceParamsToPrice(stripePriceID string, pp *stripe.PriceParams) *stripe.Price {
	sp := new(stripe.Price)
	sp.ID = stripePriceID
	sp.Metadata = pp.Metadata
	coerce(&sp.Currency, pp.Currency)
	if pp.Recurring != nil {
		sp.Recurring = &stripe.PriceRecurring{}
		coerce(&sp.Recurring.Interval, pp.Recurring.Interval)
		coerce(&sp.Recurring.UsageType, pp.Recurring.UsageType)
		coerce(&sp.Recurring.AggregateUsage, pp.Recurring.AggregateUsage)
	}
	coerce(&sp.TiersMode, pp.TiersMode)
	for _, t := range pp.Tiers {
		sp.Tiers = append(sp.Tiers, &stripe.PriceTier{
			UpTo:              deref(t.UpTo),
			FlatAmount:        deref(t.FlatAmount),
			FlatAmountDecimal: deref(t.FlatAmountDecimal),
			UnitAmount:        deref(t.UnitAmount),
			UnitAmountDecimal: deref(t.UnitAmountDecimal),
		})
	}
	return sp
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func coerce[D ~string](dst *D, src *string) {
	if src == nil {
		return
	}
	var zero D
	if dst == nil || *dst == zero {
		*dst = D(*src)
	}
}
