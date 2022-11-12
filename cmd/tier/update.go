package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"blake.io/forks"
	"golang.org/x/mod/semver"
	"kr.dev/errorfmt"
	tierroot "tier.run"
)

// checkForUpdate reports a new version if one is available; otherwise it
// returns the empty string.
func checkForUpdate() (latest string, err error) {
	defer errorfmt.Handlef("checkForUpdate: %w", &err)
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	name := filepath.Join(home, ".local", "tier", "version")

	if forks.ChildOf("update") {
		log.Println("child process; checking for updates")
		ver, err := fetchLatestVersion()
		if err != nil {
			log.Printf("error fetching latest version: %v", err)
			return "", err
		}
		os.MkdirAll(filepath.Dir(name), 0755)
		if err := os.WriteFile(name, []byte(ver), 0644); err != nil {
			log.Printf("error writing version file: %v", err)
			return "", err
		}
		log.Printf("updated version file to %q", ver)
		return "", nil
	}

	_, err = forks.Maybe("update", 1*time.Hour)
	if err != nil {
		// If the child process fails, it is okay to continue; log the
		// error if verbose is set, but always swallow the error.
		return "", err
	}

	ver, err := os.ReadFile(name)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // prevent verbose log output about this
		}
		// This means the file is bad, etc. It does not matter, we'll
		// try again next time.
		return "", err
	}

	latest = string(ver)
	current := "v" + strings.TrimSpace(tierroot.Version)
	vlogf("latest version: %s, current version: %s", latest, current)
	vlogf("update: %v", semver.Compare(latest, current))
	if semver.Compare(latest, current) > 0 {
		return latest, nil
	}
	return "", nil
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
		Latest string
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", err
	}
	return v.Latest, nil
}
