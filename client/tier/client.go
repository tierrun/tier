package tier

// cmd/tier/serve.go|18 col 22| h := api.NewHandler(tc(), vlogf)
// cmd/tier/tier.go|186 col 14| fs, err := tc().Pull(ctx, 0)
// cmd/tier/tier.go|200 col 14| fs, err := tc().Pull(ctx, 0)
// cmd/tier/tier.go|241 col 10| return tc().SubscribeToRefs(ctx, org, refs)
// cmd/tier/tier.go|247 col 14| ps, err := tc().LookupPhases(ctx, org)
// cmd/tier/tier.go|287 col 15| use, err := tc().LookupLimits(ctx, org)
// cmd/tier/tier.go|321 col 10| return tc().ReportUsage(ctx, org, fn, tier.Report{
// cmd/tier/tier.go|331 col 15| cid, err := tc().WhoIs(ctx, org)
// cmd/tier/tier.go|361 col 6| func tc() *tier.Client {
// cmd/tier/tier.go|434 col 2| tc().Push(ctx, fs, cb)
// cmd/tier/tier.go|439 col 36| link, err := url.JoinPath(dashURL[tc().Live()], "products", f.ID())

const Inf = 1<<63 - 1

type Client struct {
}

func (c *Client) Pull()            {}
func (c *Client) Push()            {}
func (c *Client) WhoIs()           {}
func (c *Client) LookupPhases()    {}
func (c *Client) LookupLimits()    {}
func (c *Client) ReportUsage()     {}
func (c *Client) SubscribeToRefs() {}
func (c *Client) Live()            {}
