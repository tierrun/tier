package stripe

import (
	"context"
	"net/http"
	"testing"

	"tier.run/fetch/fetchtest"
)

func TestSetIdempotencyKey(t *testing.T) {
	var got string
	hc := fetchtest.NewTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Idempotency-Key")
	})
	c := &Client{
		BaseURL:    fetchtest.BaseURL(hc),
		HTTPClient: hc,
	}

	var f Form
	f.SetIdempotencyKey("foo")
	if err := c.Do(context.Background(), "POST", "/", f, nil); err != nil {
		t.Fatal(err)
	}
	if got != "foo" {
		t.Errorf("got %q; want %q", got, "foo")
	}
}
