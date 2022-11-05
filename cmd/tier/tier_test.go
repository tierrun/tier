package main

import (
	"os"
	"path/filepath"
	"strings"
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

	// Make home something other than actual home as to not pick up a real
	// config and push to a real account.
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".config/tier"), 0700)
	const cfg = `{ "profiles": { "tier": {} } }`
	err = os.WriteFile(filepath.Join(home, ".config/tier/config.json"), []byte(cfg), 0600)
	if err != nil {
		t.Fatal(err)
	}

	ct := cline.Test(t)
	ct.Setenv("HOME", home)
	ct.Setenv("TIER_DEBUG", "1")
	return ct
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

func TestServeAddrFlag(t *testing.T) {
	tt := testtier(t)
	tt.RunFail("serve", "--addr", ":-1")
	tt.GrepBoth("invalid port", "bad port accepted or ignored")
}

func TestPushStdin(t *testing.T) {
	cases := []struct {
		stdin         string
		param         string
		match         string
		shouldSucceed bool
	}{
		// with and without stdin
		{"", "", "Usage:", false}, // TODO(bmizerany): should exit non-zero, but not fixing in this PR
		{"{", "", "Usage:", false},
		{"{}", "", "Usage:", false},

		{"", "foo.json", "no such file", false},
		{"{-}", "-", "invalid literal", false},
		{"{", "-", "unexpected EOF", false},

		{"{}", "-", "", true},
	}

	for _, c := range cases {
		t.Run("case", func(t *testing.T) {
			tt := testtier(t)
			tt.SetStdin(strings.NewReader(c.stdin))
			if c.shouldSucceed {
				tt.Run("push", c.param)
				if c.match != "" {
					tt.GrepBoth(c.match, "unexpected output")
				} else {
					tt.GrepBothNot(".+", "unexepcted output")
				}
			} else {
				tt.RunFail("push", c.param)
				tt.GrepBoth(c.match, "unexpected output")
			}
		})
	}
}
