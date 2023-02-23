package stripe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(t *testing.T, h func(w http.ResponseWriter, r *http.Request)) *Client {
	s := httptest.NewServer(http.HandlerFunc(h))
	t.Cleanup(s.Close)
	c := &Client{
		BaseURL:    s.URL,
		HTTPClient: s.Client(),
		Logf:       t.Logf,
	}
	return c
}

func TestLink(t *testing.T) {
	cases := []struct {
		live      bool
		accountID string
		part      string
		want      string
	}{
		{true, "acct_123", "foo", "https://dashboard.stripe.com/acct_123/foo"},
		{true, "", "foo", "https://dashboard.stripe.com/foo"},
		{false, "acct_123", "foo", "https://dashboard.stripe.com/acct_123/test/foo"},
		{false, "", "foo", "https://dashboard.stripe.com/test/foo"},
	}
	for _, tc := range cases {
		got, err := Link(tc.live, tc.accountID, tc.part)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("link(%v, %q, %q) = %q; want %q", tc.live, tc.accountID, tc.part, got, tc.want)
		}
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv("STRIPE_API_KEY", "")
	t.Setenv("STRIPE_BASE_API_URL", "")
	c, err := FromEnv()
	if err == nil {
		t.Fatalf("expected error")
	}
	if c != nil {
		t.Fatalf("expected nil client")
	}

	t.Setenv("STRIPE_API_KEY", "foo")
	t.Setenv("STRIPE_BASE_API_URL", "https://example.com")
	c, err = FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.APIKey != "foo" {
		t.Errorf("got %q; want %q", c.APIKey, "foo")
	}
	if c.BaseURL != "https://example.com" {
		t.Errorf("got %q; want %q", c.BaseURL, "https://example.com")
	}
}

func TestSetIdempotencyKey(t *testing.T) {
	var got string
	c := newTestClient(t, func(_ http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Idempotency-Key")
	})

	var f Form
	f.SetIdempotencyKey("foo")
	if err := c.Do(context.Background(), "POST", "/", f, nil); err != nil {
		t.Fatal(err)
	}
	if got != "foo" {
		t.Errorf("got %q; want %q", got, "foo")
	}
}

func TestInvalidAPIKey(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": {"message": "Invalid API Key provided: foo"}}`))
	})

	var f Form
	if err := c.Do(context.Background(), "POST", "/", f, nil); err != ErrInvalidAPIKey {
		t.Errorf("got %v; want %v", err, ErrInvalidAPIKey)
	}
}
