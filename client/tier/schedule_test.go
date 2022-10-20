package tier

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/kr/pretty"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"kr.dev/diff"
	"kr.dev/errorfmt"
)

// interesting times to be in
var (
	t0 = time.Date(2020, 1, 0, 0, 0, 0, 0, time.UTC)
	t1 = time.Date(2020, 2, 0, 0, 0, 0, 0, time.UTC)
	t2 = time.Date(2020, 3, 0, 0, 0, 0, 0, time.UTC)
)

var ignoreProviderIDs = diff.OptionList(
	diff.ZeroFields[Feature]("ProviderID"),
	diff.ZeroFields[Org]("ProviderID"),
)

func TestSchedule(t *testing.T) {
	ciOnly(t)

	c := newTestClient(t)
	ctx := context.Background()

	var model []Feature
	plan := func(name string, ff []Feature) []Feature {
		for i := range ff {
			ff[i].Plan = name
		}
		model = append(model, ff...)
		return ff
	}

	planFree := plan("plan:free@0", []Feature{{
		Name:     "feature:x",
		Interval: "@monthly",
		Currency: "usd",
	}})

	planPro := plan("plan:pro@0", []Feature{{
		Name:     "feature:x",
		Interval: "@monthly",
		Base:     100,
		Currency: "usd",
	}})

	c.Push(ctx, model, pushLogger(t))

	sub := func(org string, fs []Feature) {
		t.Helper()
		t.Logf("subscribing %s to %# v", org, pretty.Formatter(fs))
		if err := c.SubscribeTo(ctx, org, fs); err != nil {
			t.Fatalf("%# v", pretty.Formatter(err))
		}
	}

	check := func(org string, want []Phase) {
		t.Helper()
		got, err := c.LookupPhases(ctx, org)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("got phases %# v", pretty.Formatter(got))
		diff.Test(t, t.Errorf, got, want, ignoreProviderIDs)
	}

	clock := c.setClock(t, t0)
	sub("org:example", planFree)
	check("org:example", []Phase{{
		Org:       "org:example",
		Current:   true,
		Effective: t0,
		Features:  planFree,
	}})

	clock.Advance(t1)
	sub("org:example", planPro)
	check("org:example", []Phase{
		{
			Org:       "org:example",
			Current:   false,
			Effective: t0, // unchanged by advanced clock
			Features:  planFree,
		},
		{
			Org:       "org:example",
			Current:   true,
			Effective: t1, // unchanged by advanced clock
			Features:  planPro,
		},
	})

	// downgrade and check no new phases
	sub("org:example", planFree)
	check("org:example", []Phase{
		{
			Org:       "org:example",
			Current:   false,
			Effective: t0, // unchanged by advanced clock
			Features:  planFree,
		},
		{
			Org:       "org:example",
			Current:   true,
			Effective: t1, // unchanged by advanced clock
			Features:  planFree,
		},
	})
}

func TestLookupPhasesWithTiersRoundTrip(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	fs := []Feature{
		{
			Plan:      "plan:test@0",
			Name:      "feature:10",
			Interval:  "@daily",
			Currency:  "usd",
			Tiers:     []Tier{{Upto: 10}},
			Mode:      "graduated",
			Aggregate: "sum",
		},
		{
			Name:      "feature:inf",
			Plan:      "plan:test@0",
			Interval:  "@daily",
			Currency:  "usd",
			Tiers:     []Tier{{}},
			Mode:      "graduated",
			Aggregate: "sum",
		},
		{
			Name:     "feature:lic",
			Plan:     "plan:test@0",
			Interval: "@daily",
			Currency: "usd",
		},
	}

	c.setClock(t, t0)
	c.Push(ctx, fs, pushLogger(t))
	if err := c.SubscribeTo(ctx, "org:example", fs); err != nil {
		t.Fatal(err)
	}

	got, err := c.LookupPhases(ctx, "org:example")
	if err != nil {
		t.Fatal(err)
	}

	want := []Phase{{
		Org:       "org:example",
		Effective: t0,
		Current:   true,
		Features:  fs,
	}}

	diff.Test(t, t.Errorf, got, want, ignoreProviderIDs)
}

func TestSubscribeToPlan(t *testing.T) {
	fs := []Feature{{
		Plan:     "plan:pro@0",
		Name:     "feature:x",
		Interval: "@monthly",
		Base:     100,
		Currency: "usd",
	}, {
		Plan:     "plan:pro@0",
		Name:     "feature:y",
		Interval: "@monthly",
		Base:     1000,
		Currency: "usd",
	}}

	ctx := context.Background()
	tc := newTestClient(t)
	tc.Push(ctx, fs, pushLogger(t))
	tc.setClock(t, t0)

	if err := tc.SubscribeToPlan(ctx, "org:example", "plan:pro@0"); err != nil {
		t.Fatal(err)
	}

	got, err := tc.LookupPhases(ctx, "org:example")
	if err != nil {
		t.Fatal(err)
	}
	want := []Phase{{
		Org:       "org:example",
		Current:   true,
		Effective: t0,
		Features:  fs,
	}}

	diff.Test(t, t.Errorf, got, want, ignoreProviderIDs)
}

func TestDedupCustomer(t *testing.T) {
	fs := []Feature{{
		Name:     "feature:x",
		Plan:     "plan:test@0",
		Interval: "@daily",
		Currency: "usd",
	}}

	tc := newTestClient(t)
	ctx := context.Background()
	tc.Push(ctx, fs, pushLogger(t))

	var g errgroup.Group
	for i := 0; i < 3; i++ {
		g.Go(func() error {
			return tc.SubscribeTo(ctx, "org:example", fs)
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}

	got, err := tc.ListOrgs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []Org{{ID: "org:example"}}
	diff.Test(t, t.Errorf, got, want, ignoreProviderIDs)
}

func TestLookupPhases(t *testing.T) {
	fs := []Feature{{
		Name:     "feature:x",
		Plan:     "plan:test@0",
		Interval: "@daily",
		Currency: "usd",
	}}

	tc := newTestClient(t)
	ctx := context.Background()
	tc.Push(ctx, fs, pushLogger(t))

	tc.setClock(t, t0)

	if err := tc.SubscribeTo(ctx, "org:example", fs); err != nil {
		t.Fatal(err)
	}

	got, err := tc.LookupPhases(ctx, "org:example")
	if err != nil {
		t.Fatal(err)
	}
	want := []Phase{{
		Org:       "org:example",
		Current:   true,
		Effective: t0,
		Features:  fs,
	}}

	diff.Test(t, t.Errorf, got, want, ignoreProviderIDs)
}

func TestReportUsage(t *testing.T) {
	fs := []Feature{
		{
			Plan:      "plan:test@0",
			Name:      "feature:10",
			Interval:  "@monthly",
			Currency:  "usd",
			Tiers:     []Tier{{Upto: 10}},
			Mode:      "graduated",
			Aggregate: "sum",
		},
		{
			Name:      "feature:inf",
			Plan:      "plan:test@0",
			Interval:  "@monthly",
			Currency:  "usd",
			Tiers:     []Tier{{Upto: Inf}},
			Mode:      "graduated",
			Aggregate: "sum",
		},
		{
			Name:     "feature:lic",
			Plan:     "plan:test@0",
			Interval: "@monthly",
			Currency: "usd",
		},
	}

	tc := newTestClient(t)
	ctx := context.Background()
	tc.Push(ctx, fs, pushLogger(t))
	tc.setClock(t, t0)

	if err := tc.SubscribeTo(ctx, "org:example", fs); err != nil {
		t.Fatal(err)
	}

	g, groupCtx := errgroup.WithContext(ctx)
	report := func(feature string, n int) {
		g.Go(func() (err error) {
			defer errorfmt.Handlef("%s: %w", feature, &err)
			return tc.ReportUsage(groupCtx, "org:example", feature, Report{
				N:  n,
				At: t0,
			})
		})
	}

	report("feature:10", 3)
	report("feature:inf", 9)
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}

	got, err := tc.LookupLimits(ctx, "org:example")
	if err != nil {
		t.Fatal(err)
	}

	slices.SortFunc(got, func(a, b Usage) bool {
		return a.Feature < b.Feature
	})

	want := []Usage{
		{Feature: "feature:10", Start: t0, End: endOfStripeMonth(t0), Used: 3, Limit: 10},
		{Feature: "feature:inf", Start: t0, End: endOfStripeMonth(t0), Used: 9, Limit: Inf},
		{Feature: "feature:lic", Start: t1, End: t2, Used: 1, Limit: Inf},
	}

	diff.Test(t, t.Errorf, got, want)
}

func TestReportUsageFeatureNotFound(t *testing.T) {
	tc := newTestClient(t)
	ctx := context.Background()

	fs := []Feature{{
		Name:      "feature:inf",
		Plan:      "plan:test@0",
		Interval:  "@monthly",
		Currency:  "usd",
		Tiers:     []Tier{{Upto: Inf}},
		Mode:      "graduated",
		Aggregate: "sum",
	}}

	tc.Push(ctx, fs, pushLogger(t))
	if err := tc.SubscribeTo(ctx, "org:example", fs); err != nil {
		t.Fatal(err)
	}
	got := tc.ReportUsage(ctx, "org:example", "feature:nope", Report{})
	if !errors.Is(got, ErrFeatureNotFound) {
		t.Fatalf("got %v, want %v", got, ErrFeatureNotFound)
	}
}

func ciOnly(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("not in CI; skipping long test")
	}
}

func endOfStripeMonth(t time.Time) time.Time {
	return t.AddDate(0, 1, 0).Truncate(time.Minute).Add(-5 * time.Minute)
}
