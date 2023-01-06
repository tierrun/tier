package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kr.dev/diff"
	"tier.run/cmd/tier/cline"
	"tier.run/profile"
	"tier.run/stripe"
	"tier.run/stripe/stroke"
)

var serveInvalidAPIKey = func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(401)
	io.WriteString(w, `{"error":{"message":"Invalid API Key provided"}}`)
}

func TestMain(m *testing.M) {
	cline.TestMain(m, main)
}

func testtier(t *testing.T, h http.HandlerFunc) *cline.Data {
	t.Helper()
	t.Log("=== start ===")
	c := stroke.Client(t)
	if c.Live() {
		t.Fatal("STRIPE_API_KEY must be a test key")
	}

	// Make home something other than actual home as to not pick up a real
	// config and push to a real account.
	home := t.TempDir()
	t.Setenv("HOME", home) // be paranoid and just set for all tests
	chdir(t, home)

	ct := cline.Test(t)
	ct.Unsetenv("STRIPE_API_KEY") // force use of config file
	ct.Setenv("HOME", home)
	ct.Setenv("TIER_DEBUG", "1")
	ct.Setenv("DO_NOT_TRACK", "1") // prevent tests from sending events

	p := &profile.Profile{
		TestAPIKey: c.APIKey,
		AccountID:  "acct_profile",
	}

	// save a config file with the root account
	err := profile.Save("tier", p)
	if err != nil {
		t.Fatal(err)
	}

	var a stripe.Account
	if h != nil {
		s := httptest.NewServer(h)
		t.Cleanup(s.Close)

		ct.Unsetenv("STRIPE_API_KEY")
		ct.Setenv("STRIPE_BASE_API_URL", s.URL)
		a = stripe.Account{ID: "acct_state"}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Run in isolation mode by default.
		a, err = createSwitchAccount(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
	}

	if err := saveState(a); err != nil {
		t.Fatal(err)
	}
	return ct
}

func TestVersion(t *testing.T) {
	tt := testtier(t, fatalHandler(t))
	tt.Run("version")
	tt.GrepStdout(`^\d+\.\d+\.\d+`, "unexpected version format")
}

func TestLiveFlag(t *testing.T) {
	tt := testtier(t, fatalHandler(t))
	tt.Setenv("STRIPE_API_KEY", "sk_test_123")
	tt.RunFail("--live", "pull")
	tt.GrepStderr("^tier: --live provided with test key", "unexpected error message")

	tt = testtier(t, serveInvalidAPIKey)
	tt.Setenv("STRIPE_API_KEY", "sk_live_123")
	tt.RunFail("--live", "pull") // fails due to invalid key only
	tt.GrepStderr("invalid_api_key", "expected error message")
	tt.GrepBothNot("--live", "output contains --live flag")

	tt = testtier(t, fatalHandler(t))
	tt.Setenv("STRIPE_API_KEY", "sk_live_123")
	tt.RunFail("-l", "pull") // fails due to invalid key only
	tt.GrepStderr("Usage", "-l did not produce usage")
}

func TestServeAddrFlag(t *testing.T) {
	tt := testtier(t, fatalHandler(t))
	tt.RunFail("serve", "--addr", ":-1")
	tt.GrepBoth("invalid port", "bad port accepted or ignored")
}

func TestSwitchIsoloate(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if g := r.FormValue("type"); g != "standard" {
			t.Fatalf("unexpected type: %q", g)
		}
		if g := r.FormValue("metadata[tier.account]"); g != "switch" {
			t.Fatalf("unexpected metadata[tier.account]: %q", g)
		}
		io.WriteString(w, `{
			"id": "acct_123",
			"created": 12123123
		}`)
	}))
	tt := testtier(t, fatalHandler(t))
	tt.Setenv("STRIPE_BASE_API_URL", s.URL)

	// turn off isolation to avoid error about already being in isolation
	// mode
	if err := os.Remove("tier.state"); err != nil {
		t.Fatal(err)
	}

	tt.Run("switch", "-c")
	tt.GrepStdout("Running in isolation mode.", "helpful message not printed")
	tt.GrepStdout(`\shttps://dashboard.stripe.com/acct_123/test`, "expected URL")

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
	tt.GrepStdout(`\shttps://dashboard.stripe.com/acct_works/test`, "expected URL")

	tt.Run("switch", "https://dashboard.stripe.com/acct_1M9f6lCYptJuIvl3/test/invoices/in_1M9fFICYptJuIvl3JQCXUdXj")
	tt.GrepStdout("Running in isolation mode.", "helpful message not printed")
	tt.GrepStdout(`\shttps://dashboard.stripe.com/acct_1M9f6lCYptJuIvl3/test$`, "expected URL")
}

func TestSwitchPreallocateTask(t *testing.T) {
	type T struct {
		Type string
		Meta string
	}

	var got atomic.Value
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("req: %s %s: %s", r.Method, r.URL.Path, r.Form)

		if !got.CompareAndSwap(nil, T{
			Type: r.FormValue("type"),
			Meta: r.FormValue("metadata[tier.account]"),
		}) {
			panic("unexpected second request")
		}

		io.WriteString(w, `{
			"id": "acct_123",
			"created": 12123123
		}`)
	}))
	tt := testtier(t, fatalHandler(t))
	tt.Setenv("STRIPE_BASE_API_URL", s.URL)
	tt.Setenv("_TIER_BG_TASKS", "preallocateAccount")

	// turn off isolation to avoid error about already being in isolation
	// mode
	if err := os.Remove("tier.state"); err != nil {
		t.Fatal(err)
	}

	tt.Run("version")

	diff.Test(t, t.Errorf, got.Load(), T{
		Type: "standard",
		Meta: "switch",
	})
}

func TestPushStdin(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"id": "price_123"}`)
	}))

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
			tt := testtier(t, fatalHandler(t))
			tt.Unsetenv("TIER_DEBUG")
			tt.Setenv("STRIPE_BASE_API_URL", s.URL)

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

func TestPushNewFeatureExistingPlan(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		io.WriteString(w, `{
			"error": {
				"code": "resource_already_exists"
			}
		}`)
	}))

	tt := testtier(t, fatalHandler(t))
	tt.Unsetenv("TIER_DEBUG")
	tt.Setenv("STRIPE_BASE_API_URL", s.URL)
	const pj = `{
	    "plans": {
		"plan:free@0": {
		    "features": {
			"feature:foo": {}
		    }
		}
	    }
	}`
	tt.SetStdinString(pj)
	tt.RunFail("push", "-")
	tt.GrepStdout("plan already exists", "expected error message")
	tt.GrepStderr("; aborting.", "expected error message")
}

func TestPushLinks(t *testing.T) {
	c := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"id": "price_123"}`)
	}))

	tt := testtier(t, fatalHandler(t))
	tt.Unsetenv("TIER_DEBUG")
	tt.Setenv("STRIPE_BASE_API_URL", c.URL)
	tt.SetStdinString(`{
	    "plans": {
		"plan:free@0": {
		    "features": {
			"feature:foo": {}
		    }
		}
	    }
	}`)
	tt.Run("push", "-")
	tt.GrepStdout("https://dashboard.stripe.com/acct_state/prices/price_123", "expected URL")

	// get out of isolation
	if err := os.Remove("tier.state"); err != nil {
		t.Fatal(err)
	}

	// use accountID in profile
	tt.SetStdinString(`{
	    "plans": {
		"plan:free@1": {
		    "features": {
			"feature:foo": {}
		    }
		}
	    }
	}`)
	tt.Run("push", "-")
	tt.GrepStdout("https://dashboard.stripe.com/acct_profile/prices/price_123", "expected URL")

	// assume default dashboard URL is okay if STRIPE_API_KEY is set
	tt.SetStdinString(`{
	    "plans": {
		"plan:free@2": {
		    "features": {
			"feature:foo": {}
		    }
		}
	    }
	}`)
	tt.Setenv("STRIPE_API_KEY", "rk_test_123")
	tt.Run("push", "-")
	tt.GrepStdout("https://dashboard.stripe.com/test/prices/price_123", "expected URL")

	// assume default dashboard URL is okay if STRIPE_API_KEY is set; live mode
	tt.SetStdinString(`{
	    "plans": {
		"plan:free@2": {
		    "features": {
			"feature:foo": {}
		    }
		}
	    }
	}`)
	tt.Setenv("STRIPE_API_KEY", "rk_live_123")
	tt.Run("--live", "push", "-")
	tt.GrepStdout("https://dashboard.stripe.com/prices/price_123", "expected URL")
}

func TestWhoAmI(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{
			"id": "acct_123",
			"created": 1668981628
		}`)
	}))

	tt := testtier(t, fatalHandler(t))
	tt.Setenv("STRIPE_BASE_API_URL", s.URL)
	tt.Run("whoami")

	// we've already "switched" in tiertier() above
	tt.GrepStdout(`ID:\s+acct_.*`, "expected accountID")
	tt.GrepStdout(`Isolated:\s+true`, "expected created")
	tt.GrepStdout(`Created:\s+\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`, "expected created")
	tt.GrepStdout(`https://dashboard.stripe.com/acct_.*`, "expected URL")
	tt.GrepStdout(`KeySource:.*config.json`, "expected accountID")

	// undo the "switch"
	if err := os.Remove("tier.state"); err != nil {
		t.Fatal(err)
	}

	tt.Setenv("STRIPE_API_KEY", "sk_test_123")
	tt.Run("whoami")
	tt.GrepStdout(`Isolated:\s+false`, "expected accountID")
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

	tt := testtier(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(400)
		io.WriteString(w, errBody)
	})

	tt.Unsetenv("TIER_DEBUG")
	tt.RunFail("whoami")
	tt.GrepStderr("Running in isolated mode without the API key that started it.", "expected error message")
}

func TestCleanSwitchAccounts(t *testing.T) {
	wants := func(r *http.Request, method, path string) bool {
		re := regexp.MustCompile(path)
		return r.Method == method && re.MatchString(r.URL.Path)
	}
	var (
		mu     sync.Mutex
		delLog []string
	)
	tt := testtier(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case wants(r, "GET", "^/v1/accounts$"):
			io.WriteString(w, `{
				"data": [
					{
						"id": "acct_123",
						"created": 1669098188,
						"metadata": {
							"tier.account": "switch"
						}
					},
					{
						"id": "acct_456",
						"created": 1669098188
					},
					{
						"id": "acct_999",
						"created": 1669098188
					}
				]
			}`)
		case wants(r, "DELETE", "^/v1/accounts/.*"):
			mu.Lock()
			defer mu.Unlock()
			delLog = append(delLog, r.URL.Path)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	tt.Run("clean", "--switchaccounts=0")
	tt.GrepBoth("^acct_123$", "expected message")
	tt.GrepStdoutNot("acct_456", "unexpected message")
	tt.GrepStdoutNot("acct_999", "unexpected message")

	want := []string{"/v1/accounts/acct_123"}
	diff.Test(t, t.Errorf, delLog, want)
}

func TestSwitchGCLiveMode(t *testing.T) {
	tt := testtier(t, fatalHandler(t))
	tt.Setenv("STRIPE_API_KEY", "sk_live_123")
	tt.RunFail("--live", "clean", "--switchaccounts=0")
	tt.GrepBoth("refusing.*live mode", "expected hint")
}

func TestSubscribe(t *testing.T) {
	tt := testtier(t, fatalHandler(t))
	tt.Unsetenv("TIER_DEBUG")
	tt.RunFail("subscribe", "plan:free@2")
	tt.GrepStderr("org must be prefixed", "expected prefix error")

	tt.RunFail("subscribe")
	tt.GrepStderr("Usage:", "expected URL")

	tt.RunFail("subscribe", "--email")
	tt.GrepStderr("flag needs an argument", "expected flag error")

	tt.RunFail("subscribe", "--email", "foo")
	tt.GrepStderr("Usage:", "expected usage")

	tt = testtier(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/customers" {
			io.WriteString(w, `{}`)
		}
		email := r.FormValue("email")
		if !strings.Contains(email, "@") {
			w.WriteHeader(400)
			io.WriteString(w, `{"code": "invalid_email"}`)
		} else {
			io.WriteString(w, `{}`)
		}
	})
	tt.Unsetenv("TIER_DEBUG")
	tt.RunFail("subscribe", "--email", "foo", "org:test")
	tt.GrepBothNot("invalid_email", "expected invalid email code")

	tt = testtier(t, okHandler(t))
	tt.Unsetenv("TIER_DEBUG")
	tt.Run("subscribe", "--email", "foo@test.com", "org:test")
	tt.GrepBothNot(".+", "unexpected output")

	tt.Run("subscribe", "org:test")
	tt.Unsetenv("TIER_DEBUG")
	tt.GrepBothNot(".+", "unexpected output")
}

const responsePricesValidPlan = `
	{
	  "object": "list",
	  "url": "/v1/prices",
	  "has_more": false,
	  "data": [
	    {
	      "id": "price_1MG5iBCdYGloJaDMbVbTZlN1",
	      "object": "price",
	      "active": true,
	      "billing_scheme": "per_unit",
	      "created": 1671303979,
	      "currency": "usd",
	      "custom_unit_amount": null,
	      "livemode": false,
	      "lookup_key": null,
	      "metadata": {
		"tier.plan": "plan:foo@0",
		"tier.feature": "feature:bar@plan:foo@0"
	      },
	      "nickname": null,
	      "product": "tier__feature-phone-number-plan-basic-7",
	      "recurring": {
		"aggregate_usage": null,
		"interval": "month",
		"interval_count": 1,
		"usage_type": "licensed"
	      },
	      "tax_behavior": "unspecified",
	      "tiers_mode": null,
	      "transform_quantity": null,
	      "type": "recurring",
	      "unit_amount": 2000,
	      "unit_amount_decimal": "2000"
	    }
	  ]
	}
`

func TestSubscribeUnexpectedMissingCustomer(t *testing.T) {
	tt := testtier(t, func(w http.ResponseWriter, r *http.Request) {
		t.Logf("request: %s %s", r.Method, r.URL.Path)
		if wants(r, "POST", "/v1/subscription_schedules") {
			w.WriteHeader(400)
			io.WriteString(w, `{
				"error": {
					"code": "resource_missing",
					"param": "customer"
				}
			}`)
		} else if r.URL.Path == "/v1/prices" {
			io.WriteString(w, responsePricesValidPlan)
		} else {
			io.WriteString(w, `{}`)
		}
	})
	tt.RunFail("subscribe", "org:test", "plan:foo@0")
	tt.GrepStderr("TERR1050", "expected customer not found")
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

func wants(r *http.Request, method, path string) bool {
	return r.Method == method && r.URL.Path == path
}
