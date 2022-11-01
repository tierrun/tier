package tier

import (
	"context"
	"net/http"

	"tier.run/api/apitypes"
	"tier.run/refs"
)

const Inf = 1<<63 - 1

type Client struct {
	HTTPClient *http.Client
}

func (c *Client) Pull(ctx context.Context) (apitypes.Model, error) {
	panic("TODO")
}

func (c *Client) PullJSON(ctx context.Context) ([]byte, error) {
	panic("TODO")
}

func (c *Client) Push(ctx context.Context, m apitypes.Model, cb func(apitypes.Feature, error)) error {
	panic("TODO")
}

func (c *Client) WhoIs(ctx context.Context, org string) (string, error) {
	panic("TODO")
}

func (c *Client) LookupPhase(ctx context.Context, org string) (apitypes.PhaseResponse, error) {
	panic("TODO")
}

func (c *Client) LookupLimits(ctx context.Context, org string) (apitypes.UsageResponse, error) {
	panic("TODO")
}

func (c *Client) ReportUsage(ctx context.Context, org string, f refs.Name, r apitypes.ReportRequest) error {
	panic("TODO")
}

func (c *Client) SubscribeToRefs(ctx context.Context, org string, refs []string) error {
	panic("TODO")
}
