package tier

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kr/pretty"
	"golang.org/x/exp/slices"
	"kr.dev/errorfmt"
	"tier.run/stripe"
)

type Phase struct {
	Org       string
	Effective time.Time
	Features  []Feature
	Current   bool
}

func (c *Client) lookupSubscriptionID(ctx context.Context, org string) (string, error) {
	return c.cache.load("subscription:"+org, func() (string, error) {
		cid, err := c.lookupCustomer(ctx, org)
		if err != nil {
			return "", err
		}

		type T struct {
			stripe.ID
			Metadata struct {
				Name string `json:"tier.subscription"`
			}
		}

		var f stripe.Form
		f.Set("customer", cid)
		sub, err := stripe.List[T](ctx, c.Stripe, "GET", "/v1/subscription_schedules", f).Find(func(s T) bool {
			return s.Metadata.Name == "default"
		})
		if err != nil {
			return "", err
		}
		return sub.ProviderID(), nil
	})
}

func (c *Client) createSubscription(ctx context.Context, org string, phases []Phase) (err error) {
	defer errorfmt.Handlef("stripe: createSubscription: %q: %w", org, &err)

	cid, err := c.putCustomer(ctx, org)
	if err != nil {
		return err
	}

	const name = "default"
	key := "tier:subscription:create#" + name + "#" + org

	c.Logf("createSubscription: %q", key)
	c.Logf("keyprefix: %q", c.Stripe.KeyPrefix)

	_, err = stripe.Dedup(ctx, key, c.Logf, func(f stripe.Form) (err error) {
		defer func() {
			if err != nil {
				c.Logf("createSubscription: %q: %v", key, err)
			}
		}()

		f.Set("customer", cid)
		f.Set("metadata[tier.subscription]", name)
		for i, p := range phases {
			var keys []string
			for _, fe := range p.Features {
				keys = append(keys, fe.ID())
			}
			prices, err := c.lookupPrices(ctx, keys)
			if err != nil {
				return err
			}
			if len(prices) == 0 {
				return fmt.Errorf("no prices found for phase %d", i)
			}

			if i == 0 {
				f.Set("start_date", nowOrSpecific(p.Effective))
			} else {
				f.Set("phases", i-1, "end_date", nowOrSpecific(p.Effective))
			}

			for j, pid := range prices {
				c.Logf("phase %d, item %d: %s", i, j, pid)
				f.Set("phases", i, "items", j, "price", pid)
			}
		}

		return c.Stripe.Do(ctx, "POST", "/v1/subscription_schedules", f, nil)
	})
	return err
}

func (c *Client) updateSubscription(ctx context.Context, id string, phases []Phase) (err error) {
	defer errorfmt.Handlef("stripe: updateSubscription: %q: %w", id, &err)

	if id == "" {
		return errors.New("subscription id required")
	}

	var f stripe.Form
	for i, p := range phases {
		var keys []string
		for _, fe := range p.Features {
			keys = append(keys, fe.ID())
		}
		prices, err := c.lookupPrices(ctx, keys)
		if err != nil {
			return err
		}
		if len(prices) == 0 {
			return fmt.Errorf("no prices found for phase %d", i)
		}

		if i == 0 {
			// start_date for phase 0 cannot be "now" and must be a specific date.
			if p.Effective.IsZero() {
				return errors.New("the effective date for phase 0 must be specified because a schedule already exists")
			}
			f.Set("phases", 0, "start_date", p.Effective)
		} else {
			f.Set("phases", i-1, "end_date", nowOrSpecific(p.Effective))
			f.Set("phases", i, "start_date", nowOrSpecific(p.Effective))
		}
		for j, pid := range prices {
			f.Set("phases", i, "items", j, "price", pid)
		}
	}
	return c.Stripe.Do(ctx, "POST", "/v1/subscription_schedules/"+id, f, nil)
}

func (c *Client) Subscribe(ctx context.Context, org string, phases []Phase) (err error) {
	defer errorfmt.Handlef("tier: subscribe: %q: %w", org, &err)
	if len(phases) == 0 {
		return errors.New("at least one phase required")
	}

	c.Logf("Subscribe phases: %# v", pretty.Formatter(phases))

	sid, err := c.lookupSubscriptionID(ctx, org)
	if errors.Is(err, stripe.ErrNotFound) {
		return c.createSubscription(ctx, org, phases)
	} else if err != nil {
		return err
	}
	return c.updateSubscription(ctx, sid, phases)
}

// SubscribeTo subscribes org to the provided features. If a subscription has
// already begun, the current phase will be updated; otherwise a new
// subscription will be created and go into effect immediatly.
func (c *Client) SubscribeTo(ctx context.Context, org string, fs []Feature) error {
	ps, err := c.LookupPhases(ctx, org)
	if err != nil {
		return err
	}
	c.Logf("current phases: %# v", pretty.Formatter(ps))
	var cur Phase
	for i := range ps {
		if ps[i].Current {
			cur = ps[i]
		}
	}
	cur.Features = fs
	return c.Subscribe(ctx, org, []Phase{cur})
}

func (c *Client) SubscribeToPlan(ctx context.Context, org, plan string) error {
	all, err := c.Pull(ctx, 0)
	if err != nil {
		return err
	}
	var fs []Feature
	for _, f := range all {
		if f.Plan == plan {
			fs = append(fs, f)
		}
	}
	if len(fs) == 0 {
		return fmt.Errorf("no features found for plan %q", plan)
	}
	c.Logf("features for plan %q: %# v", plan, pretty.Formatter(fs))
	return c.SubscribeTo(ctx, org, fs)
}

func (c *Client) lookupPrices(ctx context.Context, keys []string) ([]string, error) {
	prices := make([]string, 0, len(keys))

	var f stripe.Form
	for _, k := range keys {
		pid, cacheHit := c.cache.lookupCache("price:" + k)
		if cacheHit {
			prices = append(prices, pid)
		} else {
			f.Add("lookup_keys[]", k)
		}
	}

	if len(prices) == len(keys) {
		// All prices were cached, so we're done.
		//
		// NOTE: If we keep going below without a list of keys, we'll
		// get all prices back from Stripe.
		return prices, nil
	}

	type T struct {
		stripe.ID
		LookupKey string `json:"lookup_key"`
	}

	pp, err := stripe.Slurp[T](ctx, c.Stripe, "GET", "/v1/prices", f)
	if err != nil {
		return nil, err
	}
	for _, p := range pp {
		prices = append(prices, p.ProviderID())
		c.cache.add("price:"+p.LookupKey, p.ProviderID())
	}

	c.Logf("keys  : %v", keys)
	c.Logf("prices: %v", prices)

	return prices, nil
}

func notFoundAsNil(err error) error {
	if errors.Is(err, stripe.ErrNotFound) {
		return nil
	}
	return err
}

func (c *Client) LookupPhases(ctx context.Context, org string) (ps []Phase, err error) {
	defer errorfmt.Handlef("LookupPhase: %w", &err)

	cid, err := c.lookupCustomer(ctx, org)
	if err != nil {
		return nil, notFoundAsNil(err)
	}

	var f stripe.Form
	f.Add("expand[]", "data.phases.items.price")
	f.Set("customer", cid)

	type T struct {
		stripe.ID
		Metadata struct {
			Name string `json:"tier.subscription"`
		}
		Current struct {
			Start int64 `json:"start_date"`
			End   int64 `json:"end_date"`
		} `json:"current_phase"`
		Phases []struct {
			Start int64 `json:"start_date"`
			Items []struct {
				Price stripePrice
			}
		}
	}

	ss, err := stripe.Slurp[T](ctx, c.Stripe, "GET", "/v1/subscription_schedules", f)
	if err != nil {
		return nil, notFoundAsNil(err)
	}
	if len(ss) == 0 {
		return nil, nil
	}

	for _, s := range ss {
		const name = "default" // TODO(bmizerany): support multiple subscriptions by name
		c.Logf("subscription schedule: %# v", pretty.Formatter(s))
		if s.Metadata.Name != name {
			continue
		}
		for _, p := range s.Phases {
			fs := make([]Feature, 0, len(p.Items))
			for _, pi := range p.Items {
				fs = append(fs, stripePriceToFeature(pi.Price))
			}
			ps = append(ps, Phase{
				Org:       org,
				Effective: time.Unix(p.Start, 0),
				Features:  fs,
				Current:   p.Start == s.Current.Start,
			})
		}
	}

	slices.SortFunc(ps, func(a, b Phase) bool {
		return a.Effective.Before(b.Effective)
	})

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
	cid, err := c.lookupCustomer(ctx, org)
	if err != nil && !errors.Is(err, stripe.ErrNotFound) {
		return "", err
	}
	if errors.Is(err, stripe.ErrNotFound) {
		return c.createCustomer(ctx, org)
	}
	return cid, err
}

func (c *Client) lookupCustomer(ctx context.Context, org string) (id string, err error) {
	if !strings.HasPrefix(org, "org:") {
		return "", errors.New(`org must have prefix "org:"`)
	}

	return c.cache.load(org, func() (string, error) {
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
		if err != nil {
			return "", err
		}
		return cus.ProviderID(), nil
	})
}

func (c *Client) createCustomer(ctx context.Context, org string) (id string, err error) {
	defer errorfmt.Handlef("createCustomer: %w", &err)
	return c.cache.load(org, func() (string, error) {
		var f stripe.Form
		f.SetIdempotencyKey("customer:create:" + org)
		f.Set("metadata[tier.org]", org)
		if c.Clock != "" {
			f.Set("test_clock", c.Clock)
		}
		var created struct {
			stripe.ID
		}
		if err := c.Stripe.Do(ctx, "POST", "/v1/customers", f, &created); err != nil {
			// TODO(bmizerany): backoff and retry if idempotency_key_in_use
			return "", err
		}
		return created.ProviderID(), nil
	})
}

func nowOrSpecific(t time.Time) any {
	if t.IsZero() {
		return "now"
	}
	return t
}
