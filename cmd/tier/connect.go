package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/term"

	"github.com/99designs/keyring"
	"tier.run/pricing"
)

func connect() error {
	fmt.Print(`To authenticate with Stripe, copy your API key from:

	https://dashboard.stripe.com/test/apikeys (test key)
	https://dashboard.stripe.com/apikeys      (live key)

Stripe API key: `)

	defer fmt.Println()

	apiKey, err := term.ReadPassword(0)
	if err != nil {
		return err
	}

	fmt.Print(strings.Repeat("*", 40))

	if err := setKey(apiKey); err != nil {
		return err
	}

	_, err = tc().WhoAmI(context.Background())
	if err != nil {
		return err
	}

	fmt.Print(`

Success.

---------
  -----
    -

Welcome to Tier.

You're now ready to push a pricing model to Stripe using Tier. To learn more,
please visit:

	https://tier.run/docs/push
`)

	return nil
}

const (
	keyringLabel     = "Tier Credentials"
	keyringService   = "Tier Credentials"
	keyringStripeKey = "stripe.cli.tier.run"
)

func setKey(apiKey []byte) error {
	if !pricing.IsValidKey(string(apiKey)) {
		return errors.New("invalid key: key must start with (\"sk_\") or (\"rk_\") prefix")
	}
	return ring().Set(keyring.Item{
		Label: keyringLabel,
		Key:   keyringStripeKey,
		Data:  apiKey,
	})
}

func getKey() (string, error) {
	i, err := ring().Get(keyringStripeKey)
	if err != nil {
		return "", err
	}
	return string(i.Data), nil
}

func ring() keyring.Keyring {
	kr, err := keyring.Open(keyring.Config{
		ServiceName: keyringService,
	})
	if err != nil {
		panic(err)
	}
	return kr
}
