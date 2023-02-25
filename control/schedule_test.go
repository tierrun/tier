package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/kr/pretty"
	"github.com/tailscale/hujson"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"kr.dev/diff"
	"kr.dev/errorfmt"
	"tier.run/refs"
	"tier.run/stripe"
	"tier.run/values"
)

var (
	mpf = refs.MustParseFeaturePlan
	mpp = refs.MustParsePlan
	mpn = refs.MustParseName
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

	s := newScheduleTester(t)

	var model []Feature
	plan := func(ff []Feature) []refs.FeaturePlan {
		model = append(model, ff...)
		var fps []refs.FeaturePlan
		for _, f := range ff {
			fps = append(fps, f.FeaturePlan)
		}
		return fps
	}

	planFree := plan([]Feature{{
		FeaturePlan: mpf("feature:x@plan:free@0"),
		Interval:    "@monthly",
		Currency:    "usd",
	}})

	planPro := plan([]Feature{{
		FeaturePlan: mpf("feature:x@plan:pro@0"),
		Interval:    "@monthly",
		Base:        100,
		Currency:    "usd",
	}})

	s.push(model)

	check := func(org string, want []Phase) {
		t.Helper()
		got, err := s.cc.LookupPhases(s.ctx, org)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("got phases %# v", pretty.Formatter(got))
		diff.Test(t, t.Errorf, got, want, ignoreProviderIDs)
	}

	s.schedule("org:example", 0, "", planFree...)
	check("org:example", []Phase{{
		Org:       "org:example",
		Current:   true,
		Effective: t0,
		Features:  planFree,
		Plans:     plans("plan:free@0"),
	}})

	s.advanceTo(t1)
	s.schedule("org:example", 0, "", planPro...)
	check("org:example", []Phase{
		{
			Org:       "org:example",
			Current:   true,
			Effective: t1, // unchanged by advanced clock
			Features:  planPro,
			Plans:     plans("plan:pro@0"),
		},
	})

	// downgrade and check no new phases
	s.schedule("org:example", 0, "", planFree...)
	check("org:example", []Phase{
		{
			Org:       "org:example",
			Current:   true,
			Effective: t1, // unchanged by advanced clock
			Features:  planFree,
			Plans:     plans("plan:free@0"),
		},
	})
}

type scheduleTester struct {
	ctx   context.Context
	t     *testing.T
	cc    *Client
	clock *Clock
}

func newScheduleTester(t *testing.T) *scheduleTester {
	t.Helper()
	c := newTestClient(t)
	clock, err := c.NewClock(context.Background(), t.Name(), t0)
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithClock(context.Background(), clock.ID())
	return &scheduleTester{ctx: ctx, t: t, cc: c, clock: clock}
}

func (s *scheduleTester) push(model []Feature) {
	s.t.Helper()
	s.cc.Push(s.ctx, model, pushLogger(s.t))
	if s.t.Failed() {
		s.t.FailNow()
	}
}

func (s *scheduleTester) advance(days int) {
	s.advanceTo(s.clock.Present().AddDate(0, 0, days))
}

func (s *scheduleTester) advanceTo(t time.Time) {
	if err := s.clock.Advance(s.ctx, t); err != nil {
		s.t.Fatal(err)
	}
	if err := s.clock.Wait(s.ctx); err != nil {
		s.t.Fatal(err)
	}
}

func (s *scheduleTester) advanceToNextPeriod() {
	// TODO(bmizerany): make Phase aware so that it jumps based on the
	// start of the next phase if the current phase ends sooner than than 1
	// interval of the current phase.
	now := s.clock.Present()
	eop := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	s.t.Logf("advancing to next period %s", eop)
	s.advanceTo(eop)
}

func (s *scheduleTester) cancel(org string) {
	s.t.Helper()
	s.t.Logf("cancelling %s", org)
	s.schedule(org, 0, "") // no features
}

func (s *scheduleTester) setPaymentMethod(org string, pm string) {
	s.t.Helper()
	s.t.Logf("setting payment method for %s to %s", org, pm)
	if err := s.cc.PutCustomer(s.ctx, org, &OrgInfo{
		PaymentMethod: pm,
		InvoiceSettings: InvoiceSettings{
			DefaultPaymentMethod: pm,
		},
	}); err != nil {
		s.t.Fatal(err)
	}
	if s.t.Failed() {
		s.t.FailNow()
	}
}

func (s *scheduleTester) schedule(org string, trialDays int, payment string, fs ...refs.FeaturePlan) {
	s.t.Helper()
	s.t.Logf("subscribing %s to %v with trialDays=%d", org, fs, trialDays)

	var ps []Phase
	if trialDays < 0 {
		ps = []Phase{{Trial: true, Features: fs}}
	} else if trialDays == 0 {
		ps = []Phase{{Features: fs}}
	} else {
		ps = []Phase{{
			Trial:    true,
			Features: fs,
		}, {
			Effective: t0.AddDate(0, 1, 0),
			Features:  fs,
		}}
	}
	p := ScheduleParams{
		PaymentMethod: payment,
		Phases:        ps,
	}
	if err := s.cc.Schedule(s.ctx, org, p); err != nil {
		s.t.Fatalf("error subscribing: %v", err)
	}
}

func (s *scheduleTester) report(org, name string, n int) {
	s.t.Helper()
	if err := s.cc.ReportUsage(s.ctx, org, mpn(name), Report{
		N: n,
	}); err != nil {
		s.t.Fatal(err)
	}
}

// ignores start/end times, and usage
//
//lint:ignore U1000 saving for a rainy day
func (s *scheduleTester) checkLimits(org string, want []Usage) {
	s.t.Helper()
	got, err := s.cc.LookupLimits(s.ctx, org)
	if err != nil {
		s.t.Fatal(err)
	}
	slices.SortFunc(got, func(a, b Usage) bool {
		return refs.ByName(a.Feature, b.Feature)
	})
	s.diff(got, want, diff.ZeroFields[Usage]("Start", "End", "Feature"))
	if s.t.Failed() {
		s.t.FailNow()
	}
}

// ignores period dates
func (s *scheduleTester) checkInvoices(org string, want []Invoice) {
	s.t.Helper()
	got, err := s.cc.LookupInvoices(s.ctx, org)
	if err != nil {
		s.t.Fatal(err)
	}
	s.t.Logf("got invoices %# v", pretty.Formatter(got))
	ignorePeriod := diff.KeepFields[Period]()
	s.diff(got, want, ignorePeriod)
}

func (s *scheduleTester) diff(got, want any, opts ...diff.Option) {
	s.t.Helper()
	diff.Test(s.t, s.t.Errorf, got, want, opts...)
}

func lineItem(fp refs.FeaturePlan, quantity int, amount float64) InvoiceLineItem {
	return InvoiceLineItem{
		Feature:  fp,
		Quantity: quantity,
		Amount:   amount,
	}
}

func TestScheduleFreeTrials(t *testing.T) {
	s := newScheduleTester(t)

	featureX := mpf("feature:x@plan:test@0")

	s.push([]Feature{{
		FeaturePlan: featureX,
		Interval:    "@monthly",
		Currency:    "usd",
		Mode:        "graduated",
		Aggregate:   "sum",
		Tiers:       []Tier{{Upto: Inf, Price: 1}},
	}})

	s.schedule("org:paid", 0, "", featureX)
	s.schedule("org:trial", 14, "", featureX)
	s.schedule("org:free", -1, "", featureX)

	s.report("org:paid", "feature:x", 1)
	s.report("org:trial", "feature:x", 1)
	s.report("org:free", "feature:x", 1)

	s.advance(15)

	s.report("org:paid", "feature:x", 1)
	s.report("org:trial", "feature:x", 1)
	s.report("org:free", "feature:x", 1)

	s.advance(31)

	zero := lineItem(featureX, 0, 0)
	s.checkInvoices("org:paid", []Invoice{{
		Lines: []InvoiceLineItem{
			lineItem(featureX, 2, 2),
			zero,
		},
		SubtotalPreTax: 2,
		Subtotal:       2,
		TotalPreTax:    2,
		Total:          2,
	}, {
		Lines: []InvoiceLineItem{
			zero,
		},
	}})

	s.checkInvoices("org:trial", []Invoice{{
		Lines: []InvoiceLineItem{
			lineItem(featureX, 2, 0),
		},
	}, {
		Lines: []InvoiceLineItem{
			zero,
		},
	}})

	s.checkInvoices("org:free", []Invoice{{
		Lines: []InvoiceLineItem{
			lineItem(featureX, 2, 0),
		},
	}, {
		Lines: []InvoiceLineItem{
			zero,
		},
	}})
}

func TestSchedule_TrialSwapWithPaid(t *testing.T) {
	s := newScheduleTester(t)

	featureX := mpf("feature:x@plan:test@0")

	s.push([]Feature{{
		FeaturePlan: featureX,
		Interval:    "@monthly",
		Currency:    "usd",
		Mode:        "graduated",
		Aggregate:   "sum",
		Tiers:       []Tier{{Upto: Inf}},
	}})

	steps := []struct {
		trialDays  int
		wantStatus string
	}{
		{0, "active"},
		{14, "trialing"},
		{0, "active"},
		{-1, "trialing"},
		{0, "active"},
		{14, "trialing"},
		{0, "active"},
	}

	for i, step := range steps {
		t.Logf("step: %+v", step)
		s.schedule("org:test", step.trialDays, "", featureX)
		status, err := s.cc.LookupStatus(s.ctx, "org:test")
		if err != nil {
			t.Fatalf("[%d]: unexpected error: %v", i, err)
		}
		if status != step.wantStatus {
			t.Errorf("[%d]: status = %q, want %q", i, status, step.wantStatus)
		}
	}
}

func TestScheduleCancel(t *testing.T) {
	featureX := mpf("feature:x@plan:test@0")
	featureBase := mpf("feature:base@plan:test@0")

	s := newScheduleTester(t)
	s.push([]Feature{{
		FeaturePlan: featureX,
		Interval:    "@monthly",
		Currency:    "usd",
		Mode:        "graduated",
		Aggregate:   "sum",
		Tiers:       []Tier{{Upto: Inf, Base: 1000}},
	}, {
		FeaturePlan: featureBase,
		Interval:    "@monthly",
		Currency:    "usd",
		Base:        31 * 1000,
	}})

	s.setPaymentMethod("org:paid", "pm_card_us")
	s.schedule("org:paid", 0, "", featureX, featureBase)
	s.report("org:paid", "feature:x", 99)
	s.advance(10)
	s.cancel("org:paid")
	s.advanceToNextPeriod()

	// check usage is billed
	s.checkInvoices("org:paid", []Invoice{{
		Lines: []InvoiceLineItem{{
			Feature:   featureBase,
			Quantity:  1,
			Amount:    -21000,
			Proration: true,
		},
			lineItem(featureX, 99, 0),   // usage on feature:x
			lineItem(featureX, 0, 1000), // flag fee on feature:x
		},
		SubtotalPreTax: -20000,
		Subtotal:       -20000,
		TotalPreTax:    -20000,
		Total:          -20000,
	}, {
		Lines: []InvoiceLineItem{
			lineItem(featureBase, 1, 31000),
			lineItem(featureX, 0, 0),
		},
		SubtotalPreTax: 31000,
		Subtotal:       31000,
		TotalPreTax:    31000,
		Total:          31000,
	}})
}

func TestScheduleCancelNothing(t *testing.T) {
	s := newScheduleTester(t)
	s.cancel("org:paid")
	// nothing failed, success
}

func TestScheduleCancelNoLimits(t *testing.T) {
	featureX := mpf("feature:x@plan:test@0")
	s := newScheduleTester(t)
	s.push([]Feature{{
		FeaturePlan: featureX,
		Interval:    "@monthly",
		Currency:    "usd",
		Mode:        "graduated",
		Aggregate:   "sum",
		Tiers:       []Tier{{Upto: Inf, Base: 1000}},
	}})
	s.schedule("org:paid", 0, "", featureX)
	s.checkLimits("org:paid", []Usage{{Feature: featureX, Limit: Inf}})
	s.cancel("org:paid")
	s.checkLimits("org:paid", nil)
}

func TestScheduleMinMaxItems(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	var fs []Feature
	for i := 0; i < 21; i++ {
		fs = append(fs, Feature{
			FeaturePlan: mpf(fmt.Sprintf("feature:%d@plan:test@0", i)),
			Interval:    "@daily",
			Currency:    "usd",
		})
	}

	c.Push(ctx, fs, pushLogger(t))

	// effectively cancel an org that does not exist
	err := c.SubscribeTo(ctx, "org:example", nil)
	if err != nil {
		// canceling an org that does not exist is not an error, it's a nop
		t.Fatal(err)
	}

	fps := FeaturePlans(fs)
	err = c.SubscribeTo(ctx, "org:example", fps)
	if !errors.Is(err, ErrTooManyItems) {
		t.Fatalf("got %v, want %v", err, ErrTooManyItems)
	}

	// check that we can still subscribe to the max number of items
	wantFeatures := fps[:20]
	if err := c.SubscribeTo(ctx, "org:example", wantFeatures); err != nil {
		t.Fatal(err)
	}
	got, err := c.LookupPhases(ctx, "org:example")
	if err != nil {
		t.Fatal(err)
	}

	want := []Phase{{
		Org:      "org:example",
		Features: wantFeatures,
		Current:  true,
		Plans:    nil, // fragments only
	}}
	diff.Test(t, t.Errorf, got, want, diff.ZeroFields[Phase]("Effective"))
}

func TestSchedulePaymentMethod(t *testing.T) {
	// Here we mock because test payment methods are not supported for use
	// with subscriptions default_settings. Yay.
	// From Stripe Discord:
	//   > bmizerany: Does that mean that only manual testing using a browser
	//     to collect fake card numbers and manually check the subscription?
	//   > stripe: Yep.

	type G struct {
		updateSched bool
		fromSub     string
		card        string // "not_set" means not set
	}

	const existingSub = `{"data":[{
		"id": "sub_123",
		"metadata": {"tier.subscription": "default"},
	}]}`

	cases := []struct {
		subResp string
		card    string
		want    []G
	}{
		{
			subResp: `{}`,
			card:    "pm_card_FAKE",
			want: []G{
				{card: "pm_card_FAKE"},
			},
		},
		{
			subResp: existingSub,
			card:    "pm_card_FAKE",
			want: []G{
				{card: "not_set", fromSub: "sub_123"},
				{card: "pm_card_FAKE", updateSched: true},
			},
		},
		{
			subResp: `{}`,
			want: []G{
				{card: "not_set"},
			},
		},
		{
			subResp: existingSub,
			want: []G{
				{card: "not_set", fromSub: "sub_123"},
				{card: "not_set", updateSched: true},
			},
		},
		{
			subResp: `{"data":[{
				"id": "sub_123",
				"metadata": {"tier.subscription": "default"},
				"schedule": {"id": "sub_sched_123"},
			}]}`,
			want: []G{
				{card: "not_set", updateSched: true},
			},
		},
	}

	for _, tt := range cases {
		t.Run("", func(t *testing.T) {
			var got List[G]
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Logf("fake stripe: %s %s", r.Method, r.URL.Path)
				switch {
				case wants(r, "GET", "/v1/customers"):
					var c stripeCustomer
					c.ID = "cust_123"
					c.Metadata.Org = "org:example"
					jsonEncodeList(t, w, c)
				case wants(r, "GET", "/v1/subscriptions"):
					writeHuJSON(w, tt.subResp) // force new schedule
				case wants(r, "GET", "/v1/prices"):
					var p stripePrice
					p.ID = "price_123"
					p.Metadata.Feature = mpf("feature:x@plan:test@0")
					jsonEncodeList(t, w, p)
				case wants(r, "GET", "/v1/subscription_schedules/sub_sched_123"):
					var ss stripeSubSchedule
					jsonEncode(t, w, ss)
				case wants(r, "POST", "/v1/subscription_schedules/sub_sched_123"):
					const key = "default_settings[default_payment_method]"
					pm := r.FormValue(key)
					if _, ok := r.Form[key]; !ok {
						pm = "not_set"
					}
					got.Append(G{updateSched: true, card: pm})
				case wants(r, "POST", "/v1/subscription_schedules"):
					fromSub := r.FormValue("from_subscription")
					const key = "default_settings[default_payment_method]"
					pm := r.FormValue(key)
					if _, ok := r.Form[key]; !ok {
						pm = "not_set"
					}
					got.Append(G{fromSub: fromSub, card: pm})

					writeHuJSON(w, `{"id": "sub_sched_123"}`)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
			}))
			t.Cleanup(s.Close)

			cc := &Client{
				Logf: t.Logf,
				Stripe: &stripe.Client{
					BaseURL: s.URL,
				},
			}

			if err := cc.Schedule(context.Background(), "org:example", ScheduleParams{
				PaymentMethod: tt.card,
				Phases: []Phase{{
					Features: []refs.FeaturePlan{mpf("feature:x@plan:test@0")},
				}},
			}); err != nil {
				t.Fatal(err)
			}

			diff.Test(t, t.Errorf, got.Slice(), tt.want)
		})
	}
}

func TestLookupPhasesWithTiersRoundTrip(t *testing.T) {
	s := newScheduleTester(t)

	fs := []Feature{
		{
			// TODO(bmizerany): G: check/test plan name formats
			FeaturePlan: mpf("feature:10@plan:test@0"),
			Interval:    "@daily",
			Currency:    "usd",
			Tiers:       []Tier{{Upto: 10}},
			Mode:        "graduated",
			Aggregate:   "sum",
		},
		{
			FeaturePlan: mpf("feature:inf@plan:test@0"),
			Interval:    "@daily",
			Currency:    "usd",
			Tiers:       []Tier{{}},
			Mode:        "graduated",
			Aggregate:   "sum",
		},
		{
			FeaturePlan: mpf("feature:lic@plan:test@0"),
			Interval:    "@daily",
			Currency:    "usd",
		},
	}

	fps := make([]refs.FeaturePlan, len(fs))
	for i, f := range fs {
		fps[i] = f.FeaturePlan
	}

	s.push(fs)
	s.schedule("org:example", 0, "", fps...)

	got, err := s.cc.LookupPhases(s.ctx, "org:example")
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
		FeaturePlan: mpf("feature:x@plan:pro@0"),
		Interval:    "@monthly",
		Base:        100,
		Currency:    "usd",
	}, {
		FeaturePlan: mpf("feature:y@plan:pro@0"),
		Interval:    "@monthly",
		Base:        1000,
		Currency:    "usd",
	}}

	s := newScheduleTester(t)
	s.push(fs)

	efs, err := Expand(fs, "plan:pro@0")
	if err != nil {
		t.Fatal(err)
	}

	s.schedule("org:example", 0, "", efs...)

	got, err := s.cc.LookupPhases(s.ctx, "org:example")
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
		FeaturePlan: mpf("feature:x@plan:test@0"),
		Interval:    "@daily",
		Currency:    "usd",
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
			FeaturePlan: mpf("feature:x@plan:test@0"),
			Interval:    "@daily",
			Currency:    "usd",
		},
		{
			FeaturePlan: mpf("feature:y@plan:test@0"),
			Interval:    "@daily",
			Currency:    "usd",
		},
	}

	s := newScheduleTester(t)
	s.push(fs0)
	s.schedule("org:example", 0, "", FeaturePlans(fs0)...)

	got, err := s.cc.LookupPhases(s.ctx, "org:example")
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
			FeaturePlan: mpf("feature:x@plan:test@1"),
			Interval:    "@daily",
			Currency:    "usd",
		},
		{
			FeaturePlan: mpf("feature:y@plan:test@1"),
			Interval:    "@daily",
			Currency:    "usd",
		},
	}
	s.push(fs1)

	fpsFrag := FeaturePlans(append(fs0, fs1[1:]...))
	s.schedule("org:example", 0, "", fpsFrag...)

	got, err = s.cc.LookupPhases(s.ctx, "org:example")
	if err != nil {
		t.Fatal(err)
	}

	for i, p := range got {
		p.Features = slices.Clone(p.Features)
		refs.SortGroupedByVersion(p.Features)
		got[i] = p
	}

	want = []Phase{{
		Org:       "org:example",
		Current:   true,
		Effective: t0,
		Features:  fpsFrag,

		Plans: plans("plan:test@0"),
	}}

	diff.Test(t, t.Errorf, got, want, ignoreProviderIDs)
}

func TestLookupPaymentMethods(t *testing.T) {
	tc := newTestClient(t)
	ctx := context.Background()
	if err := tc.PutCustomer(ctx, "org:example", nil); err != nil {
		t.Fatal(err)
	}

	// We can't get payment methods back from Stripe in Test Mode without
	// manually creating them via SetupIntents. So we'll just check that we
	// get an empty list, without an error.
	pms, err := tc.LookupPaymentMethods(ctx, "org:example")
	if err != nil {
		t.Fatal(err)
	}
	if len(pms) != 0 {
		t.Errorf("len(pms) = %d, expected 0", len(pms))
	}
}

func TestCheckoutRequiredAddress(t *testing.T) {
	type G struct {
		successURL string
		cancelURL  string
		bac        string // billing_address_collection
		trialDays  string
	}

	var mu sync.Mutex
	var got []G
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case wants(r, "GET", "/v1/customers"):
			jsonEncode(t, w, msa{
				"data": []msa{
					{
						"metadata": msa{
							"tier.org": "org:demo",
						},
					},
				},
			})
		case wants(r, "POST", "/v1/checkout/sessions"):
			mu.Lock()
			got = append(got, G{
				successURL: r.FormValue("success_url"),
				cancelURL:  r.FormValue("cancel_url"),
				bac:        r.FormValue("billing_address_collection"),
				trialDays:  r.FormValue("subscription_data[trial_period_days]"),
			})
			mu.Unlock()
			jsonEncode(t, w, msa{
				"URL": "http://co.com/123",
			})
		default:
			t.Errorf("UNEXPECTED: %s %s", r.Method, r.URL.Path)
		}
	})

	s := httptest.NewServer(h)
	t.Cleanup(s.Close)

	cc := &Client{
		Logf: t.Logf,
		Stripe: &stripe.Client{
			BaseURL: s.URL,
		},
	}

	TF := []bool{true, false}
	for _, withAddress := range TF {
		for _, withFeatures := range TF {
			for _, withCancel := range TF {
				for _, withTrial := range TF {
					got = nil

					var (
						bac       = values.ReturnIf(withAddress, "required")
						cancelURL = values.ReturnIf(withCancel, "https://c.com")
						features  = values.ReturnIf(withFeatures, []Feature{{}})
						trialDays = values.ReturnIf(withTrial, 14)
					)

					link, err := cc.Checkout(context.Background(), "org:demo", "http://s.com", &CheckoutParams{
						Features:              features,
						RequireBillingAddress: withAddress,
						CancelURL:             cancelURL,
						TrialDays:             trialDays,
					})
					if err != nil {
						t.Fatal(err)
					}

					if want := "http://co.com/123"; link != want {
						t.Errorf("link = %q; want %q", link, want)
					}

					diff.Test(t, t.Errorf, got, []G{{
						successURL: "http://s.com",
						cancelURL:  cancelURL,
						bac:        bac,
						trialDays:  values.ReturnIf(withTrial && withFeatures, strconv.Itoa(trialDays)),
					}})
				}
			}
		}
	}
}

func TestLookupPhasesNoSchedule(t *testing.T) {
	// TODO(bmizerany): This tests assumptions, but we need an integration
	// test provin fields "like" trial actually fall off / go to zero when
	// the trial is over. This needs a test clock with stripe.
	newHandler := func(s string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case wants(r, "GET", "/v1/customers"):
				writeHuJSON(w, `
					{"data": [
						{
							"metadata": {"tier.org": "org:test"},
							"id":       "cus_test",
						},
					]}
				`)
			case wants(r, "GET", "/v1/subscriptions"):
				writeHuJSON(w, `
					{"data": [
						{
							%s
							"metadata": {
								"tier.subscription": "default",
							},
							"items": {"data": [{"price": {
								"metadata": {"tier.feature": "feature:x@0"},
							}}]},
						},
					]}
				`, s)
			default:
				panic(fmt.Errorf("unknown request: %s %s", r.Method, r.URL.Path))
			}
		})
	}

	fs := []refs.FeaturePlan{
		mpf("feature:x@0"),
	}

	cases := []struct {
		s    string
		want []Phase
	}{
		{
			s: `
				"start_date": 123123123,
			`,
			want: []Phase{{
				Org:       "org:test",
				Effective: time.Unix(123123123, 0),
				Current:   true,
				Trial:     false,
				Features:  fs,
			}},
		},
		{
			s: `
				"start_date": 123123123,
				"cancel_at": 223123123,
			`,
			want: []Phase{{
				Org:       "org:test",
				Effective: time.Unix(123123123, 0),
				Current:   true,
				Features:  fs,
			}, {
				Org:       "org:test",
				Effective: time.Unix(223123123, 0),
			}},
		},
		{
			s: `
				"status": "active",
				"start_date": 100000000,
				"trial_end": 200000000,
				"cancel_at": 400000000,
			`,
			want: []Phase{{
				Org:       "org:test",
				Effective: time.Unix(100000000, 0),
				Current:   false,
				Trial:     true,
				Features:  fs,
			}, {
				Org:       "org:test",
				Effective: time.Unix(200000000, 0),
				Features:  fs,
				Current:   true, // <---- current
				Trial:     false,
			}, {
				Org:       "org:test",
				Effective: time.Unix(400000000, 0),
				Features:  nil, // cancel plan
				Current:   false,
				Trial:     false,
			}},
		},
		{
			s: `
				"status": "trialing",
				"start_date": 100000000,
				"trial_end": 200000000,
			`,
			want: []Phase{{
				Org:       "org:test",
				Effective: time.Unix(100000000, 0),
				Current:   true,
				Trial:     true,
				Features:  fs,
			}, {
				Org:       "org:test",
				Effective: time.Unix(200000000, 0),
				Features:  fs,
			}},
		},
		{
			s: `
				// no cancel_at but having status "canceled" will add a cancel phase
				"status": "canceled",
				"start_date": 100000000,
				"trial_end": 200000000,
				"canceled_at": 400000000,
			`,
			want: []Phase{{
				Org:       "org:test",
				Effective: time.Unix(100000000, 0),
				Current:   false,
				Trial:     true,
				Features:  fs,
			}, {
				Org:       "org:test",
				Effective: time.Unix(200000000, 0),
				Features:  fs,
			}, {
				Org:       "org:test",
				Effective: time.Unix(400000000, 0),
				Features:  nil,
				Current:   true,
			}},
		},
	}

	for _, tt := range cases {
		t.Run("", func(t *testing.T) {
			s := httptest.NewServer(newHandler(tt.s))
			t.Cleanup(s.Close)

			cc := &Client{
				Logf: t.Logf,
				Stripe: &stripe.Client{
					BaseURL: s.URL,
				},
			}

			ctx := context.Background()
			got, err := cc.LookupPhases(ctx, "org:test")
			if err != nil {
				t.Fatal(err)
			}

			diff.Test(t, t.Errorf, got, tt.want)
		})
	}
}

func TestReportUsage(t *testing.T) {
	fs := []Feature{
		{
			FeaturePlan: mpf("feature:10@plan:test@0"),
			Interval:    "@monthly",
			Currency:    "usd",
			Tiers:       []Tier{{Upto: 10}},
			Mode:        "graduated",
			Aggregate:   "sum",
		},
		{
			FeaturePlan: mpf("feature:inf@plan:test@0"),
			Interval:    "@monthly",
			Currency:    "usd",
			Tiers:       []Tier{{Upto: Inf}},
			Mode:        "graduated",
			Aggregate:   "sum",
		},
		{
			FeaturePlan: mpf("feature:lic@plan:test@0"),
			Interval:    "@monthly",
			Currency:    "usd",
		},
	}

	s := newScheduleTester(t)
	s.push(fs)

	s.schedule("org:example", 0, "", FeaturePlans(fs)...)

	g, groupCtx := errgroup.WithContext(s.ctx)
	report := func(feature string, n int) {
		fn, err := refs.ParseName(feature)
		if err != nil {
			t.Fatal(err)
		}
		g.Go(func() (err error) {
			defer errorfmt.Handlef("%s: %w", feature, &err)
			return s.cc.ReportUsage(groupCtx, "org:example", fn, Report{
				N: n,
			})
		})
	}

	report("feature:10", 3)
	report("feature:inf", 9)
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}

	s.checkLimits("org:example", []Usage{
		{Feature: mpf("feature:10@plan:test@0"), Start: t0, End: endOfStripeMonth(t0), Used: 3, Limit: 10},
		{Feature: mpf("feature:inf@plan:test@0"), Start: t0, End: endOfStripeMonth(t0), Used: 9, Limit: Inf},
		{Feature: mpf("feature:lic@plan:test@0"), Start: t1, End: t2, Used: 1, Limit: Inf},
	})
}

func TestReportUsageFeatureNotFound(t *testing.T) {
	tc := newTestClient(t)
	ctx := context.Background()

	fs := []Feature{{
		FeaturePlan: mpf("feature:inf@plan:test@0"),
		Interval:    "@monthly",
		Currency:    "usd",
		Tiers:       []Tier{{Upto: Inf}},
		Mode:        "graduated",
		Aggregate:   "sum",
	}}

	tc.Push(ctx, fs, pushLogger(t))
	if err := tc.SubscribeTo(ctx, "org:example", FeaturePlans(fs)); err != nil {
		t.Fatal(err)
	}
	fn := mpn("feature:nope")
	got := tc.ReportUsage(ctx, "org:example", fn, Report{})
	if !errors.Is(got, ErrFeatureNotFound) {
		t.Fatalf("got %v, want %v", got, ErrFeatureNotFound)
	}
}

func TestSubscribeToUnknownFeatures(t *testing.T) {
	tc := newTestClient(t)
	ctx := context.Background()

	fs := refs.MustParseFeaturePlans(
		"feature:A@plan:a@0",
		"feature:B@plan:b@0",
	)

	got := tc.SubscribeTo(ctx, "org:example", fs)
	if !errors.Is(got, ErrFeatureNotFound) {
		t.Fatalf("got %v, want %v", got, ErrFeatureNotFound)
	}

	// make only plan:a valid
	tc.Push(ctx, []Feature{{
		FeaturePlan: mpf("feature:A@plan:a@0"),
		Interval:    "@monthly",
		Currency:    "usd",
		Tiers:       []Tier{{Upto: Inf}},
		Mode:        "graduated",
		Aggregate:   "sum",
	}}, pushLogger(t))

	got = tc.SubscribeTo(ctx, "org:example", fs)
	if !errors.Is(got, ErrFeatureNotFound) {
		t.Fatalf("got %v, want %v", got, ErrFeatureNotFound)
	}
}

func TestSchedulePutCustomer(t *testing.T) {
	// TODO(bmizerany): convert this all to table tests
	tc := newTestClient(t)
	ctx := context.Background()

	type o = OrgInfo
	copyOrg := new(o)

	check := func(org string, in, want *OrgInfo, wantPutErr, wantLookupErr error) {
		t.Helper()
		err := tc.PutCustomer(ctx, org, in)
		if !errors.Is(err, wantPutErr) {
			t.Fatalf("got %v, want %v", err, wantPutErr)
		}
		got, err := tc.LookupOrg(ctx, org)
		if !errors.Is(err, wantLookupErr) {
			t.Fatalf("got %v, want %v", err, wantLookupErr)
		}
		if want == copyOrg {
			want = in
			if want.Metadata == nil {
				want.Metadata = map[string]string{}
			}
		}
		diff.Test(t, t.Errorf, got, want, diff.ZeroFields[OrgInfo]("Created"))
	}

	check("org:invalid", &o{Email: "invalid"}, nil, ErrInvalidEmail, ErrOrgNotFound)

	check("org:a", &o{Email: "a@a.com"}, copyOrg, nil, nil)
	check("org:b", &o{Email: "b@b.com"}, copyOrg, nil, nil)
	check("org:a", &o{Email: "aa@aa.com"}, copyOrg, nil, nil)

	c0 := &o{
		Email: "c@c.com",
		Metadata: map[string]string{
			"foo": "bar",
		},
	}

	check("org:c", c0, copyOrg, nil, nil)
	check("org:c", &o{
		Email: "do@notUpdate.com",
		Metadata: map[string]string{
			"foo":      "XXXX",
			"tier.baz": "qux", // should cause aborted update
		},
	}, c0, ErrInvalidMetadata, nil)

	check("org:c", &o{
		Email: "c1@c.com",
		Metadata: map[string]string{
			"foo": "bar",
			"c":   "ccc",
		},
	}, copyOrg, nil, nil)

	check("org:c", &o{
		Email: "c1@c.com",
		Metadata: map[string]string{
			"foo": "",
		},
	}, &o{
		Email: "c1@c.com",
		Metadata: map[string]string{
			// "foo" is removed
			"c": "ccc",
		},
	}, nil, nil)

	check("org:c", &o{
		Email:       "c1@c.com",
		Name:        "The Name",
		Description: "The Desc",
		Phone:       "111-111-1111",
		Metadata: map[string]string{
			"c": "ccc",
		},
	}, &o{
		Email:       "c1@c.com",
		Name:        "The Name",
		Description: "The Desc",
		Phone:       "111-111-1111",
		Metadata: map[string]string{
			"c": "ccc",
		},
	}, nil, nil)

	check("org:c", &o{
		// nothing to update; nop
	}, &o{
		Email:       "c1@c.com",
		Name:        "The Name",
		Description: "The Desc",
		Phone:       "111-111-1111",
		Metadata: map[string]string{
			"c": "ccc",
		},
	}, nil, nil)
}

func ciOnly(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("not in CI; skipping long test")
	}
}

func endOfStripeMonth(t time.Time) time.Time {
	return t.AddDate(0, 1, 0).Truncate(time.Minute).Add(-5 * time.Minute)
}

func plans(ss ...string) []refs.Plan {
	var ps []refs.Plan
	for _, s := range ss {
		ps = append(ps, mpp(s))
	}
	return ps
}

func wants(r *http.Request, method, pattern string) bool {
	pattern = "^" + pattern + "$"
	rx := regexp.MustCompile(pattern)
	return r.Method == method && rx.MatchString(r.URL.Path)
}

func writeHuJSON(w io.Writer, s string, args ...any) {
	s = fmt.Sprintf(s, args...)
	b, err := hujson.Standardize([]byte(s))
	if err != nil {
		panic(err)
	}
	_, err = w.Write(b)
	if err != nil {
		panic(err)
	}
}

type msa map[string]any

func jsonEncode(t *testing.T, w io.Writer, v any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("jsonEncode: %v", err)
	}
}

func jsonEncodeList[T any](t *testing.T, w io.Writer, l ...T) {
	t.Helper()
	var v = struct {
		Data []T `json:"data"`
	}{
		Data: l,
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("jsonEncode: %v", err)
	}
}

type List[T any] struct {
	mu sync.Mutex
	a  []T
}

func (l *List[T]) Append(v T) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.a = append(l.a, v)
}

func (l *List[T]) Slice() []T {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.a)
}
