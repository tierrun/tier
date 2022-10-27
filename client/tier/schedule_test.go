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
	"tier.run/refs"
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
	plan := func(name string, ff []Feature) []refs.FeaturePlan {
		model = append(model, ff...)
		var fps []refs.FeaturePlan
		for _, f := range ff {
			fps = append(fps, f.Name)
		}
		return fps
	}

	planFree := plan("plan:free@0", []Feature{{
		Name:     must(refs.ParseFeaturePlan("feature:x@plan:free@0")),
		Interval: "@monthly",
		Currency: "usd",
	}})

	planPro := plan("plan:pro@0", []Feature{{
		Name:     refs.MustParseFeaturePlan("feature:x@plan:pro@0"),
		Interval: "@monthly",
		Base:     100,
		Currency: "usd",
	}})

	c.Push(ctx, model, pushLogger(t))

	sub := func(org string, fs []refs.FeaturePlan) {
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
		Plans:     plans("plan:free@0"),
	}})

	clock.Advance(t1)
	sub("org:example", planPro)
	check("org:example", []Phase{
		{
			Org:       "org:example",
			Current:   false,
			Effective: t0, // unchanged by advanced clock
			Features:  planFree,
			Plans:     plans("plan:free@0"),
		},
		{
			Org:       "org:example",
			Current:   true,
			Effective: t1, // unchanged by advanced clock
			Features:  planPro,
			Plans:     plans("plan:pro@0"),
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
			Plans:     plans("plan:free@0"),
		},
		{
			Org:       "org:example",
			Current:   true,
			Effective: t1, // unchanged by advanced clock
			Features:  planFree,
			Plans:     plans("plan:free@0"),
		},
	})
}

func TestLookupPhasesWithTiersRoundTrip(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	fs := []Feature{
		{
			// TODO(bmizerany): G: check/test plan name formats
			Name:      featurePlan("feature:10@plan:test@0"),
			Interval:  "@daily",
			Currency:  "usd",
			Tiers:     []Tier{{Upto: 10}},
			Mode:      "graduated",
			Aggregate: "sum",
		},
		{
			Name:      featurePlan("feature:inf@plan:test@0"),
			Interval:  "@daily",
			Currency:  "usd",
			Tiers:     []Tier{{}},
			Mode:      "graduated",
			Aggregate: "sum",
		},
		{
			Name:     featurePlan("feature:lic@plan:test@0"),
			Interval: "@daily",
			Currency: "usd",
		},
	}

	fps := make([]refs.FeaturePlan, len(fs))
	for i, f := range fs {
		fps[i] = f.Name
	}

	c.setClock(t, t0)
	c.Push(ctx, fs, pushLogger(t))
	if err := c.SubscribeTo(ctx, "org:example", fps); err != nil {
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
		Features:  fps,

		Plans: plans("plan:test@0"),
	}}

	diff.Test(t, t.Errorf, got, want, ignoreProviderIDs)
}

func TestSubscribeToPlan(t *testing.T) {
	fs := []Feature{{
		Name:     featurePlan("feature:x@plan:pro@0"),
		Interval: "@monthly",
		Base:     100,
		Currency: "usd",
	}, {
		Name:     featurePlan("feature:y@plan:pro@0"),
		Interval: "@monthly",
		Base:     1000,
		Currency: "usd",
	}}

	ctx := context.Background()
	tc := newTestClient(t)
	tc.Push(ctx, fs, pushLogger(t))
	tc.setClock(t, t0)

	p := plans("plan:pro@0")[0]
	if err := tc.SubscribeToPlan(ctx, "org:example", p); err != nil {
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
		Features:  FeaturePlans(fs),

		Plans: plans("plan:pro@0"),
	}}

	diff.Test(t, t.Errorf, got, want, ignoreProviderIDs)
}

func TestDedupCustomer(t *testing.T) {
	fs := []Feature{{
		Name:     featurePlan("feature:x@plan:test@0"),
		Interval: "@daily",
		Currency: "usd",
	}}

	tc := newTestClient(t)
	ctx := context.Background()
	tc.Push(ctx, fs, pushLogger(t))

	var g errgroup.Group
	for i := 0; i < 3; i++ {
		g.Go(func() error {
			return tc.SubscribeTo(ctx, "org:example", FeaturePlans(fs))
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
	fs0 := []Feature{
		{
			Name:     featurePlan("feature:x@plan:test@0"),
			Interval: "@daily",
			Currency: "usd",
		},
		{
			Name:     featurePlan("feature:y@plan:test@0"),
			Interval: "@daily",
			Currency: "usd",
		},
	}

	tc := newTestClient(t)
	ctx := context.Background()
	tc.Push(ctx, fs0, pushLogger(t))

	tc.setClock(t, t0)

	if err := tc.SubscribeTo(ctx, "org:example", FeaturePlans(fs0)); err != nil {
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
		Features:  FeaturePlans(fs0),

		Plans: plans("plan:test@0"),
	}}
	diff.Test(t, t.Errorf, got, want, ignoreProviderIDs)

	fs1 := []Feature{
		{
			Name:     featurePlan("feature:x@plan:test@1"),
			Interval: "@daily",
			Currency: "usd",
		},
		{
			Name:     featurePlan("feature:y@plan:test@1"),
			Interval: "@daily",
			Currency: "usd",
		},
	}
	tc.Push(ctx, fs1, pushLogger(t))

	fpsFrag := FeaturePlans(append(fs0, fs1[1:]...))
	if err := tc.SubscribeTo(ctx, "org:example", fpsFrag); err != nil {
		t.Fatal(err)
	}

	got, err = tc.LookupPhases(ctx, "org:example")
	if err != nil {
		t.Fatal(err)
	}

	for i, p := range got {
		p.Features = slices.Clone(p.Features)
		slices.SortFunc(p.Features, func(a, b refs.FeaturePlan) bool {
			if a.Version() < b.Version() {
				return true
			}
			return a.String() < b.String()
		})
		got[i] = p
	}

	t.Logf("got: %# v", pretty.Formatter(got))

	want = []Phase{{
		Org:       "org:example",
		Current:   true,
		Effective: t0,
		Features:  fpsFrag,

		Plans: plans("plan:test@0"),
	}}

	diff.Test(t, t.Errorf, got, want, ignoreProviderIDs)
}

func TestReportUsage(t *testing.T) {
	fs := []Feature{
		{
			Name:      featurePlan("feature:10@plan:test@0"),
			Interval:  "@monthly",
			Currency:  "usd",
			Tiers:     []Tier{{Upto: 10}},
			Mode:      "graduated",
			Aggregate: "sum",
		},
		{
			Name:      featurePlan("feature:inf@plan:test@0"),
			Interval:  "@monthly",
			Currency:  "usd",
			Tiers:     []Tier{{Upto: Inf}},
			Mode:      "graduated",
			Aggregate: "sum",
		},
		{
			Name:     featurePlan("feature:lic@plan:test@0"),
			Interval: "@monthly",
			Currency: "usd",
		},
	}

	tc := newTestClient(t)
	ctx := context.Background()
	tc.Push(ctx, fs, pushLogger(t))
	tc.setClock(t, t0)

	if err := tc.SubscribeTo(ctx, "org:example", FeaturePlans(fs)); err != nil {
		t.Fatal(err)
	}

	g, groupCtx := errgroup.WithContext(ctx)
	report := func(feature string, n int) {
		fn, err := refs.ParseName(feature)
		if err != nil {
			t.Fatal(err)
		}
		g.Go(func() (err error) {
			defer errorfmt.Handlef("%s: %w", feature, &err)
			return tc.ReportUsage(groupCtx, "org:example", fn, Report{
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
		return refs.ByName(a.Feature, b.Feature)
	})

	want := []Usage{
		{Feature: featurePlan("feature:10@plan:test@0"), Start: t0, End: endOfStripeMonth(t0), Used: 3, Limit: 10},
		{Feature: featurePlan("feature:inf@plan:test@0"), Start: t0, End: endOfStripeMonth(t0), Used: 9, Limit: Inf},
		{Feature: featurePlan("feature:lic@plan:test@0"), Start: t1, End: t2, Used: 1, Limit: Inf},
	}

	diff.Test(t, t.Errorf, got, want)
}

func TestReportUsageFeatureNotFound(t *testing.T) {
	tc := newTestClient(t)
	ctx := context.Background()

	fs := []Feature{{
		Name:      featurePlan("feature:inf@plan:test@0"),
		Interval:  "@monthly",
		Currency:  "usd",
		Tiers:     []Tier{{Upto: Inf}},
		Mode:      "graduated",
		Aggregate: "sum",
	}}

	tc.Push(ctx, fs, pushLogger(t))
	if err := tc.SubscribeTo(ctx, "org:example", FeaturePlans(fs)); err != nil {
		t.Fatal(err)
	}
	fn := refs.MustParseName("feature:nope")
	got := tc.ReportUsage(ctx, "org:example", fn, Report{})
	if !errors.Is(got, ErrFeatureNotFound) {
		t.Fatalf("got %v, want %v", got, ErrFeatureNotFound)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func ciOnly(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("not in CI; skipping long test")
	}
}

func endOfStripeMonth(t time.Time) time.Time {
	return t.AddDate(0, 1, 0).Truncate(time.Minute).Add(-5 * time.Minute)
}

func featurePlan(s string) refs.FeaturePlan {
	return must(refs.ParseFeaturePlan(s))
}

func plans(ss ...string) []refs.Plan {
	var ps []refs.Plan
	for _, s := range ss {
		ps = append(ps, refs.MustParsePlan(s))
	}
	return ps
}
