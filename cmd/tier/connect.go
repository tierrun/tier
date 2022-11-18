package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"tailscale.com/logtail/backoff"

	"tier.run/fetch"
	"tier.run/profile"
)

func connect() error {
	ctx := context.Background()

	deviceName, err := os.Hostname()
	if err != nil {
		return err
	}

	ls, err := getLinks(ctx, "https://api.stripe.com", deviceName)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, `Tier connect instructions:

1. Take a mental note of the following code:

	%s

2. Follow this link and verify the above code:

	%s

3. Return here and continue using Tier.

`, ls.VerificationCode, ls.BrowserURL)

	p, err := fetchProfile(ctx, ls.PollURL)
	if err != nil {
		return err
	}

	if err := profile.Save("tier", p); err != nil {
		return err
	}

	// TODO(bmizerany): check whoami to verify keys are correct

	fmt.Printf(`Welcome to Tier!

 ---------
   -----
     -

You may now use the Tier CLI to manage you pricing model in Stripe.

Please continue to https://tier.run/docs for more information.
`)

	return nil
}

type Links struct {
	BrowserURL       string `json:"browser_url"`
	PollURL          string `json:"poll_url"`
	VerificationCode string `json:"verification_code"`
}

const defaultDashboardBaseURL = "https://dashboard.stripe.com"

func getLinks(ctx context.Context, baseURL string, deviceName string) (*Links, error) {
	urlStr, err := url.JoinPath(defaultDashboardBaseURL, "/stripecli/auth")
	if err != nil {
		return nil, err
	}

	return fetchOK[*Links](ctx, "POST", urlStr, url.Values{
		"device_name": []string{deviceName},
	}.Encode())

}

var errNotRedeemed = errors.New("not redeemed")

func fetchProfile(ctx context.Context, pollURL string) (*profile.Profile, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	bo := backoff.NewBackoff("keys", nopLogf, 5*time.Second)
	for {
		ks, err := fetchOK[*profile.Profile](ctx, "GET", pollURL, nil)
		if err == nil {
			if ks.Redeemed {
				return ks, nil
			} else {
				err = errNotRedeemed
			}
		}
		bo.BackOff(ctx, err)
	}
}

// getKey returns the API from the environment variable STRIPE_API_KEY, or the
// live mode key in config.json, or the test mode key in config.json, in that
// order. It returns an error if no key is found.
func getKey() (key, source string, err error) {
	if envAPIKey != "" {
		return envAPIKey, "STRIPE_API_KEY", nil
	}
	p, err := profile.Load("tier")
	if err != nil {
		return "", "", err
	}
	if *flagLive {
		return p.LiveAPIKey, profile.ConfigPath(), nil
	}
	return p.TestAPIKey, profile.ConfigPath(), nil
}

//lint:ignore U1000 this type is used as a type parameter, but staticcheck seems to not be able to detect that yet. Remove this comment when staticcheck will stop complaining.
type jsonError struct {
	Err struct {
		Code    string
		Param   string
		Message string
	} `json:"error"`
}

func (e *jsonError) Error() string {
	return fmt.Sprintf("stripe: %s: %s: %s", e.Err.Code, e.Err.Param, e.Err.Message)
}

func fetchOK[R any](ctx context.Context, method, urlStr string, body any, opts ...any) (R, error) {
	return fetch.OK[R, *jsonError](ctx, http.DefaultClient, method, urlStr, body, append([]any{http.Header{
		"Content-Type":               []string{"application/x-www-form-urlencoded"},
		"Accept-Encoding":            []string{"identity"},
		"User-Agent":                 []string{"tier (version TODO)"},
		"X-Stripe-Client-User-Agent": []string{"xxx"},
	}}, opts...)...)
}

func nopLogf(format string, args ...any) {}
