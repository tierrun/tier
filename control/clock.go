package control

import (
	"context"
	"errors"
	"net/url"
	"time"

	"tailscale.com/logtail/backoff"
	"tier.run/stripe"
)

type stripeClock struct {
	ID      string
	Status  string
	Present int64 `json:"frozen_time"`
}

type Clock struct {
	id      string
	present time.Time
	status  string

	sc   *stripe.Client
	logf func(format string, args ...any)
}

// ClockFromID returns a Clock for the given clock ID. It does not check that
// the clock exists or what its status is. If the status or present time are
// needed, clients should call Sync.
func (c *Client) ClockFromID(id string) *Clock {
	return &Clock{
		id: id,
		sc: c.Stripe,
	}
}

// NewClock creates a new clock in the Stripe account associated with the
// client and returns a Clock ready to use.
func (c *Client) NewClock(ctx context.Context, name string, start time.Time) (*Clock, error) {
	var f stripe.Form
	f.Set("name", name)
	f.Set("frozen_time", start.Truncate(time.Second))
	var v stripeClock
	if err := c.Stripe.Do(ctx, "POST", "/v1/test_helpers/test_clocks", f, &v); err != nil {
		return nil, err
	}
	return &Clock{
		id:      v.ID,
		present: time.Unix(v.Present, 0),
		sc:      c.Stripe,
		logf:    c.Logf,
	}, nil
}

func (c *Clock) ID() string         { return c.id }
func (c *Clock) Present() time.Time { return c.present }
func (c *Clock) Status() string     { return c.status }

// Link returns a link to the clock in the Stripe dashboard.
func (c *Clock) Link() string {
	dashURL, err := url.JoinPath("https://dashboard.stripe.com", c.sc.AccountID, "/test/test-clocks", c.ID())
	if err != nil {
		panic(err) // should never happen
	}
	return dashURL
}

// Advance advances the clock to the given time.
func (c *Clock) Advance(ctx context.Context, to time.Time) error {
	var f stripe.Form
	f.Set("frozen_time", to.Truncate(time.Second))
	var v stripeClock
	if err := c.sc.Do(ctx, "POST", "/v1/test_helpers/test_clocks/"+c.ID()+"/advance", f, &v); err != nil {
		return err
	}
	c.present = time.Unix(v.Present, 0)
	return nil
}

var errForceBackoff = errors.New("force backoff")

// Wait waits for the clock to be ready, or until the context is canceled. It
// returns an error if any.
func (c *Clock) Wait(ctx context.Context) error {
	bo := backoff.NewBackoff("stroke: clock: advance backoff", c.vlogf, 5*time.Second)
	for {
		c.Sync(ctx)
		c.vlogf("clock %q is %q", c.ID(), c.Status())
		if c.Status() == "ready" {
			return nil
		}
		bo.BackOff(context.Background(), errForceBackoff)
	}
}

func (c *Clock) Sync(ctx context.Context) error {
	var f stripe.Form
	var v stripeClock
	if err := c.sc.Do(ctx, "GET", "/v1/test_helpers/test_clocks/"+c.ID(), f, &v); err != nil {
		return err
	}
	c.present = time.Unix(v.Present, 0)
	c.status = v.Status
	return nil
}

func (c *Clock) vlogf(format string, args ...any) {
	if c.logf != nil {
		c.logf(format, args...)
	}
}
