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

func TestReportBackground(t *testing.T) {
	recv := make(chan []byte, 5) // arbitrary buffer size
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

	tt := testtier(t, fatalHandler(t))
	tt.Unsetenv("DO_NOT_TRACK")
	tt.Setenv("_TIER_BG_TASKS", "track")
	tt.Setenv("_TIER_EVENTS", "{}")
	tt.Setenv("TIER_TRACK_BASE_URL", s.URL)
	tt.Run("version") // does not hit stripe, so this should not error
	got := wait()
	want := "{}"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReportForeground(t *testing.T) {
	recv := make(chan []byte, 5) // arbitrary buffer size
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

	tt := testtier(t, serveInvalidAPIKey)
	tt.Unsetenv("DO_NOT_TRACK")
	tt.Setenv("TIER_TRACK_BASE_URL", s.URL)
	tt.RunFail("pull")
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

func fatalHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t.Helper()
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
	}
}
