package tier

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kr/pretty"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"kr.dev/diff"
	"tier.run/stripe"
	"tier.run/stripe/stroke"
)

type clientTester struct {
	*Client
}

func newTestClient(t *testing.T) *clientTester {
	t.Helper()

	tc, err := stripe.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tc.APIKey, "_test_") {
		t.Fatal("expected test key")
	}
	return &clientTester{
		&Client{
			Stripe: stroke.WithAccount(t, context.Background(), tc),
			Logf:   t.Logf,
		},
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

func TestAppendPhase(t *testing.T) {
	t.Parallel()

	fs := []Feature{
		{
			Name:      "feature:x",
			Plan:      "plan:test@0",
			Interval:  "@daily",
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
			Currency: "usd",
			Title:    "Test2",
			Base:     1000,
		},
	}

	ctx := context.Background()

	tc := newTestClient(t)
	tc.Push(ctx, fs, pushLogger(t))

	var f stripe.Form
	f.Set("email", "org@example.com")
	f.Set("metadata", "tier.org", "org:example")
	if err := tc.Stripe.Do(ctx, "POST", "/v1/customers", f, nil); err != nil {
		t.Fatalf("%#v", err)
	}

	now := time.Now().Truncate(time.Second)
	want := []Phase{
		{
			Plans: []string{"plan:test@0"},
		},
		{
			Effective: now.AddDate(0, 0, 1),
			Plans:     []string{"plan:test@0"},
		},
	}

	if err := tc.Subscribe(ctx, "org:example", want); err != nil {
		t.Fatal(err)
	}

	got, err := tc.LookupPhases(ctx, "org:example")
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("got: %# v", pretty.Formatter(got))

	for i := range got {
		if closeEnough(got[i].Effective, now) {
			got[i].Effective = now
		}
	}
	for i := range want {
		if want[i].Effective.IsZero() {
			want[i].Effective = now
		}
	}

	diff.Test(t, t.Errorf, got, want)
}

func TestDedupCustomer(t *testing.T) {
	t.Parallel()

	tc := newTestClient(t)
	ctx := context.Background()
	tc.Push(ctx, []Feature{{
		Name:     "feature:x",
		Plan:     "plan:test@0",
		Interval: "@daily",
		Currency: "usd",
	}}, pushLogger(t))

	start := make(chan bool)
	var g errgroup.Group
	for i := 0; i < 3; i++ {
		g.Go(func() error {
			<-start
			if err := tc.Subscribe(ctx, "org:example", []Phase{
				{Plans: []string{"plan:test@0"}},
			}); err != nil {
				// TODO(bmizerany): Subscribe should backoff an
				// retry if idempotency_key_in_use is
				// encountered.
				var e *stripe.Error
				if errors.As(err, &e) && e.Code == "idempotency_key_in_use" {
					return nil
				}
				return err
			}
			return nil
		})
	}
	close(start)
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}

	type W struct {
		stripe.ID
		Metadata map[string]string
	}
	got, err := stripe.Slurp[W](ctx, tc.Stripe, "GET", "/v1/customers", stripe.Form{})
	if err != nil {
		t.Fatal(err)
	}
	want := []W{{
		ID: "cus_test",
		Metadata: map[string]string{
			"tier.org": "org:example",
		},
	}}
	diff.Test(t, t.Errorf, got, want, diff.ZeroFields[W]("ID"))
}

func closeEnough(a, b time.Time) bool {
	return a.Sub(b).Abs().Minutes() < 3
}

func pushLogger(t *testing.T) func(f Feature, err error) {
	t.Helper()
	return func(f Feature, err error) {
		t.Helper()
		t.Logf("pushed %s: %v", f.Name, err)
		if err != nil {
			t.Fail()
		}
	}
}
