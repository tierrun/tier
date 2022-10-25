package api

import (
	"context"
	"net/http"
	"testing"

	"golang.org/x/exp/slices"
	"kr.dev/diff"
	"tier.run/api/apitypes"
	"tier.run/client/tier"
	"tier.run/fetch"
	"tier.run/fetch/fetchtest"
	"tier.run/stripe/stroke"
	"tier.run/trweb"
)

func newTestClient(t *testing.T) (*http.Client, *tier.Client) {
	sc := stroke.Client(t)
	sc = stroke.WithAccount(t, sc)
	tc := &tier.Client{
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
	c, tc := newTestClient(t)

	m := []tier.Feature{
		{
			Plan:     "plan:test@0",
			Name:     "feature:x",
			Interval: "@monthly",
			Currency: "usd",
		},
		{
			Plan:      "plan:test@0",
			Name:      "feature:t",
			Interval:  "@monthly",
			Currency:  "usd",
			Aggregate: "sum",
			Mode:      "graduated",
			Tiers: []tier.Tier{
				{Upto: tier.Inf, Price: 100},
			},
		},
	}
	if err := tc.Push(ctx, m, func(f tier.Feature, err error) {
		if err != nil {
			t.Logf("error pushing [%q %q]: %v", f.Plan, f.Name, err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	whoIs := func(org string, wantErr error) {
		t.Helper()
		g, err := fetch.OK[apitypes.WhoIsResponse, *trweb.HTTPError](ctx, c, "GET", "/v1/whois?org="+org, nil)
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
		_, err := fetch.OK[struct{}, *trweb.HTTPError](ctx, c, "POST", "/v1/subscribe", &apitypes.SubscribeRequest{
			Org: org,
			Phases: []apitypes.Phase{{
				Features: features,
			}},
		})
		diff.Test(t, t.Errorf, err, wantErr)
	}

	report := func(org, feature string, n int, wantErr error) {
		t.Helper()
		_, err := fetch.OK[struct{}, *trweb.HTTPError](ctx, c, "POST", "/v1/report", &apitypes.ReportRequest{
			Feature: feature,
			Org:     org,
			N:       n,
		})
		diff.Test(t, t.Errorf, err, wantErr)
	}

	checkUsage := func(org string, want []apitypes.Usage) {
		t.Helper()
		got, err := fetch.OK[apitypes.UsageResponse, *trweb.HTTPError](ctx, c, "GET", "/v1/limits?org="+org, nil)
		if err != nil {
			t.Fatal(err)
		}
		slices.SortFunc(got.Usage, func(a, b apitypes.Usage) bool {
			return a.Feature < b.Feature
		})
		diff.Test(t, t.Errorf, got, apitypes.UsageResponse{
			Org:   org,
			Usage: want,
		})
	}

	checkPhase := func(org string, want apitypes.PhaseResponse) {
		t.Helper()
		got, err := fetch.OK[apitypes.PhaseResponse, *trweb.HTTPError](ctx, c, "GET", "/v1/phase?org="+org, nil)
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
			Feature: "feature:t",
			Used:    10,
			Limit:   tier.Inf,
		},
		{
			Feature: "feature:x",
			Used:    1,
			Limit:   tier.Inf,
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
		Features: []string{"feature:t@plan:test@0", "feature:x@plan:test@0"},
		Plans:    []string{"plan:test@0"},
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

	m := []tier.Feature{
		{
			Plan:     "plan:test@0",
			Name:     "feature:x",
			Interval: "@monthly",
			Currency: "usd",
		},
		{
			Plan:      "plan:test@0",
			Name:      "feature:t",
			Interval:  "@monthly",
			Currency:  "usd",
			Aggregate: "sum",
			Mode:      "graduated",
			Tiers: []tier.Tier{
				{Upto: tier.Inf, Price: 100},
			},
		},
	}
	if err := tc.Push(ctx, m, func(f tier.Feature, err error) {
		if err != nil {
			t.Logf("error pushing [%q %q]: %v", f.Plan, f.Name, err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	// cheating and using the tier client because ATM the API only supports
	// subscribing to plans.
	frag := m[1:]
	if err := tc.SubscribeTo(ctx, "org:test", frag); err != nil {
		t.Fatal(err)
	}

	got, err := fetch.OK[apitypes.PhaseResponse, *trweb.HTTPError](ctx, c, "GET", "/v1/phase?org=org:test", nil)
	if err != nil {
		t.Fatal(err)
	}

	want := apitypes.PhaseResponse{
		Features:  []string{"feature:t@plan:test@0"},
		Plans:     nil,
		Fragments: []string{"feature:t@plan:test@0"},
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
