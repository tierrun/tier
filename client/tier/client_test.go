package tier

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"kr.dev/diff"
	"tier.run/client/tier/apitype"
	"tier.run/fetch/fetchtest"
)

func TestReport(t *testing.T) {
	var got []apitype.UpdateCount
	c := fetchtest.NewTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
		var v apitype.UpdateCount
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			t.Errorf("Decode: %v", err)
		}
		got = append(got, v)

		// "echo" back the request
		if err := json.NewEncoder(w).Encode(apitype.UpdateResponse{
			Used:  v.N - v.P,
			Limit: apitype.Inf,
		}); err != nil {
			t.Errorf("Encode: %v", err)
		}
	})

	tc := &Client{HTTPClient: c}
	now := time.Now().Truncate(time.Second)

	err := tc.Report(context.Background(), now, "org:acme", "feature:convert", 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []apitype.UpdateCount{
		{Op: "incr", P: 1, Org: "org:acme", Feature: "feature:convert", Now: now.UTC()},
	}
	diff.Test(t, t.Errorf, got, want)

	// refund before commit
	got = nil // reset
	err = tc.Report(context.Background(), now, "org:acme", "feature:convert", 1)
	if err != nil {
		t.Fatal(err)
	}

	err = tc.Refund(context.Background(), now, "org:acme", "feature:convert", 1)
	if err != nil {
		t.Error(err)
	}

	want = []apitype.UpdateCount{
		{Op: "incr", P: 1, Org: "org:acme", Feature: "feature:convert", Now: now.UTC()},
		{Op: "decr", N: 1, Org: "org:acme", Feature: "feature:convert", Now: now.UTC()},
	}

	diff.Test(t, t.Errorf, got, want)
}
