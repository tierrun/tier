package main

import (
	"testing"

	"tier.run/cmd/tier/cline"
	"tier.run/stripe"
)

func TestMain(m *testing.M) {
	cline.TestMain(m, main)
}

func testtier(t *testing.T) *cline.Data {
	t.Helper()
	t.Log("=== start ===")
	c, err := stripe.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.Live() {
		t.Fatal("STRIPE_API_KEY must be a live key")
	}
	return cline.Test(t)
}

func TestVersion(t *testing.T) {
	tt := testtier(t)
	tt.Run("version")
	tt.GrepStdout(`^\d+\.\d+\.\d+`, "unexpected version format")
}

func TestLiveFlag(t *testing.T) {
	tt := testtier(t)
	tt.Setenv("STRIPE_API_KEY", "sk_test_123")
	tt.RunFail("--live", "pull")
	tt.GrepStderr("^tier: --live provided with test key", "unexpected error message")

	tt = testtier(t)
	tt.Setenv("STRIPE_API_KEY", "sk_live_123")
	tt.RunFail("--live", "pull") // fails due to invalid key only
	tt.GrepStderr("stripe:.*Invalid API Key", "unexpected error message")
	tt.GrepBothNot("--live", "output contains --live flag")

	tt = testtier(t)
	tt.Setenv("STRIPE_API_KEY", "sk_live_123")
	tt.RunFail("-l", "pull") // fails due to invalid key only
	tt.GrepStderr("Usage", "-l did not produce usage")
}
