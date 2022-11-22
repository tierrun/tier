package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/semver"
	tierroot "tier.run"
	"tier.run/envknobs"
)

var versionFileName = filepath.Join(envknobs.XDGDataHome(), "tier", "latest-version")

func updateAvailable() string {
	ver, err := os.ReadFile(versionFileName)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		vlogf("error reading version file: %v", err)
		return ""
	}
	latest := string(ver)
	current := "v" + strings.TrimSpace(tierroot.Version)
	vlogf("latest version: %s, current version: %s", latest, current)
	vlogf("update: %v", semver.Compare(latest, current))
	if semver.Compare(latest, current) > 0 {
		return latest
	}
	return ""
}

func checkForUpdate() error {
	ver, err := fetchLatestVersion()
	if err != nil {
		vlogf("error fetching latest version: %v", err)
		return err
	}
	os.MkdirAll(filepath.Dir(versionFileName), 0755)
	if err := os.WriteFile(versionFileName, []byte(ver), 0644); err != nil {
		vlogf("error writing version file: %v", err)
		return err
	}
	vlogf("updated version file to %q", ver)
	return nil
}

func fetchLatestVersion() (string, error) {
	updateURL := os.Getenv("TIER_UPDATE_URL")
	if updateURL == "" {
		updateURL = "https://tier.run/releases"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", updateURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var v struct {
		Latest string `json:"latest"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", err
	}
	return v.Latest, nil
}
