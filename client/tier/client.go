// Package tier contains the local client for interacting with the tier sidecar
// API.
//
// For more information, please see https://tier.run/docs.
package tier

import (
	"context"
	"net/http"
	"time"

	"tier.run/api/apitypes"
	"tier.run/fetch"
	"tier.run/refs"
	"tier.run/trweb"
)

const Inf = 1<<63 - 1

type Client struct {
	HTTPClient *http.Client
}

func (c *Client) client() *http.Client {
	if c.HTTPClient == nil {
		return http.DefaultClient
	}
	return c.HTTPClient
}

// Pull fetches the complete pricing model from Stripe.
func (c *Client) Pull(ctx context.Context) (apitypes.Model, error) {
	return fetch.OK[apitypes.Model, *trweb.HTTPError](ctx, c.client(), "GET", "/v1/pull", nil)
}

// PullJSON fetches the complete pricing model from Stripe and returns the raw
// JSON response.
func (c *Client) PullJSON(ctx context.Context) ([]byte, error) {
	return fetch.OK[[]byte, *trweb.HTTPError](ctx, c.client(), "GET", "/v1/pull", nil)
}

// WhoIS reports the Stripe ID for the given organization.
func (c *Client) WhoIs(ctx context.Context, org string) (apitypes.WhoIsResponse, error) {
	return fetch.OK[apitypes.WhoIsResponse, *trweb.HTTPError](ctx, c.client(), "GET", "/v1/whois?org="+org, nil)
}

// LookupPhase reports information about the current phase the provided org is scheduled in.
func (c *Client) LookupPhase(ctx context.Context, org string) (apitypes.PhaseResponse, error) {
	return fetch.OK[apitypes.PhaseResponse, *trweb.HTTPError](ctx, c.client(), "GET", "/v1/phase?org="+org, nil)
}

// LookupLimits reports the current usage and limits for the provided org.
func (c *Client) LookupLimits(ctx context.Context, org string) (apitypes.UsageResponse, error) {
	return fetch.OK[apitypes.UsageResponse, *trweb.HTTPError](ctx, c.client(), "GET", "/v1/limits?org="+org, nil)
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

type Check struct {
	ok     bool
	err    error
	report func(n int) error
}

func (c Check) OK() bool      { return c.ok }
func (c Check) Err() error    { return c.err }
func (c Check) Report() error { return c.ReportN(1) }

func (c Check) ReportN(n int) error {
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
//  if c.Can(ctx, "org:acme", "feature:convert").OK() { ... }
//
// reporting usage post consumption looks like:
//
//  ans := c.Can(ctx, "org:acme", "feature:convert")
//  if !ans.OK() {
//    return ""
//  }
//  defer ans.Report() // or ReportN
//  return convert(temp)
//
func (c *Client) Can(ctx context.Context, org, feature string) Check {
	limit, used, err := c.LookupLimit(ctx, org, feature)
	if err != nil {
		// TODO(bmizerany): caching of usage and limits in imminent and
		// the cache can be consulted before failing to "allow by
		// default", but for now simply allow by default right away.
		return Check{ok: true, err: err}
	}
	if used >= limit {
		return Check{}
	}
	report := func(n int) error {
		return c.Report(ctx, org, feature, 1)
	}
	return Check{ok: true, report: report}
}

// Report reports a usage of n for the provided org and feature at the current
// time.
func (c *Client) Report(ctx context.Context, org, feature string, n int) error {
	fn, err := refs.ParseName(feature)
	if err != nil {
		return err
	}
	_, err = fetch.OK[struct{}, *trweb.HTTPError](ctx, c.client(), "POST", "/v1/report", apitypes.ReportRequest{
		Org:     org,
		Feature: fn,
		N:       n,
		At:      time.Now(),
	})
	return err
}

// ReportUsage reports usage based on the provided ReportRequest fields.
func (c *Client) ReportUsage(ctx context.Context, r apitypes.ReportRequest) error {
	_, err := fetch.OK[struct{}, *trweb.HTTPError](ctx, c.client(), "POST", "/v1/report", r)
	return err
}

// SubscribeNow subscribes the provided org to the provided feature or plan,
// effective immediately.
//
// Any in-progress scheduled is overwritten and the customer is billed with
// prorations immediately.
func (c *Client) SubscribeNow(ctx context.Context, org string, featuresAndPlans ...string) error {
	return c.Subscribe(ctx, org, []apitypes.Phase{{
		Effective: time.Now(),
		Features:  featuresAndPlans,
	}})
}

// Subscribe schedules phases for the provided org.
//
// If no phase is provided that matches any in-progress phase, the in-progress
// schedule is ended and the customer is billed with prorations immediately and
// the new schedule goes into effect.
//
// To amend or modify an in-progress schedule, lookup the current phases with
// LookupPhases, update the list, but be sure to keep any phase with Current
// set to true unmodified, and call Subscribe with the updated list of phases.
func (c *Client) Subscribe(ctx context.Context, org string, phases []apitypes.Phase) error {
	_, err := fetch.OK[struct{}, *trweb.HTTPError](ctx, c.client(), "POST", "/v1/subscribe", apitypes.SubscribeRequest{
		Org:    org,
		Phases: phases,
	})
	return err
}
