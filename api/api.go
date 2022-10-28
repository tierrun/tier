package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/kr/pretty"
	"tier.run/api/apitypes"
	"tier.run/client/tier"
	"tier.run/materialize"
	"tier.run/refs"
	"tier.run/trweb"
	"tier.run/values"
)

// HTTP Errors
var errorLookup = map[error]error{
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

func lookupErr(err error) error {
	for {
		if e, ok := errorLookup[err]; ok {
			return e
		}
		if err = errors.Unwrap(err); err == nil {
			return nil
		}
	}
}

type Handler struct {
	Logf   func(format string, args ...any)
	c      *tier.Client
	helper func()
}

func NewHandler(c *tier.Client, logf func(string, ...any)) *Handler {
	return &Handler{c: c, Logf: logf, helper: func() {}}
}

func (h *Handler) logf(format string, args ...interface{}) {
	h.helper()
	if h.Logf != nil {
		h.Logf("api:"+format, args...)
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	var debug []string // TODO(bmizerany): pool to remove allocs
	dbg := func(msg string) {
		debug = append(debug, msg)
	}
	defer func() {
		h.logf("%s %s %s: %# v", r.Method, r.URL.Path, debug, pretty.Formatter(err))
	}()

	dbg("serve")
	err = h.serve(w, r)

	if trweb.WriteError(w, lookupErr(err)) || trweb.WriteError(w, err) {
		dbg("writeerr")
		return
	}

	var e *tier.ValidationError
	if errors.As(err, &e) {
		dbg("validationerr")
		trweb.WriteError(w, &trweb.HTTPError{
			Status:  400,
			Code:    "invalid_request",
			Message: e.Message,
		})
		return
	}

	if err != nil {
		dbg("catchallerr")
		trweb.WriteError(w, trweb.InternalError)
		return
	}

	dbg("writeok")
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
	case "/v1/pull":
		return h.servePull(w, r)
	default:
		return trweb.NotFound
	}
}

func (h *Handler) serveSubscribe(w http.ResponseWriter, r *http.Request) error {
	var sr apitypes.SubscribeRequest
	if err := trweb.DecodeStrict(r, &sr); err != nil {
		return err
	}

	if len(sr.Phases) == 0 {
		return invalidRequestf("a minimum of one phase is required")
	}

	expand := func(rs []string) ([]refs.FeaturePlan, error) {
		var expanded []refs.FeaturePlan
		for _, s := range rs {
			fp, err := refs.ParseFeaturePlan(s)
			if err != nil {
				pl, err := refs.ParsePlan(s)
				if err != nil {
					return nil, invalidRequestf("invalid feature plan: %q", s)
				}
				fps, err := h.c.ExpandPlan(r.Context(), pl)
				if err != nil {
					return nil, err
				}
				expanded = append(expanded, fps...)
			} else {
				expanded = append(expanded, fp)
			}
		}
		return expanded, nil
	}

	if !sr.Phases[0].Effective.IsZero() {
		return invalidRequestf("effective must not be specified for the first phase; for now")
	}

	var phases []tier.Phase
	for _, p := range sr.Phases {
		fps, err := expand(p.Features)
		if err != nil {
			return err
		}
		phases = append(phases, tier.Phase{
			Effective: p.Effective,
			Features:  fps,
		})
	}

	return h.c.Subscribe(r.Context(), sr.Org, phases)
}

func (h *Handler) serveReport(w http.ResponseWriter, r *http.Request) error {
	var rr apitypes.ReportRequest
	if err := trweb.DecodeStrict(r, &rr); err != nil {
		return err
	}

	fn, err := refs.ParseName(rr.Feature)
	if err != nil {
		return err
	}

	return h.c.ReportUsage(r.Context(), rr.Org, fn, tier.Report{
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
			return httpJSON(w, apitypes.PhaseResponse{
				Effective: p.Effective,
				Features:  values.Strings(p.Features),
				Plans:     values.Strings(p.Plans),
				Fragments: values.Strings(p.Fragments()),
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
			Feature: u.Feature.String(),
			Limit:   u.Limit,
			Used:    u.Used,
		})
	}

	return httpJSON(w, rr)
}

func (h *Handler) servePull(w http.ResponseWriter, r *http.Request) error {
	m, err := h.c.Pull(r.Context(), 0)
	if err != nil {
		return err
	}
	b, err := materialize.ToPricingJSON(m)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
	return nil
}

func httpJSON(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "\t")
	return enc.Encode(v)
}

func invalidRequestf(reason string, args ...any) *trweb.HTTPError {
	return &trweb.HTTPError{
		Status:  400,
		Code:    "invalid_request",
		Message: fmt.Sprintf(reason, args...),
	}
}
