package main

import (
	"context"
	"os"

	"github.com/stripe/stripe-cli/pkg/config"
	"github.com/stripe/stripe-cli/pkg/login"
)

func connect() error {
	cfg := &config.Config{}
	cfg.LogLevel = "error"
	cfg.InitConfig()
	if err := login.Login(context.Background(), "https://dashboard.stripe.com", cfg, os.Stdin); err != nil {
		return err
	}
	return nil
}

func getKey(live bool) (string, error) {
	cfg := &config.Config{}
	cfg.LogLevel = "error"
	cfg.InitConfig()
	return cfg.Profile.GetAPIKey(live)
}
