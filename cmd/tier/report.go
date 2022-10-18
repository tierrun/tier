package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptrace"

	"go4.org/types"
	"golang.org/x/sync/errgroup"
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
}

var report = &reporter{}

func init() {
	report.init()
}

// reporter reports events to the telemetry server. It is designed immediately
// wake the telemetry server while operations are in progress so that when
// events can be sent quickly just before the process exits, preventing awkward
// lags at the end of commands.
type reporter struct {
	g errgroup.Group
}

func (r *reporter) init() {
	r.g.SetLimit(50)                  // arbitrary; large enough to avoid blocking
	r.send(context.Background(), nil) // wake up the telemetry server
}

func (r *reporter) send(ctx context.Context, ev *event) {
	ok := r.g.TryGo(func() error {
		data, err := json.Marshal(ev)
		if err != nil {
			vvlogf("report: %v", err)
			return nil
		}

		done := make(chan struct{})
		ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
			WroteRequest: func(info httptrace.WroteRequestInfo) {
				vvlogf("report: err: %v", info.Err)
				// one the request has been written, we're done
				// and callers may unblock before a response is
				// received
				close(done)
			},
		})

		go func() {
			vvlogf("sending event: %v", ev)
			req, err := http.NewRequestWithContext(ctx, "POST", "https://tele.tier.run/api/t", bytes.NewReader(data))
			if err != nil {
				panic(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", "tier/1") // TODO(bmizerany): include version, commit, etc (already in event.Version)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				vvlogf("error sending events: %v", err)
				return
			}
			resp.Body.Close()
			vvlogf("sent events: %v", resp.Status)
		}()

		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if !ok {
		vvlogf("report: event dropped")
	}
}

// flush sends pending events to the server; it blocks until all events are sent
//
// It is an error to call flush before init.
func (r *reporter) flush() error {
	return r.g.Wait() // hook up a context?
}
