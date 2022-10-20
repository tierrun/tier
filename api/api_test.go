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
}
