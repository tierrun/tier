package control

import (
	"context"
	"errors"
	"sync"
	"testing"

	"golang.org/x/exp/slices"
	"kr.dev/diff"
	"tier.run/refs"
	"tier.run/stripe"
	"tier.run/stripe/stroke"
)

func newTestClient(t *testing.T) *Client {
	t.Helper()
	t.Parallel()

	sc := stroke.Client(t)
	if sc.Live() {
		t.Fatal("expected test key")
	}
	if sc.KeyPrefix == "" {
		t.Fatal("KeyPrefix must be set")
	}
	return &Client{
		Stripe: stroke.WithAccount(t, sc),
		Logf:   t.Logf,
	}
}

func TestRoundTrip(t *testing.T) {
	tc := newTestClient(t)
	ctx := context.Background()

	want := []Feature{
		{
			FeaturePlan: refs.MustParseFeaturePlan("feature:decimal@fractionalBase"),
			Interval:    "@daily",
			Currency:    "eur",
			Base:        0.1,
		},
		{
			FeaturePlan: refs.MustParseFeaturePlan("feature:test@plan:free@3"),
			Interval:    "@daily",
			Currency:    "eur",
			Title:       "Test2",
			Base:        1000,
		},
		{
			FeaturePlan: refs.MustParseFeaturePlan("feature:test@plan:free@theVersion"),
			PlanTitle:   "PlanTitle",
			Interval:    "@yearly",
			Currency:    "usd",
			Title:       "FeatureTitle",
			Mode:        "volume",
			Aggregate:   "perpetual",
			Tiers: []Tier{
				{Upto: 1, Price: 100, Base: 1},
				{Upto: 2, Price: 200, Base: 2},
				{Upto: 3, Price: 300, Base: 3},
			},
		},
	}

	if !slices.IsSortedFunc(want, func(a, b Feature) bool {
		return a.Less(b.FeaturePlan)
	}) {
		t.Fatal("want must be sorted")
	}

	if err := tc.Push(ctx, want, pushLogger(t)); err != nil {
		t.Fatal(err)
	}

	got, err := tc.Pull(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}

	slices.SortFunc(got, func(a, b Feature) bool {
		// TODO(bmizerany): embed struct on
		return a.Less(b.FeaturePlan)
	})

	diff.Test(t, t.Errorf, got, want,
		diff.ZeroFields[Feature]("ProviderID"))

	t.Run("product title", func(t *testing.T) {
		var got struct {
			Name string
		}
		if err := tc.Stripe.Do(ctx, "GET", "/v1/products/tier__feature-test-plan-free-theVersion", stripe.Form{}, &got); err != nil {
			t.Fatal(err)
		}
		const want = "PlanTitle - FeatureTitle"
		if got.Name != want {
			t.Errorf("got %q, want %q", got.Name, want)
		}
	})
}

func TestPushPlanInvalidDecimal(t *testing.T) {
	tc := newTestClient(t) // TODO(bmizerany): use a client without creating an account
	ctx := context.Background()

	fs := []Feature{
		{
			FeaturePlan: refs.MustParseFeaturePlan("feature:test@plan:free@theVersion"),
			Interval:    "@yearly",
			Currency:    "usd",
			Mode:        "volume",
			Aggregate:   "perpetual",
			Tiers: []Tier{
				// 13 decimals is greater than the max allowed by Stripe: 12 decimals
				{Upto: 1, Price: 0.1111111111111, Base: 1},
			},
		},
	}
	got := tc.Push(ctx, fs, pushLogWith(t, t.Logf))
	if !errors.Is(got, ErrInvalidPrice) {
		t.Fatalf("got %v, want ErrInvalidDecimal", got)
	}
}

func TestPushPlanImmutability(t *testing.T) {
	tc := newTestClient(t)
	ctx := context.Background()

	pushes := []struct {
		featurePlan string
		err         error
	}{
		{"feature:x@plan:test@0", nil},
		{"feature:y@plan:test@0", ErrPlanExists},
		{"feature:x@plan:test@1", nil},
		{"feature:y@plan:test@1", ErrPlanExists},
	}

	for i, push := range pushes {
		fs := []Feature{{
			FeaturePlan: refs.MustParseFeaturePlan(push.featurePlan),
			Interval:    "@daily",
			Currency:    "eur",
		}}
		var errs []error
		cb := func(_ Feature, err error) {
			errs = append(errs, err)
		}
		if err := tc.Push(ctx, fs, cb); !errors.Is(err, push.err) {
			t.Errorf("[%d]: got %v, want %v", i, err, push.err)
		}
		if push.err != nil {
			if len(errs) == 0 {
				t.Fatalf("[%d]: expected errors", i)
			}
			if !errors.Is(errs[0], push.err) {
				t.Errorf("[%d]: got %v, want %v", i, errs[0], push.err)
			}
		}
	}
}

func TestPushAllFeaturesLoggedOnFailure(t *testing.T) {
	tc := newTestClient(t)
	ctx := context.Background()

	fs := []Feature{
		{
			FeaturePlan: refs.MustParseFeaturePlan("feature:x@plan:test@0"),
			Interval:    "@daily",
			Currency:    "eur",
		},
		{
			FeaturePlan: refs.MustParseFeaturePlan("feature:y@plan:test@0"),
			Interval:    "@daily",
			Currency:    "eur",
		},
	}

	check := func(want []error) {
		t.Helper()
		var wantErr bool
		for _, err := range want {
			if err != nil {
				wantErr = true
			}
		}

		var mu sync.Mutex
		var got []error
		if err := tc.Push(ctx, fs, func(_ Feature, err error) {
			mu.Lock()
			defer mu.Unlock()
			got = append(got, err)
		}); err == nil && wantErr {
			t.Fatal("expected error")
		}
		diff.Test(t, t.Errorf, got, want)
	}

	check([]error{nil, nil})
	check([]error{ErrPlanExists, ErrPlanExists})
}

func pushLogger(t *testing.T) func(f Feature, err error) {
	t.Helper()
	return pushLogWith(t, t.Fatalf)
}

func pushLogWith(t *testing.T, fatalf func(string, ...any)) func(f Feature, err error) {
	t.Helper()
	return func(f Feature, err error) {
		t.Helper()
		if err == nil {
			t.Logf("pushed %q", f.FeaturePlan)
		} else {
			fatalf("error pushing %q: %v", f.FeaturePlan, err)
		}
	}
}
