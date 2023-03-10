// Package tier contains the local client for interacting with the tier sidecar
// API.
//
// For more information, please see https://tier.run/docs.
package tier

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"time"

	"tailscale.com/logtail/backoff"
	"tier.run/api/apitypes"
	"tier.run/fetch"
	"tier.run/refs"
	"tier.run/types/tax"
)

// ClockHeader is the header used to pass the clock ID to the tier sidecar.
// It is exported for use by the sidecar API. Most users want to use WithClock.
const ClockHeader = "Tier-Clock"

const Inf = 1<<63 - 1

type Client struct {
	// APIKey is the API key used, and set in the Authorization header.
	APIKey string

	BaseURL    string // the base URL of the tier sidecar; default is http://127.0.0.1:8080
	HTTPClient *http.Client

	Logf func(fmt string, args ...any)
}

func (c *Client) logf(fmt string, args ...any) {
	if c.Logf != nil {
		c.Logf(fmt, args...)
	}
}

type clockKey struct{}

// WithClock returns a context with the provided clock ID set. The clock ID is
// pass via the Tier-Clock header to be used by the tier sidecar.
func WithClock(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, clockKey{}, id)
}

func clockFromContext(ctx context.Context) string {
	id, _ := ctx.Value(clockKey{}).(string)
	return id
}

// FromEnv returns a Client configured from the environment. The BaseURL is set
// to the value of the TIER_BASE_URL environment variable, or
// http://127.0.0.1:8080 if unset.
//
// It returns an error if the TIER_BASE_URL environment variable is set to an
// invalid URL.
func FromEnv() (*Client, error) {
	key := os.Getenv("TIER_API_KEY")
	baseURL := os.Getenv("TIER_BASE_URL")
	if key != "" {
		if baseURL == "" {
			baseURL = "https://api.tier.run"
		}
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	_, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	return &Client{BaseURL: baseURL, APIKey: key}, nil
}

const defaultBaseURL = "http://127.0.0.1:8080"

func (c *Client) baseURL(pathStartingWithSlash string) string {
	return c.BaseURL + pathStartingWithSlash
}

func (c *Client) client() *http.Client {
	if c.HTTPClient == nil {
		return http.DefaultClient
	}
	return c.HTTPClient
}

// Push pushes the provided pricing model to Stripe.
func (c *Client) Push(ctx context.Context, m apitypes.Model) (apitypes.PushResponse, error) {
	return fetchOK[apitypes.PushResponse, *apitypes.Error](ctx, c, "POST", "/v1/push", m)
}

func (c *Client) PushJSON(ctx context.Context, m []byte) (apitypes.PushResponse, error) {
	return fetchOK[apitypes.PushResponse, *apitypes.Error](ctx, c, "POST", "/v1/push", json.RawMessage(m))
}

// Pull fetches the complete pricing model from Stripe.
func (c *Client) Pull(ctx context.Context) (apitypes.Model, error) {
	return fetchOK[apitypes.Model, *apitypes.Error](ctx, c, "GET", "/v1/pull", nil)
}

// PullJSON fetches the complete pricing model from Stripe and returns the raw
// JSON response.
func (c *Client) PullJSON(ctx context.Context) ([]byte, error) {
	return fetchOK[[]byte, *apitypes.Error](ctx, c, "GET", "/v1/pull", nil)
}

// WhoIs reports the Stripe customer ID for the provided org. OrgInfo is not set.
func (c *Client) WhoIs(ctx context.Context, org string) (apitypes.WhoIsResponse, error) {
	return fetchOK[apitypes.WhoIsResponse, *apitypes.Error](ctx, c, "GET", "/v1/whois?org="+org, nil)
}

// LookupOrg reports all known information about the provided org. The
// information is not cached by the server. If only the Stripe customer ID is
// needed and speed is of concern, users should use WhoIs.
func (c *Client) LookupOrg(ctx context.Context, org string) (apitypes.WhoIsResponse, error) {
	return fetchOK[apitypes.WhoIsResponse, *apitypes.Error](ctx, c, "GET", "/v1/whois?include=info&org="+org, nil)
}

// LookupPhase reports information about the current phase the provided org is scheduled in.
func (c *Client) LookupPhase(ctx context.Context, org string) (apitypes.PhaseResponse, error) {
	return fetchOK[apitypes.PhaseResponse, *apitypes.Error](ctx, c, "GET", "/v1/phase?org="+org, nil)
}

// LookupLimits reports the current usage and limits for the provided org.
func (c *Client) LookupLimits(ctx context.Context, org string) (apitypes.UsageResponse, error) {
	return fetchOK[apitypes.UsageResponse, *apitypes.Error](ctx, c, "GET", "/v1/limits?org="+org, nil)
}

func (c *Client) LookupPaymentMethods(ctx context.Context, org string) (apitypes.PaymentMethodsResponse, error) {
	return fetchOK[apitypes.PaymentMethodsResponse, *apitypes.Error](ctx, c, "GET", "/v1/payment_methods?org="+org, nil)
}

// LookupLimit reports the current usage and limits for the provided org and
// feature. If the feature is not currently available to the org, both limit
// and used are zero and no error is reported.
//
// It reports an error if any.
func (c *Client) LookupLimit(ctx context.Context, org, feature string) (limit, used int, err error) {
	fn, err := refs.ParseName(feature)
	if err != nil {
		return 0, 0, err
	}
	limits, err := c.LookupLimits(ctx, org)
	if err != nil {
		return 0, 0, err
	}
	for _, u := range limits.Usage {
		if u.Feature == fn {
			return u.Limit, u.Used, nil
		}
	}
	return 0, 0, nil
}

// An Answer is the response to any question for Can. It can be used in a few
// forms to shorten the logic necessary to know if a program should proceed to
// perform a user request based on their entitlements.
type Answer struct {
	ok     bool
	err    error
	report func(n int) error
}

// OK reports if the program should proceed with a user request or not. To
// prevent total failure if Can needed to reach the sidecar and was unable to,
// OK will fail optimistically and report true. If the opposite is desired,
// clients can check Err.
func (c Answer) OK() bool { return c.ok }

// Err returns the error, if any, that occurred during the call to Can.
func (c Answer) Err() error { return c.err }

// Report is the same as calling ReportN(1).
func (c Answer) Report() error { return c.ReportN(1) }

// ReportN reports usage of n units for the feature and org provided to Can.
func (c Answer) ReportN(n int) error {
	if c.report != nil {
		return c.report(n)
	}
	return nil
}

// Can is a convenience function for checking if an org has used more of a
// feature than they are entitled to and optionally reporting usage post check
// and consumption.
//
// If reporting consumption is not required, it can be used in the form:
//
//	if c.Can(ctx, "org:acme", "feature:convert").OK() { ... }
//
// reporting usage post consumption looks like:
//
//	ans := c.Can(ctx, "org:acme", "feature:convert")
//	if !ans.OK() {
//	  return ""
//	}
//	defer ans.Report() // or ReportN
//	return convert(temp)
func (c *Client) Can(ctx context.Context, org, feature string) Answer {
	limit, used, err := c.LookupLimit(ctx, org, feature)
	if err != nil {
		// TODO(bmizerany): caching of usage and limits in imminent and
		// the cache can be consulted before failing to "allow by
		// default", but for now simply allow by default right away.
		return Answer{ok: true, err: err}
	}
	if used >= limit {
		return Answer{}
	}
	report := func(n int) error {
		return c.Report(ctx, org, feature, 1)
	}
	return Answer{ok: true, report: report}
}

// Report reports a usage of n for the provided org and feature at the current
// time.
func (c *Client) Report(ctx context.Context, org, feature string, n int) error {
	fn, err := refs.ParseName(feature)
	if err != nil {
		return err
	}
	_, err = fetchOK[struct{}, *apitypes.Error](ctx, c, "POST", "/v1/report", apitypes.ReportRequest{
		Org:     org,
		Feature: fn,
		N:       n,
	})

	return err
}

type ReportParams struct {
	At      time.Time // default is 'now' at Stripe
	Clobber bool      // default is false
}

// ReportUsage reports usage based on the provided ReportRequest fields.
func (c *Client) ReportUsage(ctx context.Context, org, feature string, n int, rp *ReportParams) error {
	var p ReportParams
	if rp != nil {
		p = *rp
	}
	fn, err := refs.ParseName(feature)
	if err != nil {
		return err
	}
	r := apitypes.ReportRequest{
		Org:     org,
		Feature: fn,
		N:       n,
		At:      p.At,
		Clobber: p.Clobber,
	}
	_, err = fetchOK[struct{}, *apitypes.Error](ctx, c, "POST", "/v1/report", r)
	return err
}

// Subscribe subscribes the provided org to the provided feature or plan,
// effective immediately.
//
// Any in-progress scheduled is overwritten and the customer is billed with
// prorations immediately.
func (c *Client) Subscribe(ctx context.Context, org string, featuresAndPlans ...string) error {
	_, err := fetchOK[struct{}, *apitypes.Error](ctx, c, "POST", "/v1/subscribe", apitypes.ScheduleRequest{
		Org:    org,
		Phases: []apitypes.Phase{{Features: featuresAndPlans}},
	})
	return err
}

// Cancel cancels the subscription for the provided org.
func (c *Client) Cancel(ctx context.Context, org string) error {
	return c.Subscribe(ctx, org)
}

// Checkout creates a new checkout link for the provided org and features, if
// any; otherwise, if no features are specified, and payment setup link is
// returned instead.
func (c *Client) Checkout(ctx context.Context, org string, successURL string, p *CheckoutParams) (*apitypes.CheckoutResponse, error) {
	if p == nil {
		p = &CheckoutParams{}
	}
	r := &apitypes.CheckoutRequest{
		Org:                   org,
		SuccessURL:            successURL,
		CancelURL:             p.CancelURL,
		TrialDays:             p.TrialDays,
		Features:              p.Features,
		RequireBillingAddress: p.RequireBillingAddress,
	}
	return fetchOK[*apitypes.CheckoutResponse, *apitypes.Error](ctx, c, "POST", "/v1/checkout", r)
}

type Phase = apitypes.Phase
type OrgInfo = apitypes.OrgInfo

type CheckoutParams struct {
	TrialDays             int
	Features              []string
	CancelURL             string
	RequireBillingAddress bool
}

type ScheduleParams struct {
	Info            *OrgInfo
	Phases          []Phase
	PaymentMethodID string

	Tax tax.Applied
}

func (c *Client) Schedule(ctx context.Context, org string, p *ScheduleParams) (*apitypes.ScheduleResponse, error) {
	return fetchOK[*apitypes.ScheduleResponse, *apitypes.Error](ctx, c, "POST", "/v1/subscribe", &apitypes.ScheduleRequest{
		Org:             org,
		Info:            (*apitypes.OrgInfo)(p.Info),
		Phases:          p.Phases,
		PaymentMethodID: p.PaymentMethodID,
		Tax:             p.Tax,
	})

}

func (c *Client) WhoAmI(ctx context.Context) (apitypes.WhoAmIResponse, error) {
	return fetchOK[apitypes.WhoAmIResponse, *apitypes.Error](ctx, c, "GET", "/v1/whoami", nil)
}

// WithClock creates a new test clock with the provided name and start time,
// and returns a new context with the clock ID set.
//
// It is an error to call WithClock if a clock is already set in the context.
func (c *Client) WithClock(ctx context.Context, name string, start time.Time) (context.Context, error) {
	if clockFromContext(ctx) != "" {
		return nil, errors.New("tier: clock already set in context")
	}
	clock, err := fetchOK[apitypes.ClockResponse, *apitypes.Error](ctx, c, "POST", "/v1/clock", apitypes.ClockRequest{
		Name:    name,
		Present: start,
	})
	if err != nil {
		return nil, err
	}
	return WithClock(ctx, clock.ID), nil
}

// Advance advances the test clock set in the context to t.
//
// It is an error to call Advance if no clock is set in the context.
func (c *Client) Advance(ctx context.Context, t time.Time) error {
	clockID := clockFromContext(ctx)
	if clockID == "" {
		return errors.New("tier: no clock set in context")
	}
	cr, err := fetchOK[apitypes.ClockResponse, *apitypes.Error](ctx, c, "POST", "/v1/clock", apitypes.ClockRequest{
		ID:      clockID,
		Present: t,
	})
	if err != nil {
		return err
	}
	return c.awaitClockReady(ctx, cr.ID)
}

var errClockNotReady = errors.New("force backoff")

func (c *Client) awaitClockReady(ctx context.Context, id string) error {
	bo := backoff.NewBackoff("tier", c.logf, 5*time.Second)
	for {
		cr, err := c.syncClock(ctx, id)
		if err != nil || cr.Status != "ready" {
			c.logf("clock %s not ready: err=%v status=%q; retrying", id, err, cr.Status)
			bo.BackOff(ctx, errClockNotReady)
			continue
		}
		return nil
	}
}

func (c *Client) syncClock(ctx context.Context, id string) (apitypes.ClockResponse, error) {
	return fetchOK[apitypes.ClockResponse, *apitypes.Error](ctx, c, "GET", "/v1/clock?id="+id, nil)
}

func fetchOK[T any, E error](ctx context.Context, c *Client, method, path string, body any) (T, error) {
	up := url.UserPassword(c.APIKey, "")
	clockID := clockFromContext(ctx)
	var h http.Header
	if clockID != "" {
		h = http.Header{ClockHeader: []string{clockID}}
	}
	return fetch.OK[T, E](ctx, c.client(), method, c.baseURL(path), body, up, h)
}
