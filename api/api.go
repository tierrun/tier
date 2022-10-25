package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/kr/pretty"
	"tier.run/api/apitypes"
	"tier.run/client/tier"
	"tier.run/trweb"
	"tier.run/values"
)

// HTTP Errors
var lookupErr = map[error]error{
	tier.ErrOrgNotFound: &trweb.HTTPError{
		Status:  400,
		Code:    "org_not_found",
		Message: "org not found",
	},
	tier.ErrFeatureNotFound: &trweb.HTTPError{
		Status:  400,
		Code:    "feature_not_found",
		Message: "feature not found",
	},
	tier.ErrFeatureNotMetered: &trweb.HTTPError{
		Status:  400,
		Code:    "invalid_request",
		Message: "feature not reportable",
	},
}

type Handler struct {
	Logf   func(format string, args ...any)
	c      *tier.Client
	helper func()
}

func NewHandler(c *tier.Client, logf func(string, ...any)) *Handler {
	return &Handler{c: c, Logf: logf}
}

func (h *Handler) logf(format string, args ...interface{}) {
	if h.helper != nil {
		h.helper()
	}
	if h.Logf != nil {
		h.Logf(format, args...)
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := h.serve(w, r)
	if err != nil {
		h.logf("error: %v", err)
		// continue processing error below
	}
	if trweb.WriteError(w, lookupErr[err]) || trweb.WriteError(w, err) {
		return
	}

	var e *tier.ValidationError
	if errors.As(err, &e) {
		trweb.WriteError(w, &trweb.HTTPError{
			Status:  400,
			Code:    "invalid_request",
			Message: e.Message,
		})
		return
	}

	if err != nil {
		trweb.WriteError(w, trweb.InternalError)
		return
	}
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) error {
	switch r.URL.Path {
	case "/v1/whois":
		return h.serveWhoIs(w, r)
	case "/v1/limits":
		return h.serveLimits(w, r)
	case "/v1/report":
		return h.serveReport(w, r)
	case "/v1/subscribe":
		return h.serveSubscribe(w, r)
	case "/v1/phase":
		return h.servePhase(w, r)
	default:
		return trweb.NotFound
	}
}

func (h *Handler) serveSubscribe(w http.ResponseWriter, r *http.Request) error {
	var sr apitypes.SubscribeRequest
	if err := trweb.DecodeStrict(r, &sr); err != nil {
		return err
	}

	// currently /v1/subscribe API only supports updating the current phase
	// (or starting a new one). In the future, this endpoint will respect a
	// Currnet field on Phase and go into "advanced" mode allowing the
	// client to define how to update the schedule in stripe.
	if len(sr.Phases) != 1 {
		return invalidRequest("phases must contain exactly one phase; for now")
	}

	p := sr.Phases[0]
	if !p.Effective.IsZero() {
		return invalidRequest("effective must not be specified; for now")
	}
	if len(p.Features) > 1 {
		return invalidRequest("phase must not have more than one plan; for now")
	}
	if len(p.Features) == 0 {
		return invalidRequest("phase must have at least one plan")
	}

	// TODO(bmizerany): decide if we should detach the context here and let
	// subscribe keep going in the background for some time before a
	// response is ready.
	// select {
	// case <-r.Context().Done():
	// case <-time.After(1 * time.Second):
	// 	return 204
	// }

	plan := p.Features[0]
	return h.c.SubscribeToPlan(r.Context(), sr.Org, plan)
}

func (h *Handler) serveReport(w http.ResponseWriter, r *http.Request) error {
	var rr apitypes.ReportRequest
	if err := trweb.DecodeStrict(r, &rr); err != nil {
		return err
	}
	return h.c.ReportUsage(r.Context(), rr.Org, rr.Feature, tier.Report{
		N:       rr.N,
		At:      values.Coalesce(rr.At, time.Now()),
		Clobber: rr.Clobber,
	})
}

func (h *Handler) serveWhoIs(w http.ResponseWriter, r *http.Request) error {
	org := r.FormValue("org")
	stripeID, err := h.c.WhoIs(r.Context(), org)
	if err != nil {
		return err
	}
	return httpJSON(w, apitypes.WhoIsResponse{
		Org:      org,
		StripeID: stripeID,
	})
}

func (h *Handler) servePhase(w http.ResponseWriter, r *http.Request) error {
	org := r.FormValue("org")
	ps, err := h.c.LookupPhases(r.Context(), org)
	if err != nil {
		return err
	}

	h.Logf("lookup phases: %# v", pretty.Formatter(ps))

	for _, p := range ps {
		if p.Current {
			fs := values.MapFunc(p.Features, func(f tier.Feature) string {
				return f.FQN()
			})
			return httpJSON(w, apitypes.PhaseResponse{
				Effective: p.Effective,
				Features:  fs,
				Plans:     p.Plans,
				Fragments: values.MapFunc(p.Fragments(), func(f tier.Feature) string {
					return f.FQN()
				}),
			})
		}
	}

	return trweb.NotFound
}

func (h *Handler) serveLimits(w http.ResponseWriter, r *http.Request) error {
	org := r.FormValue("org")
	usage, err := h.c.LookupLimits(r.Context(), org)
	if err != nil {
		return err
	}

	var rr apitypes.UsageResponse
	rr.Org = org
	for _, u := range usage {
		rr.Usage = append(rr.Usage, apitypes.Usage{
			Feature: u.Feature,
			Limit:   u.Limit,
			Used:    u.Used,
		})
	}

	return httpJSON(w, rr)
}

func httpJSON(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "\t")
	return enc.Encode(v)
}

func invalidRequest(reason string) *trweb.HTTPError {
	return &trweb.HTTPError{
		Status:  400,
		Code:    "invalid_request",
		Message: reason,
	}
}
