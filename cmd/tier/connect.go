package main

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/term"

	"github.com/zalando/go-keyring"
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

const keyringService = "tier.stripe.key"

func setKey(data []byte) error {
	key := string(data)
	if !strings.HasPrefix(string(key), "sk_") {
		return errors.New("invalid key: key must start with (\"sk_\") prefix")
	}
	if strings.HasPrefix(key, "sk_live_") {
		return keyring.Set(keyringService, "live", key)
	}
	return keyring.Set(keyringService, "test", key)
}

func getKey(live bool) (string, error) {
	if live {
		return keyring.Get(keyringService, "live")
	} else {
		return keyring.Get(keyringService, "test")
	}
}
