package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/exp/slices"
	"kr.dev/diff"
	"tier.run/api/apitypes"
	"tier.run/client/tier"
	"tier.run/control"
	"tier.run/refs"
	"tier.run/stripe/stroke"
)

var (
	mpn  = refs.MustParseName
	mpp  = refs.MustParsePlan
	mpps = refs.MustParsePlans
	mpf  = refs.MustParseFeaturePlan
	mpfs = refs.MustParseFeaturePlans
)

func newTestClient(t *testing.T) (*tier.Client, *control.Client) {
	sc := stroke.Client(t)
	sc = stroke.WithAccount(t, sc)
	cc := &control.Client{
		Stripe: sc,
		Logf:   t.Logf,
	}
	h := NewHandler(cc, t.Logf)
	h.helper = t.Helper
	s := httptest.NewTLSServer(h)
	t.Cleanup(s.Close)
	tc := &tier.Client{
		BaseURL:    s.URL,
		HTTPClient: s.Client(),
	}
	return tc, cc
}

func TestAPISubscribe(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tc, cc := newTestClient(t)

	m := []control.Feature{
		{
			FeaturePlan: mpf("feature:x@plan:test@0"),
			Interval:    "@monthly",
			Currency:    "usd",
		},
		{
			FeaturePlan: mpf("feature:t@plan:test@0"),
			Interval:    "@monthly",
			Currency:    "usd",
			Aggregate:   "sum",
			Mode:        "graduated",
			Tiers: []control.Tier{
				{Upto: control.Inf, Price: 100},
			},
		},
	}
	if err := cc.Push(ctx, m, func(f control.Feature, err error) {
		if err != nil {
			t.Logf("error pushing %q: %v", f.FeaturePlan, err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	whoIs := func(org string, wantErr error) {
		t.Helper()
		defer maybeFailNow(t)
		g, err := tc.WhoIs(ctx, org)
		diff.Test(t, t.Fatalf, err, wantErr)
		if wantErr != nil {
			return
		}
		if g.Org != org {
			t.Errorf("got = %q, want %q", g.Org, org)
		}
		if g.StripeID == "" {
			t.Error("got empty stripe customer id")
		}
	}

	sub := func(org string, features []string, wantErr error) {
		t.Helper()
		defer maybeFailNow(t)
		err := tc.Subscribe(ctx, org, features...)
		diff.Test(t, t.Errorf, err, wantErr)
	}

	report := func(org, feature string, n int, wantErr error) {
		t.Helper()
		defer maybeFailNow(t)
		fn := mpn(feature)
		err := tc.ReportUsage(ctx, org, fn.String(), n, &tier.ReportParams{
			At:      time.Now().Add(1 * time.Minute),
			Clobber: false,
		})
		diff.Test(t, t.Errorf, err, wantErr)
	}

	checkUsage := func(org string, want []apitypes.Usage) {
		t.Helper()
		defer maybeFailNow(t)
		got, err := tc.LookupLimits(ctx, org)
		if err != nil {
			t.Fatal(err)
		}
		slices.SortFunc(got.Usage, apitypes.UsageByFeature)
		diff.Test(t, t.Errorf, got, apitypes.UsageResponse{
			Org:   org,
			Usage: want,
		})
	}

	checkPhase := func(org string, want apitypes.PhaseResponse) {
		t.Helper()
		defer maybeFailNow(t)
		got, err := tc.LookupPhase(ctx, org)
		if err != nil {
			t.Fatal(err)
		}
		// actively avoiding a stripe test clock here to keep the test
		// from being horribly slow, so buying time by spot checking
		// the Effective field is at least set.
		if got.Effective.IsZero() {
			t.Error("unexpected zero effective time")
		}
		ignore := diff.ZeroFields[apitypes.PhaseResponse]("Effective")
		diff.Test(t, t.Errorf, got, want, ignore)
	}

	whoIs("org:test", &apitypes.Error{
		Status:  400,
		Code:    "org_not_found",
		Message: "org not found",
	})
	sub("org:test", []string{"plan:test@0"}, nil)
	whoIs("org:test", nil)

	report("org:test", "feature:t", 9, nil)
	report("org:test", "feature:t", 1, nil)
	report("org:test", "feature:x", 1, &apitypes.Error{
		Status:  400,
		Code:    "invalid_request",
		Message: "feature not reportable",
	})

	checkUsage("org:test", []apitypes.Usage{
		{
			Feature: mpn("feature:t"),
			Used:    10,
			Limit:   control.Inf,
		},
		{
			Feature: mpn("feature:x"),
			Used:    1,
			Limit:   control.Inf,
		},
	})

	report("org:test", "feature:nope", 9, &apitypes.Error{
		Status:  400,
		Code:    "feature_not_found",
		Message: "feature not found",
	})

	report("org:nope", "feature:t", 9, &apitypes.Error{
		Status:  400,
		Code:    "org_not_found",
		Message: "org not found",
	})

	checkPhase("org:test", apitypes.PhaseResponse{
		Features: mpfs("feature:t@plan:test@0", "feature:x@plan:test@0"),
		Plans:    mpps("plan:test@0"),
	})

	sub("org:test", []string{"plan:test@0", "feature:nope@0"}, &apitypes.Error{
		Status:  400,
		Code:    "feature_not_found",
		Message: "feature not found",
	})

	sub("org:test", []string{"plan:nope@0"}, &apitypes.Error{
		Status:  400,
		Code:    "TERR1020",
		Message: "feature or plan not found",
	})
}

func TestPhaseBadOrg(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tc, _ := newTestClient(t)
	_, err := tc.LookupPhase(ctx, "org:nope")
	diff.Test(t, t.Errorf, err, &apitypes.Error{
		Status:  404,
		Code:    "not_found",
		Message: "Not Found",
	})
	_, err = tc.LookupPhase(ctx, "")
	diff.Test(t, t.Errorf, err, &apitypes.Error{
		Status:  400,
		Code:    "invalid_request",
		Message: `org must be prefixed with "org:"`,
	})
}

func TestPhaseFragments(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tc, cc := newTestClient(t)

	m := []control.Feature{
		{
			FeaturePlan: mpf("feature:x@plan:test@0"),
			Interval:    "@monthly",
			Currency:    "usd",
		},
		{
			FeaturePlan: mpf("feature:t@plan:test@0"),
			Interval:    "@monthly",
			Currency:    "usd",
			Aggregate:   "sum",
			Mode:        "graduated",
			Tiers: []control.Tier{
				{Upto: control.Inf, Price: 100},
			},
		},
	}
	if err := cc.Push(ctx, m, func(f control.Feature, err error) {
		if err != nil {
			t.Logf("error pushing %q: %v", f.FeaturePlan, err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	// cheating and using the tier client because ATM the API only supports
	// subscribing to plans.
	frag := m[1:]
	if err := cc.SubscribeTo(ctx, "org:test", control.FeaturePlans(frag)); err != nil {
		t.Fatal(err)
	}

	got, err := tc.LookupPhase(ctx, "org:test")
	if err != nil {
		t.Fatal(err)
	}

	want := apitypes.PhaseResponse{
		Features:  mpfs("feature:t@plan:test@0"),
		Plans:     nil,
		Fragments: mpfs("feature:t@plan:test@0"),
	}

	// actively avoiding a stripe test clock here to keep the test
	// from being horribly slow, so buying time by spot checking
	// the Effective field is at least set.
	if got.Effective.IsZero() {
		t.Error("unexpected zero effective time")
	}
	ignore := diff.ZeroFields[apitypes.PhaseResponse]("Effective")
	diff.Test(t, t.Errorf, got, want, ignore)
}

func TestTierPull(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tc, _ := newTestClient(t)

	want := apitypes.Model{
		Plans: map[refs.Plan]apitypes.Plan{
			mpp("plan:test@0"): {
				Title: "plan:test@0",
				Features: map[refs.Name]apitypes.Feature{
					mpn("feature:t"): {
						Base: 9,
					},
				},
			},
		},
	}

	gotResp, err := tc.Push(ctx, want)
	if err != nil {
		t.Fatal(err)
	}

	wantResp := apitypes.PushResponse{
		Results: []apitypes.PushResult{
			{Feature: mpf("feature:t@plan:test@0"), Status: "ok", Reason: "created"},
		},
	}
	diff.Test(t, t.Errorf, gotResp, wantResp)

	got, err := tc.Pull(ctx)
	if err != nil {
		t.Fatal(err)
	}

	diff.Test(t, t.Errorf, got, want)
}

func TestPushInvalidPrice(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tc, _ := newTestClient(t)

	in := apitypes.Model{
		Plans: map[refs.Plan]apitypes.Plan{
			mpp("plan:test@0"): {
				Title: "plan:test@0",
				Features: map[refs.Name]apitypes.Feature{
					mpn("feature:t"): {
						Tiers: []apitypes.Tier{{
							Price: 0.1111111111111111,
						}},
					},
				},
			},
		},
	}

	got, err := tc.Push(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	diff.Test(t, t.Errorf, got, apitypes.PushResponse{
		Results: []apitypes.PushResult{
			{
				Feature: mpf("feature:t@plan:test@0"),
				Status:  "failed",
				Reason:  "invalid price: 0.1111111111111; tier prices must not exceed 12 decimal places",
			},
		},
	})
}

func TestWhoAmI(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tc, _ := newTestClient(t)
	a, err := tc.WhoAmI(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if a.ProviderID == "" {
		t.Error("unexpected empty provider id")
	}
	if a.URL == "" {
		t.Error("unexpected empty url")
	}
}

func TestTierReport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tc, _ := newTestClient(t)

	pr, err := tc.PushJSON(ctx, []byte(`
		{
		  "plans": {
		    "plan:test@0": {
		      "features": {
			"feature:t": {
			  "tiers": [{}]
			}
		      }
		    }
		  }
		}
	`))
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range pr.Results {
		if r.Status != "ok" {
			t.Errorf("unexpected status: %s: %q: %s", r.Feature, r.Status, r.Reason)
		}
	}

	if err := tc.Subscribe(ctx, "org:test", "plan:test@0"); err != nil {
		t.Fatal(err)
	}

	if err := tc.ReportUsage(ctx, "org:test", "feature:t", 10, &tier.ReportParams{
		// Report the usage at a time in the near future to avoid
		// complaints from Stripe about being too early (e.g. the same
		// start time as the current phase) or too late (e.g. > 5mins
		// into the future.
		//
		// If this test becomes flaky, we should use Test Clocks. For
		// now, avoid the slowness of the Test Clock API.
		At: time.Now().Add(1 * time.Minute),

		Clobber: false,
	}); err != nil {
		t.Fatal(err)
	}

	limit, used, err := tc.LookupLimit(ctx, "org:test", "feature:t")
	if err != nil {
		t.Fatal(err)
	}

	if limit != control.Inf {
		t.Errorf("limit = %d, want %d", control.Inf, limit)
	}
	if used != 10 {
		t.Errorf("used = %d, want 10", used)
	}
}

func TestScheduleWithCustomerInfoNoPhases(t *testing.T) {
	ctx := context.Background()
	tc, _ := newTestClient(t)

	p := &tier.ScheduleParams{
		Info: &tier.OrgInfo{
			Email:       "test@example.com",
			Name:        "Test Name",
			Description: "Test Desc",
			Phone:       "555-555-5555",
			Metadata: map[string]string{
				"foo":            "bar",
				"tier.filter.me": "xxxx",
			},
		},
	}

	diff.Test(t, t.Fatalf,
		tc.Schedule(ctx, "org:test", p),
		&apitypes.Error{
			Status:  400,
			Code:    "invalid_metadata",
			Message: "metadata keys must not use reserved prefix ('tier.')",
		},
	)

	got, err := tc.LookupOrg(ctx, "org:test2")
	diff.Test(t, t.Errorf,
		err,
		&apitypes.Error{
			Status:  400,
			Code:    "org_not_found",
			Message: "org not found",
		},
	)
	diff.Test(t, t.Fatalf, got, apitypes.WhoIsResponse{})

	if err := tc.Schedule(ctx, "org:test", &tier.ScheduleParams{
		Info: &tier.OrgInfo{
			Email: "test2@example2.com",
		},
	}); err != nil {
		t.Fatal(err)
	}

	got, err = tc.LookupOrg(ctx, "org:test")
	if err != nil {
		t.Fatal(err)
	}

	if got.Email != "test2@example2.com" {
		t.Errorf("unexpected email: %q", got.Email)
	}

	_, err = tc.LookupPhase(ctx, "org:test")
	var e *apitypes.Error
	if !errors.As(err, &e) {
		t.Fatal("unexpected error type")
	}
	if e.Code != "not_found" {
		t.Errorf("unexpected error: %v", e)
	}
}

func maybeFailNow(t *testing.T) {
	t.Helper()
	if t.Failed() {
		t.FailNow()
	}
}
