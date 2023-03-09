package tier

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"kr.dev/diff"
	"tier.run/api/apitypes"
	"tier.run/refs"
)

func TestUserPassword(t *testing.T) {
	var mu sync.Mutex
	var got []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, _, _ := r.BasicAuth()
		mu.Lock()
		got = append(got, key)
		mu.Unlock()
		io.WriteString(w, "{}")
	}))

	t.Setenv("TIER_BASE_URL", s.URL)
	t.Setenv("TIER_API_KEY", "testkey")

	c, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Pull(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"testkey"}
	diff.Test(t, t.Errorf, got, want)
}

func TestFromEnv(t *testing.T) {
	cases := []struct {
		envBaseURL  string
		envAPIKey   string
		wantBaseURL string
		wantAPIKey  string
	}{
		{
			envBaseURL:  "https://x.com",
			envAPIKey:   "testKey",
			wantBaseURL: "https://x.com",
			wantAPIKey:  "testKey",
		},
		{
			envBaseURL:  "",
			envAPIKey:   "testKey",
			wantBaseURL: "https://api.tier.run",
			wantAPIKey:  "testKey",
		},
		{
			envBaseURL:  "",
			envAPIKey:   "",
			wantBaseURL: defaultBaseURL,
			wantAPIKey:  "",
		},
		{
			envBaseURL:  "https://y.com",
			envAPIKey:   "",
			wantBaseURL: "https://y.com",
			wantAPIKey:  "",
		},
	}

	for _, tt := range cases {
		t.Run("", func(t *testing.T) {
			t.Setenv("TIER_BASE_URL", tt.envBaseURL)
			t.Setenv("TIER_API_KEY", tt.envAPIKey)
			c, err := FromEnv()
			if err != nil {
				t.Fatal(err)
			}
			if c.BaseURL != tt.wantBaseURL {
				t.Errorf("BaseURL = %q; want %q", c.BaseURL, tt.wantBaseURL)
			}
			if c.APIKey != tt.wantAPIKey {
				t.Errorf("APIKey = %q; want %q", c.APIKey, tt.wantAPIKey)
			}
		})
	}
}

func TestReportNow(t *testing.T) {
	var mu sync.Mutex
	var got []apitypes.ReportRequest
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var v apitypes.ReportRequest
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			t.Error(err)
		}
		got = append(got, v)
		io.WriteString(w, "{}")
	}))

	c := &Client{BaseURL: s.URL}
	if err := c.Report(context.Background(), "org:foo", "feature:x", 1); err != nil {
		t.Fatal(err)
	}

	want := []apitypes.ReportRequest{
		{
			Org:     "org:foo",
			Feature: refs.MustParseName("feature:x"),
			N:       1,

			// Check that At is unset causing use to use Stripe's 'now'.
			At: time.Time{},
		},
	}

	diff.Test(t, t.Errorf, got, want)
}
