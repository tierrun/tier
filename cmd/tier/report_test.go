package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReport(t *testing.T) {
	recv := make(chan []byte, 5) // arbirary buffer size
	wait := func() string {
		t.Helper()
		select {
		case data := <-recv:
			return string(data)
		case <-time.After(1 * time.Second):
			t.Fatal("timeout")
			panic("unreachable")
		}
	}

	s := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		recv <- data
	}))
	defer s.Close()

	// test background process in foreground
	tt := testtier(t, "acct_test")
	tt.Unsetenv("DO_NOT_TRACK")
	tt.Setenv("_TIER_BG_TASKS", "track")
	tt.Setenv("_TIER_EVENTS", "{}")
	tt.Setenv("TIER_TRACK_BASE_URL", s.URL)
	tt.Run("version")
	got := wait()
	want := "{}"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// test tier reports errors correctly
	tt = testtier(t, "acct_test")
	tt.Unsetenv("DO_NOT_TRACK")
	tt.Setenv("TIER_TRACK_BASE_URL", s.URL)
	tt.Setenv("STRIPE_API_KEY", "bad_key")
	tt.RunFail("--live", "pull")
	body := wait()
	var v struct {
		Err string
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v.Err, "invalid_api_key") {
		t.Errorf("error does not contain 'invalid_api_key':\n%q", v.Err)
	}

	var extra string
	select {
	case v := <-recv:
		extra = string(v)
	case <-time.After(100 * time.Millisecond):
	}

	if extra != "" {
		t.Errorf("unexpected extra output: %q", extra)
	}
}
