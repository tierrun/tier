package main

import (
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
		key, err := getKey()
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

		sc := &stripe.Client{
			APIKey:    key,
			KeyPrefix: os.Getenv("TIER_KEY_PREFIX"),
			Logf:      vlogf,
		}
		controlClient = &control.Client{
			Stripe: sc,
			Logf:   vlogf,
		}
	}
	return controlClient
}
