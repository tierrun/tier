package tier

import (
	"context"
	"errors"
	"time"

	"tier.run/stripe"
)

type Phase struct {
	Meta  map[string]string // interesting metadata (e.g. "churn-risk=high")
	Start time.Time         // the time the phase starts; zero means immediately
	End   time.Time         // the time the phase ends; zero means never
}

var errCustomerNotFound = errors.New("customer not found")

func (c *Client) AppendPhase(ctx context.Context, email string, phases []Phase) error {
	if err := c.Stripe.Do(ctx, "POST", "/v1/subscription_schedules", stripe.Form{}, nil); err != nil {
		return err
	}
	return nil
}

// lookupCustomerID returns the customer ID for the given email address; if any.
// It returns the empty string without an error if no customer exists with the provided email.
//
// It only returns errors encountered while communicating with Stripe.
func (c *Client) lookupCustomerID(ctx context.Context, email string) (string, error) {
	var f stripe.Form
	f.Set("email")
	f.Set("limit", 1) // be defensive

	type T struct {
		stripe.ID
	}

	cs, err := stripe.Slurp[T](ctx, c.Stripe, "GET", "/v1/customers", f)
	if err != nil {
		return "", err
	}
	if len(cs) == 0 {
		return "", nil
	}
	return cs[0].ProviderID(), nil
}
