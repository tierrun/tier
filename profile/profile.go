package profile

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

var ErrProfileNotFound = errors.New("profile not found")

type Profile struct {
	Redeemed           bool   `json:"redeemed"`
	AccountID          string `json:"account_id"`
	DisplayName        string `json:"account_display_name"`
	DeviceName         string `json:"-"`
	LiveAPIKey         string `json:"livemode_key_secret"`
	LivePublishableKey string `json:"livemode_key_publishable"`
	TestAPIKey         string `json:"testmode_key_secret"`
	TestPublishableKey string `json:"testmode_key_publishable"`
}

type Profiles map[string]*Profile

type Config struct {
	Profiles Profiles `json:"profiles"`
}

func Load(name string) (*Profile, error) {
	c, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	p := c.Profiles[name]
	if p == nil {
		return nil, ErrProfileNotFound
	}
	return p, nil
}

func LoadConfig() (*Config, error) {
	f, err := open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var c *Config
	if err := json.NewDecoder(f).Decode(&c); err != nil {
		if err == io.EOF {
			return &Config{}, nil
		}
		return nil, err
	}
	host, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	for _, p := range c.Profiles {
		p.DeviceName = host
	}
	return c, nil
}

func Save(name string, p *Profile) error {
	c, err := LoadConfig()
	if err != nil {
		return err
	}

	if c.Profiles == nil {
		c.Profiles = make(Profiles)
	}
	c.Profiles[name] = p

	f, err := open()
	if err != nil {
		return err
	}
	defer f.Close()

	e := json.NewEncoder(f)
	e.SetIndent("", "    ")
	return e.Encode(c)
}

func open() (*os.File, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(home, ".config", "tier")
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}

	return os.OpenFile(filepath.Join(dir, "config.json"), os.O_CREATE|os.O_RDWR, 0o600)
}
