package tier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"tier.run/client/tier/apitype"
	"tier.run/fetch"
)

// TODO(bmizerany): send a clientID with each request via Tier-Client-ID header
// TODO(bmizerany): set User-Agent with version

type Client struct {
	Key     string
	BaseURL string

	HTTPClient *http.Client
}

func FromEnv() (*Client, error) {
	c := &Client{
		Key:     os.Getenv("TIER_KEY"),
		BaseURL: os.Getenv("TIER_URL"),
	}
	if c.BaseURL == "" {
		c.BaseURL = "https://api.tier.run"
	}
	if c.Key == "" {
		return nil, errors.New("tier: TIER_KEY must be set")
	}
	return c, nil
}

func (c *Client) ServeStripeOptions(w http.ResponseWriter, r *http.Request, org string) error {
	options, err := c.StripeOptions(r.Context(), org)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(options)
}

func (c *Client) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

type Error struct {
	Status  int
	Code    string
	Message string
}

// IsNotExist reports if the underlying error is an Error with status 404, and code ("not_found").
func IsNotExist(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Status == 404 && e.Code == "not_found"
	}
	return false
}

func (ce *Error) Error() string {
	return fmt.Sprintf("tierClientError{status:%v message:%q code:%q}",
		ce.Status, ce.Message, ce.Code)
}

// Ping pings the tier API at BaseURL and returns nil if successful; otherwise
// an error is returned.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.WhoAmI(ctx)
	return err
}

// WhoAMI returns the identity for the Tier key used by this Client.
func (c *Client) WhoAmI(ctx context.Context) (string, error) {
	s, err := fetchOK[struct{ Org string }](ctx, c, "GET", "/api/v1/whoami", nil)
	return s.Org, err
}

// StripeOptions returns the options required for use with the stripe.js for
// receiving payment information for an org. The returned type serializes to
// json in such a way that it is ready for use with stripe.js in the browser.
func (c *Client) StripeOptions(ctx context.Context, org string, opts ...any) (apitype.StripeOptions, error) {
	return fetchOK[apitype.StripeOptions](ctx, c, "POST", "/api/v1/stripe/options", map[string]any{
		"org": org,
	}, opts...)
}

// AppendPhase appends a new phase to the provided orgs schedule. The newly
// appended phase will go into effect at the provided effective time.
func (c *Client) AppendPhase(ctx context.Context, org, plan string, effective time.Time) error {
	_, err := fetchOK[struct{}](ctx, c, "POST", "/api/v1/append", map[string]any{
		"org":       org,
		"plan":      plan,
		"effective": effective,
	})
	return err
}

func (c *Client) PushModel(ctx context.Context, modelJSON string) (*apitype.PushResult, error) {
	return fetchOK[*apitype.PushResult](ctx, c, "POST", "/api/v1/push", strings.NewReader(modelJSON))
}

func (c *Client) PullModel(ctx context.Context) (*apitype.Model, error) {
	return fetchOK[*apitype.Model](ctx, c, "GET", "/api/v1/pull", nil)
}

type Phase struct {
	Domain    string    `json:"domain"`
	Org       string    `json:"org"`
	Plan      string    `json:"plan"`
	Scheduled time.Time `json:"scheduled"`
	Effective time.Time `json:"effective"`
}

type Schedule struct {
	Phases []*Phase `json:"phases"`
}

func (c *Client) LookupSchedule(ctx context.Context, org string) (*Schedule, error) {
	return fetchOK[*Schedule](ctx, c, "GET", "/api/v1/schedule?org="+org, nil)
}

type Reservation struct {
	used   int
	limit  int
	done   bool
	refund func() error
}

func (rsv *Reservation) OK() bool {
	return rsv.used < rsv.limit
}

func (rsv *Reservation) Commit() {
	rsv.done = true
}

func (rsv *Reservation) Refund() error {
	if rsv.done {
		return nil
	}
	rsv.done = true
	return rsv.refund()
}

func (c *Client) ReserveN(ctx context.Context, now time.Time, org, feature string, n int) (*Reservation, error) {
	_, err := c.UpdateCount(ctx, &apitype.UpdateCount{
		CounterID: "",
		Now:       now.UTC(),
		Org:       org,
		Feature:   feature,
		Op:        "incr",
		P:         n,
	})
	if err != nil {
		return nil, err
	}
	rsv := &Reservation{
		done: false,
		refund: func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			return c.Refund(ctx, now, org, feature, n)
		},
	}
	return rsv, err
}

func (c *Client) Refund(ctx context.Context, now time.Time, org, feature string, n int) error {
	_, err := c.UpdateCount(ctx, &apitype.UpdateCount{
		CounterID: "",
		Now:       now,
		Org:       org,
		Feature:   feature,
		Op:        "decr",
		N:         n,
	})
	return err
}

func (c *Client) UpdateCount(ctx context.Context, up *apitype.UpdateCount) (*apitype.UpdateResponse, error) {
	return fetchOK[*apitype.UpdateResponse](ctx, c, "POST", "/api/v1/reserve", up)
}

func (c *Client) Cannot(ctx context.Context, org, feature string) bool {
	// TODO(bmizerany): log err?
	rsv, err := c.ReserveN(ctx, time.Now(), org, feature, 0)
	if err != nil {
		// TODO(bmizerany): let them through if there is an error? make configurable?
		return false
	}
	return !rsv.OK()
}

func fetchOK[T any](ctx context.Context, c *Client, method, path string, body any, opts ...any) (T, error) {
	opts = append(opts, url.UserPassword(c.Key, ""))
	v, err := fetch.FetchOK[T, *Error](ctx, c.client(), method, c.BaseURL+path, body, opts...)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("tierclient: %s %s: %w", method, path, err)
	}
	return v, nil
}
