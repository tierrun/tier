package tier

import (
	"context"
	"net/http"

	"tier.run/api/apitypes"
	"tier.run/fetch"
	"tier.run/trweb"
)

const Inf = 1<<63 - 1

type Client struct {
	HTTPClient *http.Client
}

func (c *Client) Pull(ctx context.Context) (apitypes.Model, error) {
	return fetch.OK[apitypes.Model, *trweb.HTTPError](ctx, c.HTTPClient, "GET", "/v1/pull", nil)
}

func (c *Client) PullJSON(ctx context.Context) ([]byte, error) {
	return fetch.OK[[]byte, *trweb.HTTPError](ctx, c.HTTPClient, "GET", "/v1/pull", nil)
}

func (c *Client) WhoIs(ctx context.Context, org string) (apitypes.WhoIsResponse, error) {
	return fetch.OK[apitypes.WhoIsResponse, *trweb.HTTPError](ctx, c.HTTPClient, "GET", "/v1/whois?org="+org, nil)
}

func (c *Client) LookupPhase(ctx context.Context, org string) (apitypes.PhaseResponse, error) {
	return fetch.OK[apitypes.PhaseResponse, *trweb.HTTPError](ctx, c.HTTPClient, "GET", "/v1/phase?org="+org, nil)
}

func (c *Client) LookupLimits(ctx context.Context, org string) (apitypes.UsageResponse, error) {
	return fetch.OK[apitypes.UsageResponse, *trweb.HTTPError](ctx, c.HTTPClient, "GET", "/v1/limits?org="+org, nil)
}

func (c *Client) ReportUsage(ctx context.Context, r apitypes.ReportRequest) error {
	_, err := fetch.OK[struct{}, *trweb.HTTPError](ctx, c.HTTPClient, "POST", "/v1/report", r)
	return err
}

func (c *Client) SubscribeToRefs(ctx context.Context, org string, refs []string) error {
	_, err := fetch.OK[struct{}, *trweb.HTTPError](ctx, c.HTTPClient, "POST", "/v1/subscribe", apitypes.SubscribeRequest{
		Org: org,
		Phases: []apitypes.Phase{
			{Features: refs},
		},
	})
	return err
}
