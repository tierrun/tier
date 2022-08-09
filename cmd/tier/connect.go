package main

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/term"

	"github.com/99designs/keyring"
	"github.com/tierrun/tierx/pricing"
)

func connect() error {
	fmt.Print(`To authenticate with Stripe, copy your API key from:

	https://dashboard.stripe.com/test/apikeys (test key)
	https://dashboard.stripe.com/apikeys      (live key)

Stripe API key: `)

	defer fmt.Println()

	key, err := term.ReadPassword(0)
	if err != nil {
		return err
	}

	fmt.Print(strings.Repeat("*", 40))

	if err := setKey(key); err != nil {
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

func setKey(key []byte) error {
	if !pricing.IsValidKey(string(key)) {
		return errors.New("invalid key: key must start with (\"sk_\") or (\"rk_\") prefix")
	}
	return ring().Set(keyring.Item{Data: key})
}

func getKey() (string, error) {
	i, err := ring().Get("")
	if err != nil {
		return "", err
	}
	return string(i.Data), nil
}

func ring() keyring.Keyring {
	const keyringService = "tier.stripe.key"

	kr, err := keyring.Open(keyring.Config{
		ServiceName: keyringService,
	})
	if err != nil {
		panic(err)
	}
	return kr
}
