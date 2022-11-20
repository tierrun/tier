package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUpdate(t *testing.T) {
	ch := make(chan bool, 1)
	c := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"latest": %q}`, r.FormValue("test"))
		t.Logf("got request: %v", r)
		ch <- true
	}))
	defer c.Close()

	wait := func(ok bool) {
		t.Helper()
		select {
		case <-ch:
			if !ok {
				t.Fatal("unexpected update")
			}
		case <-time.After(100 * time.Millisecond):
			if ok {
				t.Fatal("expected update")
			}
		}
	}

	tt := testtier(t, "acct_test")
	tt.Setenv("TIER_UPDATE_URL", c.URL+"?test=v100")
	tt.Run("version")
	tt.GrepStderrNot(`\bv100\b`, "unexpected mention of new version")
	wait(true)
	wait(false)

	tt.Run("version")
	wait(false) // works as expected
	tt.GrepStderr(`\bv100\b`, "expected mention of new version")
	tt.GrepStdoutNot(`\bv100\b`, "unexpected mention of new version in stdout")
}
