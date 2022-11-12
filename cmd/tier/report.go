package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"blake.io/forks"
	"go4.org/types"
)

type event struct {
	TraceID string
	ID      string

	Type  string
	Name  string
	Start types.Time3339
	End   types.Time3339
	Err   error

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

func flushEvents() {
	// save events to temp json file
	if doNotTrack {
		return
	}
	vhs.Lock()
	defer vhs.Unlock()
	os.Setenv("_TIER_EVENTS", vhs.buf.String())
	_, err := forks.Maybe("track", 1*time.Second)
	if err != nil {
		// TODO(bmizerany): log to some logfile
		return
	}
}

func sendEvents() (sent bool) {
	if !forks.Child() {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	body := strings.NewReader(os.Getenv("_TIER_EVENTS"))
	req, err := http.NewRequestWithContext(ctx, "POST", "https://tele.tier.run/api/t", body)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "tier/1") // TODO(bmizerany): include version, commit, etc (already in event.Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// TODO(bmizerany): log to some logfile
		return
	}
	resp.Body.Close()
	return true
}

func isHomebrewInstall() bool {
	// a little crude but it works well enough
	p, _ := os.Executable()
	return strings.Contains(p, "/Cellar/") || strings.Contains(p, "/homebrew/")
}
