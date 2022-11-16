package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tier.run/cmd/tier/cline"
	"tier.run/fetch/fetchtest"
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
		t.Fatal("STRIPE_API_KEY must be a test key")
	}

	// Make home something other than actual home as to not pick up a real
	// config and push to a real account.
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".config/tier"), 0700)
	chdir(t, home)

	cfg := fmt.Sprintf(`{"profiles":{"tier":{"testmode_key_secret": %q}}}`, c.APIKey)
	err = os.WriteFile(filepath.Join(home, ".config/tier/config.json"), []byte(cfg), 0600)
	if err != nil {
		t.Fatal(err)
	}

	chdir(t, home)

	ct := cline.Test(t)
	ct.Unsetenv("STRIPE_API_KEY")
	ct.Setenv("HOME", home)
	ct.Setenv("TIER_DEBUG", "1")
	ct.Setenv("DO_NOT_TRACK", "1") // prevent tests from sending events
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
	tt.Unsetenv("TIER_DEBUG")
	tt.RunFail("--live", "pull") // fails due to invalid key only
	tt.GrepStderr("invalid_api_key", "expected error message")
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

func TestSwitchIsoloate(t *testing.T) {
	tt := testtier(t)
	tt.Run("switch", "-c")
	tt.GrepStdout("Running in isolation mode.", "helpful message not printed")
	tt.GrepStdout(`https://dashboard.stripe.com/acct_.*/test`, "expected URL")

	tt.RunFail("switch", "-c", "acct_123")
	tt.GrepStderr("does not accept arguments", "expected error message")

	tt.RunFail("switch")
	tt.GrepStderr("Usage:", "expected usage message")

	tt.RunFail("switch", "-c")
	tt.GrepStderr("tier.state file present", "expected error message")
	tt.GrepStderr("To switch to an ioslated account", "expected helpful hint")

	if err := os.Remove("tier.state"); err != nil {
		t.Fatal(err)
	}

	tt.Run("switch", "acct_works")
	tt.GrepStdout("Running in isolation mode.", "helpful message not printed")
	tt.GrepStdout(`https://dashboard.stripe.com/acct_works/test`, "expected URL")
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
					tt.GrepStdoutNot(".+", "unexpected output")
				}
			} else {
				tt.RunFail("push", c.param)
				tt.GrepBoth(c.match, "unexpected output")
			}
		})
	}
}

func TestWhoAmI(t *testing.T) {
	tt := testtier(t)
	tt.Run("whoami")
	tt.GrepStdout(`ID:\s+acct_.*`, "expected accountID")
	tt.GrepStdout(`Isolated:\s+false`, "expected created")
	tt.GrepStdout(`Created:\s+\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`, "expected created")
	tt.GrepStdout(`https://dashboard.stripe.com/acct_.*`, "expected URL")
	tt.GrepStdout(`KeySource:.*config.json`, "expected accountID")

	tt.Run("switch", "-c")

	tt.Setenv("STRIPE_API_KEY", os.Getenv("STRIPE_API_KEY"))
	tt.Run("whoami")
	tt.GrepStdout(`Isolated:\s+true`, "expected accountID")
	tt.GrepStdout(`KeySource:\s+STRIPE_API_KEY`, "expected accountID")
}

func TestIsolatedAccountInvalid(t *testing.T) {
	const errBody = `
		{
		  "error": {
		    "code": "account_invalid",
		    "doc_url": "https://stripe.com/docs/error-codes/account-invalid",
		    "message": "The provided key 'rk_test_*********************************************************************************************0wwMmD' does not have access to account 'acct_1M2KTDCfAzqq5Iv8' (or that account does not exist). Application access may have been revoked.",
		    "type": "invalid_request_error"
		  }
		}
	`

	c := fetchtest.NewServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(400)
		io.WriteString(w, errBody)
	})

	tt := testtier(t)
	tt.Unsetenv("TIER_DEBUG")
	tt.Setenv("STRIPE_BASE_API_URL", fetchtest.BaseURL(c))
	tt.RunFail("whoami")
	tt.GrepStderr("Running in isloated mode without the API key that started it.", "expected error message")
}

func chdir(t *testing.T, dir string) {
	dir0, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(dir0); err != nil {
			panic(err)
		}
	})
}
