package api

import (
	"encoding/json"
	"net/http"
	"time"

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
	default:
		return trweb.NotFound
	}
}

func (h *Handler) serveSubscribe(w http.ResponseWriter, r *http.Request) error {
	var sr apitypes.SubscribeRequest
	if err := trweb.DecodeStrict(r, &sr); err != nil {
		return err
	}
	m, err := h.c.Pull(r.Context(), 0)
	if err != nil {
		return err
	}
	var phases []tier.Phase
	for _, p := range sr.Phases {
		if len(p.Features) > 1 {
			return &trweb.HTTPError{
				Status:  400,
				Code:    "invalid_phase",
				Message: "phase must not have more than one plan",
			}
		}
		if len(p.Features) == 0 {
			return &trweb.HTTPError{
				Status:  400,
				Code:    "invalid_request",
				Message: "phase must have at least one feature; this constraint will be lifted in the future",
			}
		}
		fs := findFeatures(m, p.Features)
		if len(fs) == 0 {
			return &trweb.HTTPError{
				Status:  400,
				Code:    "invalid_request",
				Message: "no features found for plan",
			}
		}
		phases = append(phases, tier.Phase{
			Effective: p.Effective,
			Features:  fs,
		})
	}
	return h.c.Subscribe(r.Context(), sr.Org, phases)
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

func (h *Handler) serveLimits(w http.ResponseWriter, r *http.Request) error {
	org := r.URL.Query().Get("org")
	usage, err := h.c.LookupUsage(r.Context(), org)
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

func findFeatures(fs []tier.Feature, names []string) []tier.Feature {
	if len(names) > 1 {
		panic("feature folding is not implemented")
	}
	var out []tier.Feature
	for _, f := range fs {
		for _, n := range names {
			// TODO(bmizerany): support versioned features, which
			// would include "plans" like ("feature:x@plan:free@0")
			if f.Plan == n {
				out = append(out, f)
			}
		}
	}
	return out
}
