package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kr/pretty"
	"kr.dev/diff"
	"tier.run/api/apitypes"
	"tier.run/client/tier"
	"tier.run/control"
	"tier.run/mirror/x/exp/slices"
	"tier.run/refs"
	"tier.run/stripe"
	"tier.run/stripe/stroke"
	"tier.run/types/they"
)

var (
	mpn  = refs.MustParseName
	mpp  = refs.MustParsePlan
	mpps = refs.MustParsePlans
	mpf  = refs.MustParseFeaturePlan
	mpfs = refs.MustParseFeaturePlans
)

type testClient struct {
	t *testing.T
	*tier.Client
	cc *control.Client
}

func (tc *testClient) createCoupon(ctx context.Context, code string) {
	tc.t.Helper()
	var f stripe.Form
	f.Set("amount_off", 1)
	f.Set("currency", "usd")
	f.Set("duration", "once")
	f.Set("id", code)
	if err := tc.cc.Stripe.Do(ctx, "POST", "/v1/coupons", f, nil); err != nil {
		tc.t.Fatal(err)
	}
}

func newTestClient(t *testing.T) *testClient {
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
		Logf:       t.Logf,
		BaseURL:    s.URL,
		HTTPClient: s.Client(),
	}
	return &testClient{t, tc, cc}
}

func newTestClientWithStripe(t *testing.T, fakeStripe http.HandlerFunc) *tier.Client {
	var sc *stripe.Client
	if fakeStripe == nil {
		sc = stroke.Client(t)
		sc = stroke.WithAccount(t, sc)
	} else {
		s := httptest.NewServer(fakeStripe)
		t.Cleanup(s.Close)
		sc = &stripe.Client{
			BaseURL:    s.URL,
			HTTPClient: s.Client(),
			Logf:       t.Logf,
		}
	}

	cc := &control.Client{
		Stripe: sc,
		Logf:   t.Logf,
	}
	h := NewHandler(cc, t.Logf)
	h.helper = t.Helper
	s := httptest.NewTLSServer(h)
	t.Cleanup(s.Close)
	tc := &tier.Client{
		Logf:       t.Logf,
		BaseURL:    s.URL,
		HTTPClient: s.Client(),
	}
	return tc
}

func TestAPICheckout(t *testing.T) {
	ctx := context.Background()
	tc := newTestClient(t)
	m := []byte(`
		{"plans": {"plan:test@0": {"features": {
			"feature:x": {}
		}}}}
	`)
	pr, err := tc.PushJSON(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pushed: %s", pretty.Formatter(pr))

	t.Run("card setup", func(t *testing.T) {
		r, err := tc.Checkout(ctx, "org:test", "https://example.com/success", nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("checkout: %s", r.URL)
		if r.URL == "" {
			t.Error("unexpected empty checkout url")
		}
	})
	t.Run("subscription", func(t *testing.T) {
		r, err := tc.Checkout(ctx, "org:test", "https://example.com/success", &tier.CheckoutParams{
			Features: []string{"feature:x@plan:test@0"},
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("checkout: %s", r.URL)
		if r.URL == "" {
			t.Error("unexpected empty checkout url")
		}
	})
}

func TestAPISubscribe(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tc := newTestClient(t)

	m := []byte(`
		{"plans": {"plan:test@0": {"features": {
			"feature:x": {},
			"feature:t": {"tiers": [{"price": 100}]}
		}}}}
	`)

	pr, err := tc.PushJSON(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pushed: %s", pretty.Formatter(pr))

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
		ignore := diff.ZeroFields[apitypes.PhaseResponse]("Effective", "Current")
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
		Code:    "TERR1020",
		Message: "feature or plan not found",
	})

	sub("org:test", []string{"plan:nope@0"}, &apitypes.Error{
		Status:  400,
		Code:    "TERR1020",
		Message: "feature or plan not found",
	})

	_, err = tc.Schedule(ctx, "org:test", &tier.ScheduleParams{
		Phases: []apitypes.Phase{
			{Trial: true, Features: []string{"plan:test@0"}},
		},
		PaymentMethodID: "pm_card_us",
	})

	// Quick lint check to make sure the PaymentMethod made it to Stripe.
	// In production, payment methods can be set on a sub by sub basis;
	// however in test mode, we may only use test payment methods, and in
	// test mode, stripe does not accept test payment methods on a sub by
	// sub basis, so there is no real way to test our support for this
	// feature. Instead, here, we just check stripe complains about the
	// payment method to show it saw what we wanted it to see in
	// production.
	diff.Test(t, t.Errorf, err, &apitypes.Error{
		Status:  400,
		Code:    "invalid_payment_method",
		Message: "No such PaymentMethod: 'pm_card_us'",
	})
}

func TestScheduleAutomaticTax(t *testing.T) {
	ctx := context.Background()
	var gotTaxEnabled string
	tc := newTestClientWithStripe(t, func(w http.ResponseWriter, r *http.Request) {
		// TODO(bmizerany): Build a statefull-ish stripe mock service
		switch {
		case they.Want(r, "GET", "/v1/prices"):
			io.WriteString(w, `{"data": [{"metadata": {
				"tier.plan": "plan:test@0",
				"tier.feature": "feature:t@plan:test@0"
			}}]}`)
		case they.Want(r, "GET", "/v1/customers"):
			io.WriteString(w, `{"data":[{"id": "cus_123", "meatadata": {
				"tier.org": "org:test"
			}}]}`)
		case they.Want(r, "POST", "/v1/customers"):
			io.WriteString(w, `{"id": "cus_123"}`)
		case they.Want(r, "GET", "/v1/subscriptions"):
			io.WriteString(w, `{"data": [{
				"id": "sub_123",
				"metadata": {
					"tier.subscription": "default"
				},
				"schedule": {"id": "sub_sched_123"}
			}]}`)
		case they.Want(r, "GET", "/v1/subscription_schedules/sub_sched_123"):
			io.WriteString(w, `{"data": [{"id": "sub_sched_123", "metadata": {
				"tier.subscription": "default"
			}}]}`)
		case they.Want(r, "POST", "/v1/subscription_schedules/sub_sched_123"):
			gotTaxEnabled = r.FormValue("default_settings[automatic_tax][enabled]")
			io.WriteString(w, `{"id": "sub_sched_123"}`)
		default:
			t.Logf("unexpected stripe request: %s %s", r.Method, r.URL)
			w.WriteHeader(999)
			io.WriteString(w, `{}`)
		}
	})
	_, err := tc.Schedule(ctx, "org:test", &tier.ScheduleParams{
		Tax: tier.Taxation{Automatic: true},
		Phases: []apitypes.Phase{
			{
				Features: []string{"plan:test@0"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if gotTaxEnabled != "true" {
		t.Errorf("got %q, want true", gotTaxEnabled)
	}
}

func TestCancel(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tc := newTestClient(t)
	_, err := tc.PushJSON(ctx, []byte(`{
	  "plans": {
	    "plan:test@0": {
	      "features": {
		"feature:t": {
		  "tiers": [{}]
		}
	      }
	    }
	  }
	}`))
	if err != nil {
		t.Fatal(err)
	}

	checkUsage := func(org string, want []apitypes.Usage) {
		t.Helper()
		u, err := tc.LookupLimits(ctx, org)
		if err != nil {
			t.Fatal(err)
		}
		diff.Test(t, t.Errorf, u.Usage, want)
	}

	if err := tc.Subscribe(ctx, "org:test", "plan:test@0"); err != nil {
		t.Fatal(err)
	}
	checkUsage("org:test", []apitypes.Usage{
		{Feature: mpn("feature:t"), Used: 0, Limit: control.Inf},
	})

	if err := tc.Cancel(ctx, "org:test"); err != nil {
		t.Fatal(err)
	}
	checkUsage("org:test", nil)
}

func TestPhaseBadOrg(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tc := newTestClient(t)
	_, err := tc.LookupPhase(ctx, "org:nope")
	diff.Test(t, t.Errorf, err, &apitypes.Error{
		Status:  400,
		Code:    "org_not_found",
		Message: "org not found",
	})
	_, err = tc.LookupPhase(ctx, "")
	diff.Test(t, t.Errorf, err, &apitypes.Error{
		Status:  400,
		Code:    "invalid_request",
		Message: `org must be prefixed with "org:"`,
	})
}

func TestPhase(t *testing.T) {
	t.Parallel()

	now := time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)

	ctx := context.Background()
	tc := newTestClient(t)

	ctx, err := tc.WithClock(ctx, t.Name(), now)
	if err != nil {
		t.Fatal(err)
	}

	m := []byte(`
		{"plans": {"plan:test@0": {"features": {"feature:x": {}}}}}
	`)

	_, err = tc.PushJSON(ctx, m)
	if err != nil {
		t.Fatal(err)
	}

	tc.createCoupon(ctx, "coupon_test")

	cases := []struct {
		phases []tier.Phase
		want   apitypes.PhaseResponse
	}{
		{
			phases: []tier.Phase{{
				Trial:    true,
				Features: []string{"plan:test@0"},
				Coupon:   "coupon_test",
				CouponData: &apitypes.Coupon{
					ID: "coupon_test",
				},
			}, {
				Effective: now.AddDate(0, 0, 14),
				Trial:     false,
				Features:  []string{"plan:test@0"},
			}},
			want: apitypes.PhaseResponse{
				Effective: now,
				End:       now.AddDate(0, 0, 14),
				Features:  mpfs("feature:x@plan:test@0"),
				Plans:     mpps("plan:test@0"),
				Trial:     true,
				Coupon:    "coupon_test",
				CouponData: &apitypes.Coupon{
					ID: "coupon_test",
				},
			},
		},
		{
			phases: []tier.Phase{{
				Features: []string{"plan:test@0"},
			}},
			want: apitypes.PhaseResponse{
				Effective: now,
				Features:  mpfs("feature:x@plan:test@0"),
				Plans:     mpps("plan:test@0"),
			},
		},
		{
			phases: []tier.Phase{{
				Trial:    true,
				Features: []string{"plan:test@0"},
			}},
			want: apitypes.PhaseResponse{
				Trial:     true,
				Effective: now,
				Features:  mpfs("feature:x@plan:test@0"),
				Plans:     mpps("plan:test@0"),
			},
		},
	}

	for _, tt := range cases {
		t.Run("", func(t *testing.T) {
			if _, err := tc.Schedule(ctx, "org:test", &tier.ScheduleParams{Phases: tt.phases}); err != nil {
				t.Fatal(err)
			}
			got, err := tc.LookupPhase(ctx, "org:test")
			if err != nil {
				t.Fatal(err)
			}

			c := got.Current

			if c.Effective.IsZero() {
				t.Errorf("got zero effective time, want non-zero")
			}
			if c.End.IsZero() {
				t.Errorf("got zero end time, want non-zero")
			}
			if !c.End.After(c.Effective) {
				t.Errorf("unexpected effective time %s after end time %s", c.Effective, c.End)
			}

			got.Current = apitypes.Period{}
			diff.Test(t, t.Errorf, got, tt.want, diff.KeepFields[apitypes.Coupon]("ID"))
		})
	}
}

func TestPhaseFragments(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tc := newTestClient(t)

	m := []byte(`
		{"plans": {"plan:test@0": {"features": {
			"feature:x": {},
			"feature:t": {"tiers": [{"price": 100}]}
		}}}}
	`)

	pr, err := tc.PushJSON(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pushed: %s", pretty.Formatter(pr))

	if err := tc.Subscribe(ctx, "org:test", "feature:x@plan:test@0"); err != nil {
		t.Fatal(err)
	}

	got, err := tc.LookupPhase(ctx, "org:test")
	if err != nil {
		t.Fatal(err)
	}

	want := apitypes.PhaseResponse{
		Features:  mpfs("feature:x@plan:test@0"),
		Plans:     nil,
		Fragments: mpfs("feature:x@plan:test@0"),
	}

	// actively avoiding a stripe test clock here to keep the test
	// from being horribly slow, so buying time by spot checking
	// the Effective field is at least set.
	if got.Effective.IsZero() {
		t.Error("unexpected zero effective time")
	}
	ignore := diff.ZeroFields[apitypes.PhaseResponse]("Effective", "Current")
	diff.Test(t, t.Errorf, got, want, ignore)
}

func TestTierPull(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tc := newTestClient(t)

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
	tc := newTestClient(t)

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
	tc := newTestClient(t)
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

func TestClock(t *testing.T) {
	t.Parallel()

	tc := newTestClient(t)

	now := time.Now().Truncate(time.Second)
	ctx, err := tc.WithClock(context.Background(), t.Name(), now)
	if err != nil {
		t.Fatal(err)
	}

	now = now.Add(100 * time.Hour)
	if err := tc.Advance(ctx, now); err != nil {
		t.Fatal(err)
	}

	if err := tc.Subscribe(ctx, "org:test"); err != nil {
		t.Fatal(err)
	}

	c, err := tc.LookupOrg(ctx, "org:test")
	if err != nil {
		t.Fatal(err)
	}

	if !c.Created.Equal(now) {
		t.Errorf("c.Created = %v; want %v", c.Created, now)
	}
}

func TestTierReport(t *testing.T) {
	t.Parallel()

	tc := newTestClient(t)

	now := time.Now()
	ctx, err := tc.WithClock(context.Background(), t.Name(), now)
	if err != nil {
		t.Fatal(err)
	}

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

	report := func(n int, at time.Time, wantErr error) {
		t.Helper()
		if err := tc.ReportUsage(ctx, "org:test", "feature:t", n, &tier.ReportParams{
			// Force 'now' at Stripe.
			At: at,

			Clobber: false,
		}); !errors.Is(err, wantErr) {
			t.Errorf("err = %v; want %v", err, wantErr)
		}
	}

	report(10, now, nil)
	report(10, now.Add(1*time.Minute), nil)

	limit, used, err := tc.LookupLimit(ctx, "org:test", "feature:t")
	if err != nil {
		t.Fatal(err)
	}

	if limit != control.Inf {
		t.Errorf("limit = %d, want %d", control.Inf, limit)
	}
	if used != 20 {
		t.Errorf("used = %d, want 10", used)
	}
}

func TestScheduleWithCustomerInfoNoPhases(t *testing.T) {
	ctx := context.Background()
	tc := newTestClient(t)

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

	_, err := tc.Schedule(ctx, "org:test", p)
	diff.Test(t, t.Fatalf,
		err,
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

	if _, err := tc.Schedule(ctx, "org:test", &tier.ScheduleParams{
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

func TestScheduleWithCustomerPaymentMethodUpdate(t *testing.T) {
	ctx := context.Background()
	tc := newTestClient(t)

	_, err := tc.Schedule(ctx, "org:test", &tier.ScheduleParams{
		Info: &tier.OrgInfo{
			PaymentMethod: "pm_card_us", // "auto attach" payment method
			Email:         "e@example.com",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	pms, err := tc.LookupPaymentMethods(ctx, "org:test")
	if err != nil {
		t.Fatal(err)
	}
	if len(pms.PaymentMethods) != 1 {
		t.Fatalf("unexpected number of payment methods: %d", len(pms.PaymentMethods))
	}

	resolvedPM := pms.PaymentMethods[0].ProviderID()
	_, err = tc.Schedule(ctx, "org:test", &tier.ScheduleParams{
		Info: &tier.OrgInfo{
			InvoiceSettings: apitypes.InvoiceSettings{
				DefaultPaymentMethod: resolvedPM,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	o, err := tc.LookupOrg(ctx, "org:test")
	if err != nil {
		t.Fatal(err)
	}

	if o.InvoiceSettings.DefaultPaymentMethod != resolvedPM {
		t.Errorf("default_payment_method = %q; want %q", o.InvoiceSettings.DefaultPaymentMethod, resolvedPM)
	}
}

func TestPaymentMethods(t *testing.T) {
	ctx := context.Background()
	tc := newTestClient(t)

	if err := tc.Subscribe(ctx, "org:test"); err != nil {
		t.Fatal(err)
	}

	got, err := tc.LookupPaymentMethods(ctx, "org:test")
	if err != nil {
		t.Fatal(err)
	}

	want := apitypes.PaymentMethodsResponse{
		Org:            "org:test",
		PaymentMethods: nil,
	}

	diff.Test(t, t.Errorf, got, want)
}

func maybeFailNow(t *testing.T) {
	t.Helper()
	if t.Failed() {
		t.FailNow()
	}
}
