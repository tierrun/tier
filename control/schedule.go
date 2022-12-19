package control

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kr/pretty"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"kr.dev/errorfmt"
	"tier.run/refs"
	"tier.run/stripe"
	"tier.run/values"
)

// TODO(bmizerany): we don't support names in the MVP but the hook is
// here as a reminder of the thought that has gone into it so far and
// that we want to support it on subscribe.
const scheduleNameTODO = "default"

// Errors
var (
	ErrOrgNotFound     = errors.New("org not found")
	ErrInvalidMetadata = errors.New("invalid metadata")
	ErrInvalidPhase    = errors.New("invalid phase")

	// ErrInvalidFeature is returned when a customer that should have been
	// created is not found after "creating" it. This can happen in Test
	// Mode if the test data was cleared but the idempotency key is still
	// cached at Stripe.
	ErrUnexpectedMissingOrg = errors.New("unexpected missing org")
)

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

type OrgInfo struct {
	Email       string
	Name        string
	Description string
	Phone       string
	Metadata    map[string]string
}

type Phase struct {
	Org       string // set on read
	Effective time.Time
	Features  []refs.FeaturePlan
	Current   bool

	// Plans is the set of plans that are currently active for the phase. A
	// plan is considered active in a phase if all of its features are
	// listed in the phase. If any features from a plan is in the phase
	// without the other features in the plan, this phase is considered
	// "fragmented".
	Plans []refs.Plan
}

func (p *Phase) Fragments() []refs.FeaturePlan {
	var fs []refs.FeaturePlan
	for _, f := range p.Features {
		if !slices.Contains(p.Plans, f.Plan()) {
			fs = append(fs, f)
		}
	}
	return fs
}

type subscription struct {
	ID         string
	ScheduleID string
	Name       string
	Features   []Feature
}

func (c *Client) lookupSubscription(ctx context.Context, org, name string) (subscription, error) {
	cid, err := c.WhoIs(ctx, org)
	if err != nil {
		return subscription{}, err
	}

	var f stripe.Form
	f.Set("customer", cid)
	f.Add("expand[]", "data.schedule")

	type T struct {
		stripe.ID
		Items struct {
			Data []struct {
				ID    string
				Price stripePrice
			}
		}
		Schedule struct {
			ID       string
			Metadata struct {
				Name string `json:"tier.subscription"`
			}
		}
	}

	// TODO(bmizerany): cache the subscription ID and looked it up
	// specifically on subsequent lookups? It doesn't seem worth it because
	// we can't have more than 50 subscriptions per customer and a Slurp
	// can handle up to 100 since tht is what Stripe allows, so we'll get
	// the subscription in a single call no matter what. The only
	// difference is looping through them wastig a little memory and CPU,
	// but these are rare events (or so we thing ATM) so we can revist
	// if/when we see problems.
	// NOTE: we can't cache the schedule information because it changes
	// over time.

	v, err := stripe.List[T](ctx, c.Stripe, "GET", "/v1/subscriptions", f).Find(func(s T) bool {
		return s.Schedule.Metadata.Name == name
	})

	var fs []Feature
	for _, v := range v.Items.Data {
		f := stripePriceToFeature(v.Price)
		f.ReportID = v.ID
		fs = append(fs, f)
	}

	c.Logf("lookupSchedule: %+v", v)
	if err != nil {
		return subscription{}, err
	}
	s := subscription{
		ID:         v.ProviderID(),
		ScheduleID: v.Schedule.ID,
		Features:   fs,
	}
	return s, nil
}

func (c *Client) createSchedule(ctx context.Context, org, name string, fromSub string, info *OrgInfo, phases []Phase) (err error) {
	defer errorfmt.Handlef("stripe: createSubscription: %q: %w", org, &err)

	cid, err := c.putCustomer(ctx, org, info)
	if err != nil {
		return err
	}

	if len(phases) == 0 {
		return nil
	}

	c.Logf("keyprefix: %q", c.Stripe.KeyPrefix)

	do := func(f stripe.Form) (string, error) {
		var v struct {
			ID string
		}
		if err := c.Stripe.Do(ctx, "POST", "/v1/subscription_schedules", f, &v); err != nil {
			return "", err
		}
		return v.ID, nil
	}

	if fromSub != "" {
		var f stripe.Form
		f.Set("from_subscription", fromSub)
		sid, err := do(f)
		if err != nil {
			return err
		}
		return c.updateSchedule(ctx, sid, name, phases)
	} else {
		var f stripe.Form
		f.Set("customer", cid)
		f.Set("metadata[tier.subscription]", name)
		for i, p := range phases {
			fs, err := c.lookupFeatures(ctx, p.Features)
			if err != nil {
				return err
			}

			if i == 0 {
				f.Set("start_date", nowOrSpecific(p.Effective))
			}

			if i > 0 && i < len(phases)-1 {
				f.Set("phases", i-1, "end_date", nowOrSpecific(p.Effective))
			}

			for j, fe := range fs {
				c.Logf("phase %d, item %d: %v", i, j, fe)
				f.Set("phases", i, "items", j, "price", fe.ProviderID)
			}
		}
		_, err := do(f)
		return err
	}
}

func (c *Client) updateSchedule(ctx context.Context, id, name string, phases []Phase) (err error) {
	defer errorfmt.Handlef("stripe: updateSchedule: %q: %w", id, &err)

	if id == "" {
		return errors.New("subscription id required")
	}

	var f stripe.Form
	if name != "" {
		// When creating a schedule using from_schedule we cannot set
		// metadata, so we must wait until the update that immediately
		// follows to do so. This is why updating the name is allowed
		// if necessary.
		//
		// Error from stripe: "You cannot set `metadata` if `from_subscription` is set."
		f.Set("metadata[tier.subscription]", name)
	}
	for i, p := range phases {
		if len(p.Features) == 0 {
			return fmt.Errorf("phase %d must contain at least one feature", i)
		}

		fs, err := c.lookupFeatures(ctx, p.Features)
		if err != nil {
			return err
		}
		if len(fs) != len(p.Features) {
			return ErrFeatureNotFound
		}

		if i == 0 {
			f.Set("phases", 0, "start_date", nowOrSpecific(p.Effective))
		} else {
			f.Set("phases", i-1, "end_date", nowOrSpecific(p.Effective))
			f.Set("phases", i, "start_date", nowOrSpecific(p.Effective))
		}
		for j, fe := range fs {
			f.Set("phases", i, "items", j, "price", fe.ProviderID)
		}
	}
	return c.Stripe.Do(ctx, "POST", "/v1/subscription_schedules/"+id, f, nil)
}

func (c *Client) Schedule(ctx context.Context, org string, info *OrgInfo, phases []Phase) (err error) {
	err = c.schedule(ctx, org, info, phases)
	var e *stripe.Error
	if errors.As(err, &e) {
		if e.Code == "resource_missing" && e.Param == "customer" {
			return ErrUnexpectedMissingOrg
		}
		if strings.Contains(e.Message, "maximum number of items") {
			return ErrTooManyItems
		}
	}
	return err
}

func (c *Client) schedule(ctx context.Context, org string, info *OrgInfo, phases []Phase) (err error) {
	defer errorfmt.Handlef("tier: schedule: %q: %w", org, &err)

	c.Logf("Subscribe phases: %# v", pretty.Formatter(phases))

	for i, p := range phases {
		if len(p.Features) > 20 {
			return ErrTooManyItems
		}
		if len(p.Features) == 0 {
			return fmt.Errorf("%w: phase %d must contain a minimum of one item", ErrInvalidPhase, i)
		}
	}

	if info != nil {
		if _, err := c.putCustomer(ctx, org, info); err != nil {
			return err
		}
	}

	s, err := c.lookupSubscription(ctx, org, scheduleNameTODO)
	if errors.Is(err, ErrOrgNotFound) {
		// We only need to pay the API penalty of creating a customer
		// if we know in fact it does not exist.
		if _, err := c.putCustomer(ctx, org, info); err != nil {
			return err
		}
		return c.createSchedule(ctx, org, scheduleNameTODO, "", info, phases)
	}
	if errors.Is(err, stripe.ErrNotFound) {
		return c.createSchedule(ctx, org, scheduleNameTODO, "", info, phases)
	}
	if err != nil {
		return err
	}
	err = c.updateSchedule(ctx, s.ScheduleID, scheduleNameTODO, phases)
	if isReleased(err) {
		return c.createSchedule(ctx, org, scheduleNameTODO, s.ScheduleID, info, phases)
	}
	return err
}

// isReleased reports if err is the error stripe reports after an attempt to
// update a schedule that is already released.
func isReleased(err error) bool {
	var e *stripe.Error
	if errors.As(err, &e) {
		// Stripe does not return a specific error code for this case
		// so we have to do the brittle thing and check the message.
		return e.Type == "invalid_request_error" && strings.Contains(e.Message, "released")
	}
	return false
}

// ScheduleNow is like Schedule but immediately starts the first phase as the
// current phase and cuts off any phases that have not yet been transitioned
// to.
//
// The first phase must have a zero Effective time to indicate that it should
// start now.
func (c *Client) ScheduleNow(ctx context.Context, org string, info *OrgInfo, phases []Phase) error {
	if len(phases) > 0 {
		if !phases[0].Effective.IsZero() {
			return errors.New("first phase must be effective now")
		}
		cps, err := c.LookupPhases(ctx, org)
		if err != nil && !errors.Is(err, ErrOrgNotFound) {
			return err
		}
		for _, p := range cps {
			if p.Current {
				p0 := phases[0]
				p.Features = p0.Features
				phases[0] = p
				break
			}
		}
	}
	return c.Schedule(ctx, org, info, phases)
}

// SubscribeTo subscribes org to the provided features effective immediately,
// taking over any in-progress schedule. The customer is billed immediately
// with prorations if any.
func (c *Client) SubscribeTo(ctx context.Context, org string, fs []refs.FeaturePlan) error {
	return c.ScheduleNow(ctx, org, nil, []Phase{{
		Features: fs,
	}})
}

func (c *Client) lookupFeatures(ctx context.Context, keys []refs.FeaturePlan) ([]Feature, error) {
	if len(keys) == 0 {
		return nil, errors.New("lookupFeatures: no features provided")
	}

	lookup := func(keys []refs.FeaturePlan) ([]Feature, error) {
		// TODO(bmizerany): return error if len(keys) == 0. No keys means
		// stripe returns all known prices.
		var f stripe.Form
		f.Add("expand[]", "data.tiers")
		for _, k := range keys {
			f.Add("lookup_keys[]", stripe.MakeID(k.String()))
		}
		pp, err := stripe.Slurp[stripePrice](ctx, c.Stripe, "GET", "/v1/prices", f)
		if err != nil {
			return nil, err
		}

		if len(pp) != len(keys) {
			// TODO(bmizerany): return a more specific error with omitted features
			return nil, ErrFeatureNotFound
		}

		fs := make([]Feature, len(pp))
		for i, p := range pp {
			fs[i] = stripePriceToFeature(p)
		}
		return fs, nil
	}

	var fs []Feature
	// lookup 10 keys at a time
	for len(keys) > 0 {
		n := 10
		if len(keys) < n {
			n = len(keys)
		}
		lfs, err := lookup(keys[:n])
		if err != nil {
			return nil, err
		}
		fs = append(fs, lfs...)
		keys = keys[n:]
	}
	return fs, nil
}

func notFoundAsNil(err error) error {
	if errors.Is(err, stripe.ErrNotFound) ||
		errors.Is(err, ErrOrgNotFound) {
		return nil
	}
	return err
}

func (c *Client) LookupPhases(ctx context.Context, org string) (ps []Phase, err error) {
	defer errorfmt.Handlef("LookupPhase: %w", &err)

	cid, err := c.WhoIs(ctx, org)
	if err != nil {
		return nil, notFoundAsNil(err)
	}

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

	g, ctx := errgroup.WithContext(ctx)

	var ss []T
	g.Go(func() error {
		var f stripe.Form
		f.Add("expand[]", "data.phases.items.price")
		f.Set("customer", cid)
		got, err := stripe.Slurp[T](ctx, c.Stripe, "GET", "/v1/subscription_schedules", f)
		if len(got) > 0 { // preserve nil
			ss = got
		}
		return notFoundAsNil(err)
	})

	var m []refs.FeaturePlan
	featureByProviderID := make(map[string]refs.FeaturePlan)
	g.Go(func() (err error) {
		fs, err := c.Pull(ctx, 0)
		if err != nil {
			return err
		}
		m = values.MapFunc(fs, func(f Feature) refs.FeaturePlan {
			featureByProviderID[f.ProviderID] = f.FeaturePlan
			return f.FeaturePlan
		})
		return err
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	for _, s := range ss {
		const name = "default" // TODO(bmizerany): support multiple subscriptions by name
		c.Logf("subscription schedule: %# v", pretty.Formatter(s))
		if s.Metadata.Name != name {
			continue
		}
		for _, p := range s.Phases {
			fs := make([]refs.FeaturePlan, 0, len(p.Items))
			for _, pi := range p.Items {
				fs = append(fs, featureByProviderID[pi.Price.ProviderID()])
			}

			var plans []refs.Plan
			for _, f := range fs {
				if slices.Contains(plans, f.Plan()) {
					continue
				}
				inModel := numFeaturesInPlan(m, f.Plan())
				inPhase := numFeaturesInPlan(fs, f.Plan())
				if inModel == inPhase {
					plans = append(plans, f.Plan())
				}

			}

			ps = append(ps, Phase{
				Org:       org,
				Effective: time.Unix(p.Start, 0),
				Features:  fs,
				Current:   p.Start == s.Current.Start,

				Plans: plans,
			})
		}
	}

	slices.SortFunc(ps, func(a, b Phase) bool {
		return a.Effective.Before(b.Effective)
	})

	return ps, nil
}

// PutCustomer safely creates or updates a customer in Stripe. It does this
// being careful to not duplicate customer records. If the customer already exists, it
// will be updated with the provided info.
func (c *Client) PutCustomer(ctx context.Context, org string, info *OrgInfo) error {
	_, err := c.putCustomer(ctx, org, info)
	var e *stripe.Error
	if errors.As(err, &e) && e.Code == "email_invalid" {
		return ErrInvalidEmail
	}
	return err
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
func (c *Client) putCustomer(ctx context.Context, org string, info *OrgInfo) (string, error) {
	cid, err := c.WhoIs(ctx, org)
	if errors.Is(err, ErrOrgNotFound) {
		return c.createCustomer(ctx, org, info)
	}
	if err != nil {
		return "", err
	}

	// finally, update the already created customer
	if err := c.updateCustomer(ctx, cid, info); err != nil {
		return "", err
	}
	return cid, nil
}

type Account struct {
	ProviderID string `json:"id"`
	Email      string `json:"email"`
	CreatedAt  int64  `json:"created"`
	KeySource  string `json:"key_source"`
	Isolated   bool   `json:"isolated"`
}

func (a *Account) Created() time.Time {
	return time.Unix(a.CreatedAt, 0)
}

func (a *Account) URL() string {
	return fmt.Sprintf("https://dashboard.stripe.com/%s", a.ProviderID)
}

func (c *Client) WhoAmI(ctx context.Context) (Account, error) {
	var a Account
	var f stripe.Form
	if err := c.Stripe.Do(ctx, "GET", "/v1/account", f, &a); err != nil {
		return Account{}, err
	}
	a.Isolated = c.Stripe.AccountID != ""
	a.KeySource = c.KeySource
	return a, nil
}

func (c *Client) Isolated() bool {
	return c.Stripe.AccountID != ""
}

func (c *Client) WhoIs(ctx context.Context, org string) (id string, err error) {
	if !strings.HasPrefix(org, "org:") {
		return "", &ValidationError{Message: "org must be prefixed with \"org:\""}
	}

	cid, err := c.cache.load(org, func() (string, error) {
		c.Logf("WhoIs: cache miss: looking up customer for %q", org)
		type T struct {
			stripe.ID
			Email    string
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
	if errors.Is(err, stripe.ErrNotFound) {
		return "", ErrOrgNotFound
	}
	return cid, err
}

// LookupOrg returns the org information on file with Stripe, uncached.
func (c *Client) LookupOrg(ctx context.Context, org string) (*OrgInfo, error) {
	cid, err := c.WhoIs(ctx, org)
	if err != nil {
		return nil, err
	}
	var f stripe.Form
	var info *OrgInfo
	if err := c.Stripe.Do(ctx, "GET", "/v1/customers/"+cid, f, &info); err != nil {
		return nil, err
	}

	for k := range info.Metadata {
		if strings.HasPrefix(k, "tier.") {
			delete(info.Metadata, k)
		}
	}

	return info, nil
}

func (c *Client) createCustomer(ctx context.Context, org string, info *OrgInfo) (id string, err error) {
	defer errorfmt.Handlef("createCustomer: %w", &err)
	return c.cache.load(org, func() (string, error) {
		var f stripe.Form
		f.SetIdempotencyKey("customer:create:" + org)
		f.Set("metadata[tier.org]", org)
		if err := setOrgInfo(&f, info); err != nil {
			return "", err
		}
		if c.Clock != "" {
			f.Set("test_clock", c.Clock)
		}
		var created struct {
			stripe.ID
		}
		if err := c.Stripe.Do(ctx, "POST", "/v1/customers", f, &created); err != nil {
			return "", err
		}
		return created.ProviderID(), nil
	})
}

func setOrgInfo(f *stripe.Form, info *OrgInfo) error {
	if info == nil {
		return nil
	}
	stripe.MaybeSet(f, "email", info.Email)
	stripe.MaybeSet(f, "name", info.Name)
	stripe.MaybeSet(f, "phone", info.Phone)
	stripe.MaybeSet(f, "description", info.Description)
	for k, v := range info.Metadata {
		if strings.HasPrefix(k, "tier.") {
			return fmt.Errorf("%w: %q", ErrInvalidMetadata, k)
		}
		f.Set("metadata", k, v)
	}
	return nil
}

func (c *Client) updateCustomer(ctx context.Context, id string, info *OrgInfo) error {
	if info == nil {
		return nil
	}
	// update customer in stripe
	var f stripe.Form
	if err := setOrgInfo(&f, info); err != nil {
		return err
	}
	return c.Stripe.Do(ctx, "POST", "/v1/customers/"+id, f, nil)
}

func nowOrSpecific(t time.Time) any {
	if t.IsZero() {
		return "now"
	}
	return t
}

func numFeaturesInPlan(fs []refs.FeaturePlan, plan refs.Plan) (n int) {
	for _, f := range fs {
		if f.InPlan(plan) {
			n++
		}
	}
	return n
}
