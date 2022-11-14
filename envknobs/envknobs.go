package envknobs

import (
	"os"
	"path/filepath"
)

func TrackingBaseURL() string { return env("TIER_TRACK_BASE_URL", "https://tele.tier.run") }
func StripeAPIToken() string  { return env("STRIPE_API_TOKEN", "") }
func StripeBaseURL() string   { return env("STRIPE_BASE_API_URL", "https://api.stripe.com") }
func ConfigFile() string {
	return env("TIER_CONFIG_FILE", filepath.Join(XDGDataHome(), "tier/config.json"))
}

func XDGDataHome() string {
	if e := os.Getenv("XDG_DATA_HOME"); e != "" {
		return e
	}
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return filepath.Join(home, ".local/share")
}

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}
