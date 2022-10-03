// Package stroke provides utilities for testing with Stripe using isolated
// accounts and clocks.
package stroke

import (
	"context"
	"testing"

	"tier.run/stripe"
)

func WithAccount(t *testing.T, ctx context.Context, c *stripe.Client) *stripe.Client {
	accountID, err := createAccount(ctx, c)
	if err != nil {
		t.Fatalf("error creating account (%s): %v", accountID, err)
	}
	t.Cleanup(func() {
		if err := c.Do(ctx, "DELETE", "/v1/accounts/"+accountID, stripe.Form{}, nil); err != nil {
			t.Fatalf("error cleaning up account (%s): %v", accountID, err)
		}
	})
	return c.CloneAs(accountID)
}

func createAccount(ctx context.Context, c *stripe.Client) (string, error) {
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
