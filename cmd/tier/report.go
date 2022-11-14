package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"go4.org/types"
	"tier.run/envknobs"
)

type event struct {
	TraceID string
	ID      string

	Type  string
	Name  string
	Start types.Time3339
	End   types.Time3339
	Err   string

	AccountID   string
	DisplayName string
	DeviceName  string
	Version     string

	GOOS   string
	GOARCH string

	IsHomebrewInstall bool // set by send
}

var doNotTrack, _ = strconv.ParseBool(os.Getenv("DO_NOT_TRACK"))

var vhs struct {
	sync.Mutex
	buf strings.Builder
	enc *json.Encoder
}

func trackEvent(ev *event) {
	vlogf("tracking: %v", ev)
	if doNotTrack {
		return
	}
	vhs.Lock()
	defer vhs.Unlock()
	if vhs.enc == nil {
		vhs.enc = json.NewEncoder(&vhs.buf)
	}
	ev.IsHomebrewInstall = isHomebrewInstall()
	if err := vhs.enc.Encode(ev); err != nil {
		panic(err)
	}
}

func sendEvents() error {
	if doNotTrack {
		vlogf("not sending events because DO_NOT_TRACK is set")
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	urlStr, err := url.JoinPath(envknobs.TrackingBaseURL(), "api/t")
	if err != nil {
		return err
	}
	body := strings.NewReader(os.Getenv("_TIER_EVENTS"))
	vlogf("sending events to %v", urlStr)
	vlogf("events: %v", os.Getenv("_TIER_EVENTS"))
	req, err := http.NewRequestWithContext(ctx, "POST", urlStr, body)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "tier/1") // TODO(bmizerany): include version, commit, etc (already in event.Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// TODO(bmizerany): log to some logfile
		return err
	}
	resp.Body.Close()
	return nil
}

func isHomebrewInstall() bool {
	// a little crude but it works well enough
	p, _ := os.Executable()
	return strings.Contains(p, "/Cellar/") || strings.Contains(p, "/homebrew/")
}
