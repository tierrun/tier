package tier

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/exp/maps"
	"kr.dev/errorfmt"
	"tailscale.com/logtail/backoff"
	"tier.run/refs"
	"tier.run/stripe"
)

type Report struct {
	N       int
	At      time.Time
	Clobber bool
}

type Usage struct {
	Feature refs.FeaturePlan
	Start   time.Time
	End     time.Time
	Used    int
	Limit   int
}

func (c *Client) ReportUsage(ctx context.Context, org string, feature refs.Name, use Report) error {
	itemID, isMetered, err := c.lookupSubscriptionItemID(ctx, org, scheduleNameTODO, feature)
	if err != nil {
		return err
	}
	if !isMetered {
		return ErrFeatureNotMetered
	}

	var f stripe.Form
	f.Set("quantity", use.N)
	f.Set("timestamp", nowOrSpecific(use.At))
	if use.Clobber {
		f.Set("action", "set")
	} else {
		f.Set("action", "increment")
	}

	// TODO(bmizerany): take idempotency key from context or use random
	// string. if in context then upstream client supplied their own.
	f.SetIdempotencyKey(randomString())

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// TODO(bmizerany): use Dedup here
	bo := backoff.NewBackoff("ReportUsage", c.Logf, 3*time.Second)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		err := c.Stripe.Do(ctx, "POST", "/v1/subscription_items/"+itemID+"/usage_records", f, nil)
		c.Logf("ReportUsage: %v", err)
		bo.BackOff(ctx, err)
		if err == nil {
			return nil
		}
	}
}

func (c *Client) LookupLimits(ctx context.Context, org string) ([]Usage, error) {
	cid, err := c.WhoIs(ctx, org)
	if err != nil {
		return nil, err
	}

	var f stripe.Form
	f.Set("customer", cid)
	f.Add("expand[]", "data.price.tiers")

	type T struct {
		stripe.ID
		Price    stripePrice
		Period   struct{ Start, End int64 }
		Quantity int
	}

	lines, err := stripe.Slurp[T](ctx, c.Stripe, "GET", "/v1/invoices/upcoming/lines", f)
	if err != nil {
		return nil, err
	}

	seen := map[refs.FeaturePlan]Usage{}
	for _, line := range lines {
		f := stripePriceToFeature(line.Price)
		if f.IsZero() { // not a Tier price
			continue
		}
		if seen[f.FeaturePlan].Used <= line.Quantity {
			seen[f.FeaturePlan] = Usage{
				Feature: f.FeaturePlan,
				Start:   time.Unix(line.Period.Start, 0),
				End:     time.Unix(line.Period.End, 0),
				Used:    line.Quantity,
				Limit:   f.Limit(),
			}
		}
	}
	return maps.Values(seen), nil
}

func (c *Client) lookupSubscriptionItemID(ctx context.Context, org, name string, feature refs.Name) (id string, isMetered bool, err error) {
	defer errorfmt.Handlef("lookupSubscriptionItemID: %w", &err)
	s, err := c.lookupSubscription(ctx, org, name)
	if err != nil {
		return "", false, err
	}
	for _, f := range s.Features {
		if f.IsVersionOf(feature) {
			return f.ReportID, f.IsMetered(), nil
		}
	}
	return "", false, fmt.Errorf("%w: %q", ErrFeatureNotFound, feature)
}

func randomString() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
