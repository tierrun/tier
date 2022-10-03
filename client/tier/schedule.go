package tier

import (
	"context"
	"errors"
	"time"

	"github.com/kr/pretty"
	"golang.org/x/exp/slices"
	"tier.run/stripe"
)

type Phase struct {
	Effective time.Time

	Plans []string          // the plans and any features to apply during the phase
	Meta  map[string]string // interesting metadata (e.g. "churn-risk=high")
}

func (c *Client) Subscribe(ctx context.Context, org string, phases []Phase) error {
	cid, err := c.putCustomer(ctx, org)
	if err != nil {
		return err
	}

	if org == "" {
		return errors.New("no customer found for email")
	}

	fs, err := c.Pull(ctx, 0)
	if err != nil {
		return err
	}

	var f stripe.Form
	f.Set("customer", cid)
	for i, p := range phases {
		if i == 0 {
			f.Set("start_date", nowOrSpecific(p.Effective))
		} else {
			f.Set("phases", i-1, "end_date", nowOrSpecific(p.Effective))
		}

		var j int
		for _, fe := range fs {
			if containsAny(p.Plans, fe.Plan, fe.Name) {
				f.Set("phases", i, "items", j, "price", fe.ProviderID)
				// f.Set("phases", i, "items", "quantity", 0)
				j++
			}
		}
		if j == 0 {
			return errors.New("no plans found for phase")
		}
	}
	return c.Stripe.Do(ctx, "POST", "/v1/subscription_schedules", f, nil)
}

func (c *Client) LookupPhases(ctx context.Context, org string) ([]Phase, error) {
	org, err := c.putCustomer(ctx, org)
	if err != nil {
		return nil, err
	}

	var f stripe.Form
	f.Add("expand[]", "data.phases.items.price")
	f.Set("customer", org)

	type T struct {
		stripe.ID
		Phases []struct {
			Start int64 `json:"start_date"`
			Items []struct {
				Price struct {
					Metadata struct {
						Plan    string `json:"tier.plan"`
						Feature string `json:"tier.feature"`
					}
				}
			}
		}
	}

	ss, err := stripe.Slurp[T](ctx, c.Stripe, "GET", "/v1/subscription_schedules", f)
	if err != nil {
		return nil, err
	}
	if len(ss) == 0 {
		return nil, nil
	}

	c.Logf("ss: %# v", pretty.Formatter(ss))

	var ps []Phase
	for _, s := range ss {
		for _, p := range s.Phases {
			plans := make([]string, 0, len(p.Items))
			for _, pi := range p.Items {
				plans = append(plans, pi.Price.Metadata.Plan)
			}
			slices.Sort(plans)
			ps = append(ps, Phase{
				Effective: time.Unix(p.Start, 0),
				Plans:     slices.Compact(plans),
			})
		}
	}

	// TODO(bmizerany): sort by Effective and decide how to disambiguate multiple schedules

	return ps, nil
}

// putCustomer safely creates a customer in Stripe for the provided org
// identifier if one does not already exists.
//
// It uses an idempotency key to avoid racing with clients that may also be
// attempting to create the same customer. This is necessary because unlike
// products and prices, customer records do not have unique user-defined
// identifiers, so we have to first look if a record already exists, and
// subsequently create one if it doesn't, but being careful to not race with
// another client which may have also seen there was no record and is attempting
// to create the record at the same time we are.  It returns the empty string
// without an error if no customer exists with the provided email.
//
// It only returns errors encountered while communicating with Stripe.
func (c *Client) putCustomer(ctx context.Context, org string) (string, error) {
	type T struct {
		stripe.ID
		Metadata struct {
			Org string `json:"tier.org"`
		}
	}
	var f stripe.Form
	cus, err := stripe.List[T](ctx, c.Stripe, "GET", "/v1/customers", f).Find(func(v T) bool {
		return v.Metadata.Org == org
	})
	if err == nil {
		return cus.ProviderID(), nil
	}
	if !errors.Is(err, stripe.ErrNotFound) {
		return "", err
	}
	return c.createCustomer(ctx, org)
}

func (c *Client) createCustomer(ctx context.Context, org string) (string, error) {
	var f stripe.Form
	f.Set("metadata[tier.org]", org)
	f.SetIdempotencyKey("customer:create:" + org)
	var created struct {
		stripe.ID
	}
	if err := c.Stripe.Do(ctx, "POST", "/v1/customers", f, &created); err != nil {
		// TODO(bmizerany): backoff and retry if idempotency_key_in_use
		return "", err
	}
	return created.ProviderID(), nil
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
