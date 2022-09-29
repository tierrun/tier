package tier

import (
	"context"
	"time"

	"golang.org/x/exp/slices"
	"tier.run/stripe"
)

// Never is intended for use as the end parameter SetSchedule when specifying a
// schedule that never expires (e.g. the final phase has no end date).
//
// Example:
//
//	c.Schedule(ctx, "bill@acme.com", phases, tier.Never)
var Never time.Time

type Phase struct {
	Effective time.Time // the time the phase starts; zero means immediately
	Plans     []string  // the plans and any features to apply during the phase

	Meta map[string]string // interesting metadata (e.g. "churn-risk=high")
}

func (c *Client) Schedule(ctx context.Context, email string, phases []Phase, end time.Time) error {
	org, err := c.lookupOrgID(ctx, email)
	if err != nil {
		return err
	}

	fs, err := c.Pull(ctx, 0)
	if err != nil {
		return err
	}

	var f stripe.Form
	f.Set("customer", org)
	for i, p := range phases {
		if i == 0 {
			f.Set("start_date", nowOrSpecific(p.Effective))
		} else {
			t := nowOrSpecific(p.Effective)
			f.Set("phases", i-1, "end_date", t)
			f.Set("phases", i, "start_date", t)
		}
		for k, v := range p.Meta {
			f.Set("phases", i, "metadata", k, v)
		}

		var j int
		for _, fe := range fs {
			if containsAny(p.Plans, fe.Plan, fe.Name) {
				f.Set("phases", i, "items", j, "price", fe.ProviderID)
				j++
				// f.Set("phases", i, "items", "quantity", 0)
			}
		}
	}
	return c.Stripe.Do(ctx, "POST", "/v1/subscription_schedules", f, nil)
}

// LookupOrgID returns the customer ID for the given email address; if any.
// It returns the empty string without an error if no customer exists with the provided email.
//
// It only returns errors encountered while communicating with Stripe.
func (c *Client) lookupOrgID(ctx context.Context, email string) (string, error) {
	var f stripe.Form
	f.Set("email", email)
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

func nowOrSpecific(t time.Time) any {
	if t.IsZero() {
		return "now"
	}
	return t
}

func containsAny(s []string, ss ...string) bool {
	for _, x := range ss {
		if slices.Contains(s, x) {
			return true
		}
	}
	return false
}
