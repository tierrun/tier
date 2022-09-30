//go:build ignore

package main

import (
	"context"
	"net/http"

	"tier.run/client/tier"
)

var tc *tier.Client

func Handler(w http.ResponseWriter, r *http.Request) {
	org := getOrgFromRequest(r)
	if org == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	switch {
	case isRequesting(r, "GET", "/lists"):
		// render lists; if any
	case isRequesting(r, "POST", "/lists"):
		// On-demand checks require a total checked against a limit such
		// as this example. If out-of-band flag raises are desired, then
		// this is reduced to a check if a flag is on or off in the
		// feature flag system.  Tier will handle turning it on or off
		// as usage is reported and tier determines the limit has been
		// reached.
		used, err := countTotalListsInMyDatabase(ctx, org)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		limit, err := tc.LookupLimit(ctx, org, "feature:lists")
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if used >= limit {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
	}
}

func isRequesting(r *http.Request, method string, path string) bool {
	return r.Method == method && r.URL.Path == path
}

func getOrgFromRequest(r *http.Request) string {
	panic("TODO")
}

func countTotalListsInMyDatabase(ctx context.Context, org string) (int, error) {
	panic("TODO")
}
