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

func TestReserveN(t *testing.T) {
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

	rsv, err := tc.ReserveN(context.Background(), now, "org:acme", "feature:convert", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !rsv.OK() {
		t.Errorf("reservation should be OK")
	}
	if err := rsv.Commit(); err != nil {
		t.Errorf("should never return error, but did: %v", err)
	}
	if err := rsv.Refund(); err != nil {
		t.Errorf("already committed reservation should never return error on refund, but did: %v", err)
	}
	want := []apitype.UpdateCount{
		{Op: "incr", P: 1, Org: "org:acme", Feature: "feature:convert", Now: now.UTC()},
	}
	diff.Test(t, t.Errorf, got, want)

	// refund before commit
	got = nil // reset
	rsv, err = tc.ReserveN(context.Background(), now, "org:acme", "feature:convert", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !rsv.OK() {
		t.Errorf("reservation should be OK")
	}
	if err := rsv.Refund(); err != nil {
		t.Error(err)
	}
	if err := rsv.Commit(); err != nil {
		t.Errorf("should never return error, but did: %v", err)
	}
	want = []apitype.UpdateCount{
		{Op: "incr", P: 1, Org: "org:acme", Feature: "feature:convert", Now: now.UTC()},
		{Op: "decr", N: 1, Org: "org:acme", Feature: "feature:convert", Now: now.UTC()},
	}
	diff.Test(t, t.Errorf, got, want)
}
