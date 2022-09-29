package tier

import (
	"context"
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

func (c *Client) Subscribe(ctx context.Context, email string, phases []Phase) error {
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
	}
	return c.Stripe.Do(ctx, "POST", "/v1/subscription_schedules", f, nil)
}

func (c *Client) LookupPhases(ctx context.Context, email string) ([]Phase, error) {
	org, err := c.lookupOrgID(ctx, email)
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

// lookupOrgID returns the customer ID for the given email address; if any.
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
