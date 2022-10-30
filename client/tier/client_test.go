package tier

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/felixge/fgprof"
	"golang.org/x/exp/slices"
	"kr.dev/diff"
	"tier.run/refs"
	"tier.run/stripe"
	"tier.run/stripe/stroke"
)

func TestMain(m *testing.M) {
	f, err := os.Create("test.prof")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	stop := fgprof.Start(f, fgprof.FormatPprof)
	defer stop() // be paranoid
	code := m.Run()
	stop()
	os.Exit(code)
}

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

func (c *Client) setClock(t *testing.T, now time.Time) *stroke.Clock {
	clock := stroke.NewClock(t, c.Stripe, t.Name(), now)
	c.Clock = clock.ID()
	return clock
}

func TestRoundTrip(t *testing.T) {
	tc := newTestClient(t)
	ctx := context.Background()

	want := []Feature{
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

	tc.Push(ctx, want, pushLogger(t))

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

func TestPushPlanImmutability(t *testing.T) {
	tc := newTestClient(t)
	ctx := context.Background()

	tc.Push(ctx, []Feature{{
		FeaturePlan: mpf("feature:original@plan:test@0"),
		Interval:    "@daily",
		Currency:    "eur",
	}}, pushLogger(t))

	var got []error
	f := func(_ Feature, err error) {
		got = append(got, err)
	}

	tc.Push(ctx, []Feature{{
		FeaturePlan: mpf("feature:new@plan:test@0"),
		Interval:    "@daily",
		Currency:    "eur",
	}}, f)

	want := []error{errors.New("TODO Push error")}
	for i, err := range got {
		if !errors.Is(err, want[i]) {
			t.Errorf("got[%d] = %v, want %v", i, err, want[i])
		}
	}
}

func pushLogger(t *testing.T) func(f Feature, err error) {
	t.Helper()
	return func(f Feature, err error) {
		t.Helper()
		if err == nil {
			t.Logf("pushed %q", f.FeaturePlan)
		} else {
			t.Fatalf("error pushing %q: %v", f.FeaturePlan, err)
		}
	}
}
