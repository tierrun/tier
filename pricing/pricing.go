package pricing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/client"
	"github.com/tailscale/hujson"
	"golang.org/x/exp/slices"
	"tailscale.com/util/multierr"
	"tier.run/pricing/internal/convert"
	"tier.run/pricing/schema"
)

// Errors
var (
	ErrKeyNotSet     = errors.New("STRIPE_API_KEY not set")
	ErrPlanExists    = errors.New("plan already exists")
	ErrFeatureExists = errors.New("feature already exists")
)

var MakeID = convert.MakeID

type Client struct {
	StripeKey string

	Logf       func(string, ...any)
	HTTPClient *http.Client

	initOnce sync.Once
	sc       *client.API

	sem chan token
}

func FromEnv() (*Client, error) {
	stripeKey := os.Getenv("STRIPE_API_KEY")
	if stripeKey == "" {
		return nil, ErrKeyNotSet
	}
	return &Client{StripeKey: stripeKey}, nil
}

func UnsafeStripeClient(c *Client) *client.API {
	c.init()
	return c.sc
}

var (
	keyPrefixes     = []string{"sk_", "rk_"}
	testKeyPrefixes = []string{"sk_test", "rk_test"}
)

func IsValidKey(key string) bool {
	for _, prefix := range keyPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// IsLiveKey reports if the key being used is a live key. It considers any key
// that is not a test key to be "live" out of an abundance of caution.
func IsLiveKey(key string) bool {
	if !IsValidKey(key) {
		return false
	}
	for _, prefix := range testKeyPrefixes {
		if strings.HasPrefix(key, prefix) {
			return false
		}
	}
	return true
}

func (c *Client) Live() bool { return IsLiveKey(c.StripeKey) }

func (c *Client) init() {
	c.initOnce.Do(func() {
		c.sc = &client.API{}
		bs := newBackends(c.HTTPClient)
		c.sc.Init(c.StripeKey, bs)
		c.sem = make(chan token, 20)
	})
}

func (c *Client) Pull(ctx context.Context) (schema.Model, error) {
	fps, err := c.FetchFeatures(ctx)
	if err != nil {
		return schema.Model{}, err
	}

	var cp *schema.Plan
	var plans schema.Plans
	for _, fp := range fps {
		if cp == nil || cp.ID != fp.PlanID {
			cp = &schema.Plan{ID: fp.PlanID, ProviderID: fp.PlanProviderID}
			plans = append(plans, cp)
		}
		cp.Features = append(cp.Features, fp.Feature)
	}

	slices.SortFunc(plans, func(a, b *schema.Plan) bool {
		return a.ID < b.ID
	})

	return schema.Model{Plans: plans}, nil
}

func (c *Client) FetchFeatures(ctx context.Context, ids ...string) ([]schema.FeaturePlan, error) {
	c.init()

	params := &stripe.PriceListParams{
		ListParams: listParams(ctx),
		// LookupKeys: sliceOfPtrs(ids), TODO: convert ids to lookup keys (also change lookup key schema)
	}
	params.AddExpand("data.product")
	params.AddExpand("data.tiers")

	it := c.sc.Prices.List(params)

	var fps []schema.FeaturePlan
	for it.Next() {
		f, err := convert.ToFeature(it.Price())
		if err != nil {
			return nil, err
		}
		fps = append(fps, schema.FeaturePlan{
			PlanID:  f.Plan,
			Feature: f,
		})
	}
	if err := it.Err(); err != nil {
		return nil, err
	}

	slices.SortFunc(fps, func(a, b schema.FeaturePlan) bool {
		if a.PlanID < b.PlanID {
			return true
		}
		if a.ID < b.ID {
			return true
		}
		return false
	})

	return fps, nil
}

type UserInfo struct {
	// TODO: add fields
}

func (c *Client) WhoAmI(ctx context.Context) (UserInfo, error) {
	c.init()
	_, err := c.sc.Account.Get()
	if err != nil {
		var e *stripe.Error
		if errors.As(err, &e) {
			return UserInfo{}, errors.New(e.Msg)
		}
		return UserInfo{}, err
	}
	return UserInfo{}, nil
}

type Stringish interface {
	string | []byte
}

type DecodeError struct {
	err error
}

func (e DecodeError) Error() string { return e.err.Error() }
func (e DecodeError) Unwrap() error { return e.err }

func Unmarshal[S string | []byte](b S) (m schema.Model, err error) {
	defer func() {
		if err != nil {
			err = DecodeError{err}
		}
	}()

	data, err := hujson.Standardize([]byte(b))
	if err != nil {
		return schema.Model{}, err
	}

	var out schema.Model
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&out); err != nil {
		return schema.Model{}, err
	}
	return out, nil
}

func Decode(r io.Reader) (schema.Model, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return schema.Model{}, err
	}
	return Unmarshal(b)
}

type PushEvent struct {
	Provider          string // (e.g. "stripe")
	Plan              string
	PlanProviderID    string
	Feature           string
	FeatureProviderID string
	Err               error
}

type ReportFunc func(*PushEvent)

func (c *Client) PushJSON(ctx context.Context, r io.Reader, f ReportFunc) error {
	c.init()

	g := &egroup{sem: c.sem}

	m, err := Decode(r)
	if err != nil {
		return err
	}

	for _, p := range m.Plans {
		p := p
		g.Go(func() error {
			return c.PushPlan(ctx, p, f)
		})
	}

	return g.Wait()
}

func (c *Client) PushPlan(ctx context.Context, p *schema.Plan, f ReportFunc) error {
	c.init()

	if err := schema.Validate(p); err != nil {
		return err
	}

	err := c.pushPlan(ctx, p.ID, p.Title)
	f(&PushEvent{
		Provider:       "stripe",
		Plan:           p.ID,
		PlanProviderID: convert.MakeID(p.ID),
		Err:            err,
	})
	if err != nil {
		if errors.Is(err, ErrPlanExists) {
			return nil
		}
		return err
	}

	pg := &egroup{sem: c.sem}
	for _, fp := range p.Features {
		fp := fp
		fp.Plan = p.ID
		pg.Go(func() error {
			err := c.pushFeature(ctx, p.ID, fp)
			f(&PushEvent{
				Provider:          "stripe",
				Plan:              p.ID,
				PlanProviderID:    convert.MakeID(p.ID),
				Feature:           fp.ID,
				FeatureProviderID: convert.MakeID(p.ID, fp.ID),
				Err:               err,
			})
			if err != nil && !errors.Is(err, ErrFeatureExists) {
				return err
			}
			return nil
		})
	}
	return pg.Wait()
}

func (c *Client) pushPlan(ctx context.Context, planID, title string) error {
	if title == "" {
		title = planID
	}
	pp := &stripe.ProductParams{
		Params: stripe.Params{
			Context: ctx,
			Metadata: map[string]string{
				"tier.plan": planID,
			},
		},
		ID:   ptr(convert.MakeID(planID)),
		Name: ptr(title),
		Type: ptr("service"),
	}
	_, err := c.sc.Products.New(pp)
	return toNiceErr(err)
}

func (c *Client) pushFeature(ctx context.Context, planID string, fp *schema.Feature) error {
	c.init()
	fp = convert.Expand(fp)
	pp, err := convert.ToPriceParams(ctx, planID, fp)
	if err != nil {
		return err
	}
	_, err = c.sc.Prices.New(pp)
	return toNiceErr(err)
}

func ptr[T any](v T) *T {
	return &v
}

func listParams(ctx context.Context) stripe.ListParams {
	return stripe.ListParams{
		Context: ctx,
		Limit:   ptr(int64(100)),
	}
}

func toNiceErr(err error) error {
	var e *stripe.Error
	if errors.As(err, &e) {
		switch {
		case e.Code == stripe.ErrorCodeResourceAlreadyExists:
			return ErrPlanExists
		case e.Type == stripe.ErrorTypeInvalidRequest && e.Param == "lookup_key":
			return ErrFeatureExists
		}
	}
	return err
}

// newBackends creates a new set of backends with the given HTTP client without
// the default logger. This is a copy of stripe.NewBackends, but one that
// doesn't puke ugly logs all over your stderr
func newBackends(httpClient *http.Client) *stripe.Backends {
	apiConfig := &stripe.BackendConfig{HTTPClient: httpClient, LeveledLogger: &stripe.LeveledLogger{Level: stripe.LevelNull}}
	connectConfig := &stripe.BackendConfig{HTTPClient: httpClient, LeveledLogger: &stripe.LeveledLogger{Level: stripe.LevelNull}}
	uploadConfig := &stripe.BackendConfig{HTTPClient: httpClient, LeveledLogger: &stripe.LeveledLogger{Level: stripe.LevelNull}}
	return &stripe.Backends{
		API:     stripe.GetBackendWithConfig(stripe.APIBackend, apiConfig),
		Connect: stripe.GetBackendWithConfig(stripe.ConnectBackend, connectConfig),
		Uploads: stripe.GetBackendWithConfig(stripe.UploadsBackend, uploadConfig),
	}
}

type token struct{}

type egroup struct {
	wg  sync.WaitGroup
	sem chan token

	mu   sync.Mutex
	errs []error
}

func (g *egroup) Go(f func() error) {
	g.sem <- token{}
	g.wg.Add(1)
	go func() {
		defer g.Done()
		err := f()
		if err != nil {
			g.mu.Lock()
			g.errs = append(g.errs, err)
			g.mu.Unlock()
		}
	}()
}

func (g *egroup) Done() {
	select {
	case <-g.sem:
	default:
	}
	g.wg.Done()
}

func (g *egroup) Wait() error {
	g.wg.Wait()
	return multierr.New(g.errs...)
}
