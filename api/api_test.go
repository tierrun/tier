package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/exp/slices"
	"kr.dev/diff"
	"tier.run/api/apitypes"
	"tier.run/client/tier"
	"tier.run/control"
	"tier.run/fetch"
	"tier.run/fetch/fetchtest"
	"tier.run/refs"
	"tier.run/stripe/stroke"
	"tier.run/trweb"
)

var (
	mpn  = refs.MustParseName
	mpps = refs.MustParsePlans
	mpf  = refs.MustParseFeaturePlan
	mpfs = refs.MustParseFeaturePlans
)

func newTestClient(t *testing.T) (*http.Client, *control.Client) {
	sc := stroke.Client(t)
	sc = stroke.WithAccount(t, sc)
	tc := &control.Client{
		Stripe: sc,
		Logf:   t.Logf,
	}
	h := NewHandler(tc, t.Logf)
	h.helper = t.Helper
	return fetchtest.NewTLSServer(t, h.ServeHTTP), tc
}

func TestAPISubscribe(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c, cc := newTestClient(t)

	tc := &tier.Client{HTTPClient: c}

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
		err := tc.ReportUsage(ctx, apitypes.ReportRequest{
			Org:     org,
			Feature: fn,
			N:       n,
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

	whoIs("org:test", &trweb.HTTPError{
		Status:  400,
		Code:    "org_not_found",
		Message: "org not found",
	})
	sub("org:test", []string{"plan:test@0"}, nil)
	whoIs("org:test", nil)

	report("org:test", "feature:t", 9, nil)
	report("org:test", "feature:t", 1, nil)
	report("org:test", "feature:x", 1, &trweb.HTTPError{
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

	report("org:test", "feature:nope", 9, &trweb.HTTPError{
		Status:  400,
		Code:    "feature_not_found",
		Message: "feature not found",
	})

	report("org:nope", "feature:t", 9, &trweb.HTTPError{
		Status:  400,
		Code:    "org_not_found",
		Message: "org not found",
	})

	checkPhase("org:test", apitypes.PhaseResponse{
		Features: mpfs("feature:t@plan:test@0", "feature:x@plan:test@0"),
		Plans:    mpps("plan:test@0"),
	})

	sub("org:test", []string{"plan:test@0", "feature:nope@0"}, &trweb.HTTPError{
		Status:  400,
		Code:    "feature_not_found",
		Message: "feature not found",
	})
}

func TestPhaseBadOrg(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c, _ := newTestClient(t)

	_, err := fetch.OK[struct{}, *trweb.HTTPError](ctx, c, "GET", "/v1/phase?org=org:nope", nil)
	diff.Test(t, t.Errorf, err, &trweb.HTTPError{
		Status:  404,
		Code:    "not_found",
		Message: "Not Found",
	})
	_, err = fetch.OK[struct{}, *trweb.HTTPError](ctx, c, "GET", "/v1/phase", nil)
	diff.Test(t, t.Errorf, err, &trweb.HTTPError{
		Status:  400,
		Code:    "invalid_request",
		Message: `org must be prefixed with "org:"`,
	})
}

func TestPhaseFragments(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c, tc := newTestClient(t)

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
	if err := tc.Push(ctx, m, func(f control.Feature, err error) {
		if err != nil {
			t.Logf("error pushing %q: %v", f.FeaturePlan, err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	// cheating and using the tier client because ATM the API only supports
	// subscribing to plans.
	frag := m[1:]
	if err := tc.SubscribeTo(ctx, "org:test", control.FeaturePlans(frag)); err != nil {
		t.Fatal(err)
	}

	got, err := fetch.OK[apitypes.PhaseResponse, *trweb.HTTPError](ctx, c, "GET", "/v1/phase?org=org:test", nil)
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
	c, _ := newTestClient(t)

	got, err := fetch.OK[json.RawMessage, *trweb.HTTPError](ctx, c, "GET", "/v1/pull", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), "{") {
		t.Errorf("expected json, got:\n%s", got)
	}
}

func maybeFailNow(t *testing.T) {
	t.Helper()
	if t.Failed() {
		t.FailNow()
	}
}
