// Package stroke provides utilities for testing with Stripe using isolated
// accounts and clocks.
package stroke

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
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
	c.Logf = t.Logf
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

func WithAccount(t testing.TB, c *stripe.Client) *stripe.Client {
	accountID, err := createAccount(c, t)
	if err != nil {
		t.Fatalf("error creating account: %v", err)
	}
	t.Logf("test account: \n\thttps://dashboard.stripe.com/%s", accountID)
	return c.CloneAs(accountID)
}

func createAccount(c *stripe.Client, t testing.TB) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	bo := backoff.NewBackoff("stroke: create account backoff", t.Logf, 10*time.Second)
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		var v struct {
			stripe.ID
		}
		var f stripe.Form
		f.Set("type", "standard")
		name := strings.Join([]string{"tier", t.Name(), randomString()}, ".")
		f.Set("business_profile", "name", name)
		err := c.Do(ctx, "POST", "/v1/accounts", f, &v)
		if err != nil {
			t.Logf("error creating account: %v; backing off", err)
			bo.BackOff(ctx, err)
			continue
		}
		return v.ProviderID(), nil
	}

}
