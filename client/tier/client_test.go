package tier

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/felixge/fgprof"
	"golang.org/x/exp/slices"
	"kr.dev/diff"
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

	tc := stroke.Client(t)
	if tc.Live() {
		t.Fatal("expected test key")
	}
	return &Client{
		Stripe: stroke.WithAccount(t, tc),
		Logf:   t.Logf,
	}
}

func (c *Client) setClock(t *testing.T, now time.Time) *stroke.Clock {
	clock := stroke.NewClock(t, c.Stripe, t.Name(), now)
	c.Clock = clock.ID()
	return clock
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	tc := newTestClient(t)
	ctx := context.Background()

	want := []Feature{
		{
			Plan:      "plan:free@theVersion",
			PlanTitle: "PlanTitle",
			Name:      "test00",
			Interval:  "@yearly",
			Currency:  "usd",
			Title:     "FeatureTitle",
			Mode:      "volume",
			Aggregate: "perpetual",
			Tiers: []Tier{
				{Upto: 1, Price: 100, Base: 1},
				{Upto: 2, Price: 200, Base: 2},
				{Upto: 3, Price: 300, Base: 3},
			},
		},
		{
			Plan:     "plan:free@03",
			Name:     "test01",
			Interval: "@daily",
			Currency: "eur",
			Title:    "Test2",
			Base:     1000,
		},
	}

	tc.Push(ctx, want, pushLogger(t))

	got, err := tc.Pull(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}

	slices.SortFunc(got, func(a, b Feature) bool {
		return a.Name < b.Name
	})

	diff.Test(t, t.Errorf, got, want,
		diff.ZeroFields[Feature]("ProviderID"))

	t.Run("product title", func(t *testing.T) {
		var got struct {
			Name string
		}
		if err := tc.Stripe.Do(ctx, "GET", "/v1/products/tier__test00__plan-free-theVersion", stripe.Form{}, &got); err != nil {
			t.Fatal(err)
		}
		const want = "PlanTitle - FeatureTitle"
		if got.Name != want {
			t.Errorf("got %q, want %q", got.Name, want)
		}
	})
}

func pushLogger(t *testing.T) func(f Feature, err error) {
	t.Helper()
	return func(f Feature, err error) {
		t.Helper()
		t.Logf("pushed %q: %v", f.Name, err)
		if err != nil {
			t.FailNow()
		}
	}
}
