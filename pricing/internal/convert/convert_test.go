package convert

import (
	"context"
	"fmt"
	"testing"

	"github.com/stripe/stripe-go/v72"
	"kr.dev/diff"
	"tier.run/pricing/schema"
	"tier.run/values"
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
		ID:    "feature:upto:zero",
		Tiers: []schema.Tier{{Upto: 0}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRoundTripInterval(t *testing.T) {
	for in, want := range intervalLookup {
		t.Run(string(in), func(t *testing.T) {
			testRoundTrip(t, &schema.Feature{
				ID:       "feature:interval",
				Interval: in,
			}, &stripe.PriceParams{
				Recurring: &stripe.PriceRecurringParams{
					Interval: ptr(string(want)),
				},
			})
		})
	}

	t.Run("invalid", func(t *testing.T) {
		_, err := ToPriceParams(context.Background(), "plan:test@0", &schema.Feature{
			ID:       "feature:interval:invalid",
			Interval: "invalid",
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
func TestRoundTripAggregate(t *testing.T) {
	for agg, want := range aggregateLookup {
		t.Run(string(agg), func(t *testing.T) {
			testRoundTrip(t, &schema.Feature{
				ID:        "feature:aggregate",
				Aggregate: agg,
				Tiers:     []schema.Tier{{Upto: 1}},
			}, &stripe.PriceParams{
				BillingScheme: ptr("tiered"),
				TiersMode:     ptr("graduated"),
				Tiers: []*stripe.PriceTierParams{
					{UpToInf: ptr(true), FlatAmount: p64(0), UnitAmount: p64(0)},
				},
				Recurring: &stripe.PriceRecurringParams{
					UsageType: ptr("metered"),

					// What is being tested here.
					AggregateUsage: ptr(string(want)),
				},
			})
		})
	}

	t.Run("invalid", func(t *testing.T) {
		_, err := ToPriceParams(context.Background(), "plan:test@0", &schema.Feature{
			ID:        "feature:aggregate:invalid",
			Aggregate: "invalid",
			Tiers:     []schema.Tier{{Upto: 1}}, // aggregate should be ignored if no tiers
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRoundTripTiersMode(t *testing.T) {
	for _, mode := range []schema.Mode{"graduated", "volume"} {
		t.Run(string(mode), func(t *testing.T) {
			testRoundTrip(t, &schema.Feature{
				ID:    "feature:mode",
				Tiers: []schema.Tier{{Upto: 1}},

				Mode: mode,
			}, &stripe.PriceParams{
				BillingScheme: ptr("tiered"),
				Tiers: []*stripe.PriceTierParams{
					{UpToInf: ptr(true), FlatAmount: p64(0), UnitAmount: p64(0)},
				},
				Recurring: &stripe.PriceRecurringParams{
					AggregateUsage: ptr("sum"),
					UsageType:      ptr("metered"),
				},

				TiersMode: ptr(string(mode)),
			})
		})
	}

	t.Run("invalid", func(t *testing.T) {
		_, err := ToPriceParams(context.Background(), "plan:test@0", &schema.Feature{
			ID:        "feature:aggregate",
			Aggregate: "invalid",
			Tiers:     []schema.Tier{{Upto: 1}}, // aggregate should be ignored if no tiers
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRoundTripTiers(t *testing.T) {
	cases := []struct {
		tiers []schema.Tier
		want  []*stripe.PriceTierParams
	}{
		{
			tiers: []schema.Tier{{Upto: 1}},
			want:  []*stripe.PriceTierParams{{UpToInf: ptr(true), UnitAmount: p64(0), FlatAmount: p64(0)}},
		},
		{
			tiers: []schema.Tier{{Upto: 1, Price: 2, Base: 3}},
			want:  []*stripe.PriceTierParams{{UpToInf: ptr(true), UnitAmount: p64(2), FlatAmount: p64(3)}},
		},
		{
			// inf
			tiers: []schema.Tier{{Upto: schema.Inf}},
			want:  []*stripe.PriceTierParams{{UpToInf: ptr(true), UnitAmount: p64(0), FlatAmount: p64(0)}},
		},
		{
			tiers: []schema.Tier{
				{Upto: 1, Price: 2, Base: 3},
				{Upto: 2, Price: 2, Base: 3},
			},
			want: []*stripe.PriceTierParams{
				{UpTo: p64(1), UnitAmount: p64(2), FlatAmount: p64(3)},
				{UpToInf: ptr(true), UnitAmount: p64(2), FlatAmount: p64(3)},
			},
		},
		{
			// unsorted
			tiers: []schema.Tier{
				{Upto: 2, Price: 2, Base: 3},
				{Upto: 1, Price: 2, Base: 3},
			},
			want: []*stripe.PriceTierParams{
				{UpTo: p64(1), UnitAmount: p64(2), FlatAmount: p64(3)},
				{UpToInf: ptr(true), UnitAmount: p64(2), FlatAmount: p64(3)},
			},
		},
		{
			// unsorted + dup
			tiers: []schema.Tier{
				{Upto: 2, Price: 2, Base: 3},
				{Upto: 1, Price: 2, Base: 3},
				{Upto: 2, Price: 4, Base: 5},
			},
			want: []*stripe.PriceTierParams{
				{UpTo: p64(1), UnitAmount: p64(2), FlatAmount: p64(3)},
				{UpTo: p64(2), UnitAmount: p64(2), FlatAmount: p64(3)},
				{UpToInf: ptr(true), UnitAmount: p64(4), FlatAmount: p64(5)},
			},
		},
		{
			tiers: []schema.Tier{
				{Upto: 2, Price: 2, Base: 3},
				{Upto: 1, Price: 2, Base: 3},
				{Upto: 2, Price: 4, Base: 5},
			},
			want: []*stripe.PriceTierParams{
				{UpTo: p64(1), UnitAmount: p64(2), FlatAmount: p64(3)},
				{UpTo: p64(2), UnitAmount: p64(2), FlatAmount: p64(3)},
				{UpToInf: ptr(true), UnitAmount: p64(4), FlatAmount: p64(5)},
			},
		},
		{
			tiers: []schema.Tier{
				{Upto: 2, Price: 2, Base: 3},
				{Upto: 1, Price: 2, Base: 3},
				{Upto: 1, Price: 2, Base: 3},
			},
			want: []*stripe.PriceTierParams{
				{UpTo: p64(1), UnitAmount: p64(2), FlatAmount: p64(3)},
				{UpTo: p64(1), UnitAmount: p64(2), FlatAmount: p64(3)},
				{UpToInf: ptr(true), UnitAmount: p64(2), FlatAmount: p64(3)},
			},
		},
	}

	for _, tt := range cases {
		t.Run("", func(t *testing.T) {
			testRoundTrip(t,
				&schema.Feature{Tiers: tt.tiers},
				&stripe.PriceParams{
					BillingScheme: ptr("tiered"),
					TiersMode:     ptr("graduated"),
					Recurring: &stripe.PriceRecurringParams{
						AggregateUsage: ptr("sum"),
						UsageType:      ptr("metered"),
					},
					Tiers: tt.want,
				})
		})
	}
}

func TestDefaults(t *testing.T) {
	t.Run("without expand", func(t *testing.T) {
		fp := &schema.Feature{}
		_, err := ToPriceParams(context.Background(), "plan:test@0", fp)
		if err != nil {
			t.Fatal(err)
		}
		diff.Test(t, t.Errorf, fp, &schema.Feature{})
	})
	t.Run("with expand", func(t *testing.T) {
		fp := Expand(&schema.Feature{})
		_, err := ToPriceParams(context.Background(), "plan:test@0", fp)
		if err != nil {
			t.Fatal(err)
		}
		diff.Test(t, t.Errorf, fp, &schema.Feature{
			Currency: "usd",
			Interval: "@monthly",
		})
	})
}

func TestRoundTripCurrency(t *testing.T) {
	for _, cur := range []string{
		"",
		"usd",
		"eur",
		"gbp",
		"jpy",
	} {
		testRoundTrip(t,
			&schema.Feature{Currency: cur},
			&stripe.PriceParams{Currency: ptr(values.Coalesce(cur, "usd"))})
	}
}

func testRoundTrip(t *testing.T, fp *schema.Feature, want *stripe.PriceParams) {
	t.Helper()

	values.MaybeSet(&want.Currency, ptr("usd"))
	values.MaybeSet(&want.BillingScheme, ptr("per_unit"))
	if len(fp.Tiers) == 0 {
		// only per_unit should set a UnitAmount
		values.MaybeSet(&want.UnitAmount, p64(0))
	}
	values.MaybeSet(&want.Recurring, &stripe.PriceRecurringParams{})
	values.MaybeSet(&want.Recurring.UsageType, ptr("licensed"))
	values.MaybeSet(&want.Recurring.Interval, ptr("month"))
	values.MaybeSet(&want.Nickname, ptr(fp.ID))

	fp = Expand(fp)
	got, err := ToPriceParams(context.Background(), "plan:test@0", fp)
	if err != nil {
		t.Fatal(err)
	}

	diff.Test(t, t.Errorf, got, want, diff.ZeroFields[stripe.PriceParams]("Params", "Product", "LookupKey"))

	// Round trip.
	gotFP, err := ToFeature(priceParamsToPrice("pr_123", got))
	if err != nil {
		t.Fatal(err)
	}

	wantFP := fp
	wantFP.ProviderID = "pr_123"
	wantFP.Plan = "plan:test@0"
	diff.Test(t, t.Errorf, gotFP, wantFP)
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

func coerce[D, S ~string](dst *D, src *S) {
	if src == nil {
		return
	}
	var zero D
	if dst == nil || *dst == zero {
		*dst = D(*src)
	}
}
