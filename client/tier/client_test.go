package tier

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/exp/slices"
	"kr.dev/diff"
	"tier.run/stripe"
)

func createAccount(t *testing.T, c *stripe.Client) string {
	t.Helper()
	var v struct {
		stripe.ID
	}
	var f stripe.Form
	f.Set("type", "standard")
	if err := c.Do(context.Background(), "POST", "/v1/accounts", f, &v); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Do(context.Background(), "DELETE", "/v1/accounts/"+v.ID.ProviderID(), stripe.Form{}, nil); err != nil {
			t.Error(err)
		}
	})
	return v.ID.ProviderID()
}

func newTestClient(t *testing.T) *Client {
	t.Helper()

	tc, err := stripe.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tc.APIKey, "_test_") {
		t.Fatal("expected test key")
	}
	return &Client{
		Stripe: tc.CloneAs(createAccount(t, tc)),
		Logf:   t.Logf,
	}
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

	t.Run("push", func(t *testing.T) {
		for _, f := range want {
			f := f
			t.Run(f.Name, func(t *testing.T) {
				t.Parallel()
				if err := tc.Push(ctx, f); err != nil {
					t.Fatal(err)
				}
			})
		}
	})

	t.Run("pull", func(t *testing.T) {
		got, err := tc.Pull(ctx, 0)
		if err != nil {
			t.Fatal(err)
		}

		slices.SortFunc(got, func(a, b Feature) bool {
			return a.Name < b.Name
		})

		diff.Test(t, t.Errorf, got, want,
			diff.ZeroFields[Feature]("ProviderID"))
	})

	t.Run("product title", func(t *testing.T) {
		f := want[0]
		var got struct {
			Name string
		}
		tc.Stripe.Do(ctx, "GET", "/v1/products/"+f.ID(), stripe.Form{}, &got)
		const want = "PlanTitle (yearly): FeatureTitle (theVersion)"
		if got.Name != want {
			t.Errorf("got %q, want %q", got.Name, want)
		}
	})
}

func TestAppendPhase(t *testing.T) {
	t.Parallel()

	fs := []Feature{
		{
			Name:      "feature:x",
			Plan:      "plan:test@0",
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
			Name:     "feature:y",
			Plan:     "plan:test@0",
			Interval: "@daily",
			Currency: "eur",
			Title:    "Test2",
			Base:     1000,
		},
	}

	ctx := context.Background()

	tc := newTestClient(t)
	t.Run("push", func(t *testing.T) {
		for _, f := range fs {
			f := f
			t.Run(f.Name, func(t *testing.T) {
				t.Parallel()
				if err := tc.Push(ctx, f); err != nil {
					t.Fatal(err)
				}
			})
		}
	})

	var f stripe.Form
	f.Set("email", "org@example.com")
	if err := tc.Stripe.Do(ctx, "POST", "/v1/customers", f, nil); err != nil {
		t.Fatalf("%#v", err)
	}

	ps := []Phase{
		{
			Plans: []string{"plan:test@0"},
		},
	}

	if err := tc.Schedule(ctx, "org@example.com", ps, Never); err != nil {
		t.Fatal(err)
	}
}
