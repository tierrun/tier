package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"

	"tier.run/api"
	"tier.run/control"
	"tier.run/profile"
	"tier.run/stripe"
)

func serve(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "listening on %s\n", ln.Addr())

	h := api.NewHandler(cc(), vlogf)
	return http.Serve(ln, h)
}

var controlClient *control.Client

func cc() *control.Client {
	if controlClient == nil {
		key, source, err := getKey()
		if err != nil {
			fmt.Fprintf(stderr, "tier: There was an error looking up your Stripe API Key: %v\n", err)
			if errors.Is(err, profile.ErrProfileNotFound) {
				fmt.Fprintf(stderr, "tier: Please run `tier connect` to connect your Stripe account\n")
			}
			os.Exit(1)
		}

		if stripe.IsLiveKey(key) {
			if !*flagLive {
				fmt.Fprintf(stderr, "tier: --live is required if stripe key is a live key\n")
				os.Exit(1)
			}
		} else {
			if *flagLive {
				fmt.Fprintf(stderr, "tier: --live provided with test key\n")
				os.Exit(1)
			}
		}

		a, err := getState()
		if err != nil {
			fmt.Fprintf(stderr, "tier: %v", err)
			os.Exit(1)
		}

		keyPrefix := a.ID
		if keyPrefix == "" {
			keyPrefix = os.Getenv("TIER_KEY_PREFIX")
		}
		sc := &stripe.Client{
			APIKey:    key,
			KeyPrefix: keyPrefix,
			AccountID: a.ID,
			Logf:      vlogf,
			BaseURL:   stripe.BaseURL(),
		}
		controlClient = &control.Client{
			Stripe:    sc,
			Logf:      vlogf,
			KeySource: source,
		}
	}
	return controlClient
}

const stateFile = "tier.state"

func saveState(a stripe.Account) error {
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return os.WriteFile(stateFile, data, 0600)
}

func getState() (stripe.Account, error) {
	var a stripe.Account
	data, err := os.ReadFile(stateFile)
	if os.IsNotExist(err) {
		return a, nil
	}
	if err != nil {
		return a, err
	}
	if err := json.Unmarshal(data, &a); err != nil {
		return a, err
	}
	if a.ID == "" {
		return a, fmt.Errorf("no account ID in state file")
	}
	return a, nil
}
