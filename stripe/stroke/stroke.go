// Package stroke provides utilities for testing with Stripe using isolated
// accounts and clocks.
package stroke

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"testing"
	"time"

	"tailscale.com/logtail/backoff"
	"tier.run/stripe"
)

// Client returns a new stripe.Client initialized from the STRIPE_API_KEY
// environment variable. The KeyPrefix is set to a random string.
func Client(t *testing.T) *stripe.Client {
	c, err := stripe.FromEnv()
	if err != nil {
		t.Skipf("skipping test: %v", err)
	}
	c.KeyPrefix = randomString()
	return c
}

// randomString returns a random hex encoded 16 byte string.
func randomString() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func WithAccount(t *testing.T, c *stripe.Client) *stripe.Client {
	accountID, err := createAccount(c)
	if err != nil {
		t.Fatalf("error creating account (%s): %v", accountID, err)
	}

	t.Cleanup(func() {
		if !t.Failed() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := c.Do(ctx, "DELETE", "/v1/accounts/"+accountID, stripe.Form{}, nil); err != nil {
				t.Fatalf("error cleaning up account (%s): %v", accountID, err)
			}
		} else {
			t.Logf("skipping account removal for failed test:\n\thttps://dashboard.stripe.com/%s", accountID)
		}
	})
	return c.CloneAs(accountID)
}

func createAccount(c *stripe.Client) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var v struct {
		stripe.ID
	}
	var f stripe.Form
	f.Set("type", "standard")
	if err := c.Do(ctx, "POST", "/v1/accounts", f, &v); err != nil {
		return "", err
	}
	return v.ID.ProviderID(), nil
}

type Clock struct {
	id      string
	advance func(time.Time)
	now     func() time.Time
	logf    func(string, ...any)
	dashURL string
}

func NewClock(t *testing.T, c *stripe.Client, name string, start time.Time) *Clock {
	type T struct {
		ID     string
		Status string
		Time   int64 `json:"frozen_time"`
	}

	ctx := context.Background()

	do := func(method, path string, f stripe.Form) (v T) {
		t.Helper()
		if err := c.Do(ctx, method, path, f, &v); err != nil {
			t.Fatalf("error calling %s %s: %v", method, path, err)
		}
		return
	}

	var f stripe.Form
	f.Set("name", name)
	f.Set("frozen_time", start)
	v := do("POST", "/v1/test_helpers/test_clocks", f)
	path := "/v1/test_helpers/test_clocks/" + v.ID

	// NOTE: There is no point in deleting clocks. Clients should use
	// isolated accounts, which when deleted, delete all associated clocks
	// and other objects. The API call to delete each clock would just be a
	// waste of time.

	dashURL, err := url.JoinPath("https://dashboard.stripe.com", c.AccountID, "/test/test-clocks", v.ID)
	if err != nil {
		panic(err) // should never happen
	}

	cl := &Clock{
		id: v.ID,
		advance: func(now time.Time) {
			var f stripe.Form
			f.Set("frozen_time", now)
			do("POST", path+"/advance", f)
		},
		now: func() time.Time {
			v := do("GET", path, stripe.Form{})
			return time.Unix(v.Time, 0).UTC()
		},
		logf:    t.Logf,
		dashURL: dashURL,
	}

	return cl
}

var errForceBackoff = errors.New("force backoff")

// ID returns the ID of the clock.
func (c *Clock) ID() string           { return c.id }
func (c *Clock) DashboardURL() string { return c.dashURL }

func (c *Clock) Advance(t time.Time) {
	c.advance(t)
	bo := backoff.NewBackoff("stroke: clock: advance backoff", c.logf, 5*time.Second)
	for {
		t = t.Truncate(time.Second)
		now := c.Now()
		c.logf("stroke: clock: advacing now=%v, want=%v", now, t)
		if now.After(t) || now.Equal(t) {
			return
		}
		bo.BackOff(context.Background(), errForceBackoff)
	}
}

// Now retrieves the current time for the clock from Stripe and returns it.
func (c *Clock) Now() time.Time { return c.now().UTC() }
