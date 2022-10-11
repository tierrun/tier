package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/exp/maps"
)

type Error struct {
	AccountID string
	Type      string
	Code      string
	Param     string
	Message   string
	DocURL    string
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("stripe: ")
	if e.AccountID != "" {
		b.WriteString(e.AccountID)
	}
	if e.Code != "" {
		b.WriteString(": ")
		b.WriteString(e.Code)
	}
	if e.Type != "" {
		b.WriteString(": ")
		b.WriteString(e.Type)
	}
	if e.Param != "" {
		b.WriteString(": ")
		b.WriteString(e.Param)
	}
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	}
	return b.String()
}

// Form maps a string key to a list of values. It is intended for use when
// building request bodies for Stripe requests.
type Form struct {
	v              url.Values
	idempotencyKey string
}

// Clone returns a clone f.
func (f Form) Clone() Form {
	return Form{v: maps.Clone(f.v)}
}

func (f *Form) SetIdempotencyKey(key string) {
	f.idempotencyKey = key
}

// Add creates a key and value from args and adds the value to the key. The key
// is constructed from all values in args up until the final, which will be
// used as the value.
//
// Values are converted to strings according to fmt.Sprint rules, with the
// exception of time.Time values, which are converted to unix time (seconds
// since epoch).
//
// Example mapping:
//
//	f.Add("tiers", 0, "up_to", 3) // => "tiers[0][up_to]=3"
//	f.Add("metadata", "link", "http://example.com") // => "metadata[link]=http://example.com"
//	f.Add("product[name]", "foo") // => "product[name]=foo"
//	f.Add("started", time.Unix(10, 0)) // => "started=10"
func (f *Form) Add(args ...any) {
	if f.v == nil {
		f.v = url.Values{}
	}
	f.v.Add(formKeyVal(args...))
}

func (f *Form) Set(args ...any) {
	if f.v == nil {
		f.v = url.Values{}
	}
	f.v.Set(formKeyVal(args...))
}

// Encode encodes the values into “URL encoded” form ("bar=baz&foo=quux")
// sorted by key.
func (f *Form) Encode() string {
	return f.v.Encode()
}

func formKeyVal(args ...any) (string, string) {
	if len(args) == 0 {
		panic("stripe: form key and value required")
	}
	var key string
	for i := range args[:len(args)-1] {
		if i == 0 {
			key = fmt.Sprint(args[i])
		} else {
			key = fmt.Sprintf("%s[%v]", key, args[i])
		}
	}

	v := args[len(args)-1]
	switch vv := v.(type) {
	case time.Time:
		v = vv.Unix()
	}

	return key, fmt.Sprint(v)
}

type Client struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
	AccountID  string

	// KeyPrefix is prepended to all idempotentcy keys. Use a new key prefix
	// after deleting test data. It is not recommended for use with live mode.
	KeyPrefix string
}

func FromEnv() (*Client, error) {
	key, ok := os.LookupEnv("STRIPE_API_KEY")
	if !ok {
		return nil, errors.New("stripe: missing STRIPE_API_KEY")
	}
	return &Client{APIKey: key}, nil
}

func (c *Client) Live() bool {
	return !strings.Contains(c.APIKey, "_test_")
}

func (c *Client) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func (c *Client) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return "https://api.stripe.com"
}

func (c *Client) Do(ctx context.Context, method, path string, f Form, out any) error {
	urlStr, err := url.JoinPath(c.baseURL(), path)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, strings.NewReader(f.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.APIKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if f.idempotencyKey != "" {
		key := f.idempotencyKey
		if c.KeyPrefix != "" {
			key = c.KeyPrefix + "#" + key
		}
		req.Header.Set("Idempotency-Key", key)
	}
	if c.AccountID != "" {
		req.Header.Set("Stripe-Account", c.AccountID)
	}

	resp, err := c.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		var e struct {
			Error *Error
		}
		if err := json.NewDecoder(resp.Body).Decode(&e); err != nil {
			return fmt.Errorf("stripe: error parsing error response: %w", err)
		}
		e.Error.AccountID = c.AccountID
		return e.Error
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *Client) CloneAs(accountID string) *Client {
	return &Client{
		APIKey:     c.APIKey,
		BaseURL:    c.BaseURL,
		HTTPClient: c.HTTPClient,
		AccountID:  accountID,
	}
}
