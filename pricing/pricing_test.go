package pricing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stripe/stripe-go/v72"
	"github.com/tierrun/tierx/pricing/schema"
	"golang.org/x/sync/errgroup"
	"kr.dev/diff"
)

var cleanup errgroup.Group

func TestMain(m *testing.M) {
	code := m.Run()
	if err := cleanup.Wait(); err != nil {
		panic(err)
	}
	os.Exit(code)
}

func newTester(t *testing.T) *Client {
	t.Helper()

	tc, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(tc.StripeKey, "sk_test_") {
		panic("stripe key must be a test key")
	}

	tc.Logf = t.Logf

	tr := &setAcctTransport{}
	tc.HTTPClient = &http.Client{
		Transport: tr,
	}
	tc.init()

	ctx := context.Background()
	a, err := tc.sc.Account.New(&stripe.AccountParams{
		Params: stripe.Params{Context: ctx},
		Email:  ptr("ci@tier.run"),
		Type:   ptr("standard"),
	})
	if err != nil {
		t.Fatal(err)
	}

	// all subsequent requests will set the Stripe-Account header a.ID
	tr.accountID = a.ID

	t.Cleanup(func() {
		cleanup.Go(func() error {
			_, err := tc.sc.Account.Del(a.ID, &stripe.AccountParams{
				Params: stripe.Params{Context: ctx},
			})
			return err
		})
	})

	return tc
}

type setAcctTransport struct {
	accountID string
}

func (t *setAcctTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.accountID != "" {
		newReq := r.Clone(r.Context())
		newReq.Header.Set("Stripe-Account", t.accountID)
		r = newReq
	}
	return http.DefaultTransport.RoundTrip(r)
}

func TestPush(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		want    string
		wantErr error
	}{
		{
			name: "zero plans",
			in:   `{"plans": {}}`,
			want: `{"plans": {}}`,
		},
		{
			name: "feature default to standard pricing",
			in: `
				{"plans": {"plan:test@0": {"features": {"feature:test": {}}}}}
			`,
			want: `
				{"plans": {"plan:test@0": {"features": {"feature:test": {
					"type": "licensed",
					"interval": "@monthly",
					"currency": "usd",
				}}}}}
			`,
		},
		{
			name: "single tier no limit specified",
			in: `
				{"plans": {"plan:test@0": {"features": {"feature:test": {
					"tiers": [{}],
				}}}}}
			`,
			want: `
				{"plans": {"plan:test@0": {"features": {"feature:test": {
					"mode": "graduated",
					"aggregate": "sum",
					"interval": "@monthly",
					"currency": "usd",
					"tiers": [{}],
				}}}}}
			`,
		},
		{
			name: "single tier limit",
			in: `
				{"plans": {"plan:test@0": {"features": {"feature:test": {
					"tiers": [{"upto": 100, "price": 1, "base": 0}],
				}}}}}
			`,
			want: `
				{"plans": {"plan:test@0": {"features": {"feature:test": {
					"mode": "graduated",
					"aggregate": "sum",
					"interval": "@monthly",
					"currency": "usd",
					"tiers": [{"upto": 100, "price": 1, "base": 0}],
				}}}}}
			`,
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tc := newTester(t)
			r := strings.NewReader(tt.in)
			report := func(plan, feature string, err error) {
				t.Logf("result: %s %s %v", plan, feature, err)
			}
			if err := tc.PushJSON(context.Background(), r, report); !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
			got, err := tc.Pull(context.Background())
			if err != tt.wantErr {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
			want, err := Unmarshal(tt.want)
			if err != nil {
				t.Fatal(err)
			}
			diff.Test(t, t.Errorf, got, want,
				diff.ZeroFields[schema.Feature]("ProviderID", "Plan"),
				diff.ZeroFields[schema.Plan]("ProviderID"))
		})
	}
}

func TestPushExistingPlanAndFeatures(t *testing.T) {
	t.Parallel()

	tc := newTester(t)
	ctx := context.Background()

	var got recorder
	checkPush := func(want error, pj string) {
		t.Helper()
		err := tc.PushJSON(ctx, strings.NewReader(pj), got.Report)
		if !errors.Is(err, want) {
			t.Errorf("err = %v, want %v", err, want)
		}
	}

	checkPush(nil, `{"plans": {"plan:test@0": {"features": {"feature:test": {}}}}}`)
	checkPush(nil, `{"plans": {"plan:test@0": {"features": {"feature:test": {}}}}}`)
	checkPush(nil, `{"plans": {"plan:test@0": {"features": {"feature:test": {}}}}}`)
	checkPush(nil, `{"plans": {"plan:test@0": {"features": {"feature:xxxx": {}}}}}`)

	want := recorder{
		"plan:test@0 - created",
		"plan:test@0 feature:test created",
		"plan:test@0 - plan already exists",
		"plan:test@0 feature:test feature already exists",
		"plan:test@0 - plan already exists",
		"plan:test@0 feature:test feature already exists",
		"plan:test@0 - plan already exists",
		"plan:test@0 feature:xxxx created",
	}

	diff.Test(t, t.Errorf, got, want)
}

type recorder []string

func (r *recorder) Report(plan, feature string, err error) {
	if feature == "" {
		feature = "-"
	}
	errStr := "created"
	if err != nil {
		errStr = err.Error()
	}
	*r = append(*r, fmt.Sprintf("%s %s %s", plan, feature, errStr))
}
