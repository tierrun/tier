package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kr/pretty"
	"tier.run/api/apitypes"
	"tier.run/api/materialize"
	"tier.run/control"
	"tier.run/stripe"
	"tier.run/trweb"
	"tier.run/values"
)

// HTTP Errors
var errorLookup = map[error]error{
	control.ErrOrgNotFound: &trweb.HTTPError{
		Status:  400,
		Code:    "org_not_found",
		Message: "org not found",
	},
	control.ErrFeatureNotFound: &trweb.HTTPError{
		Status:  400,
		Code:    "feature_not_found",
		Message: "feature not found",
	},
	control.ErrFeatureNotMetered: &trweb.HTTPError{
		Status:  400,
		Code:    "invalid_request",
		Message: "feature not reportable",
	},
	stripe.ErrInvalidAPIKey: &trweb.HTTPError{
		Status:  401,
		Code:    "invalid_api_key",
		Message: "invalid api key",
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
	c      *control.Client
	helper func()
}

func NewHandler(c *control.Client, logf func(string, ...any)) *Handler {
	return &Handler{c: c, Logf: logf, helper: func() {}}
}

func isInvalidAccount(err error) bool {
	var e *stripe.Error
	return errors.As(err, &e) && e.Code == "account_invalid"
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	bw := &byteCountResponseWriter{ResponseWriter: w}
	err = h.serve(bw, r)
	if isInvalidAccount(err) {
		trweb.WriteError(w, &trweb.HTTPError{
			Status: 401,
			Code:   "account_invalid",
		})
		return
	}
	if trweb.WriteError(w, lookupErr(err)) || trweb.WriteError(w, err) {
		return
	}
	var e *control.ValidationError
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
	if bw.n == 0 {
		io.WriteString(w, "{}")
	}
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) error {
	switch r.URL.Path {
	case "/v1/whoami":
		return h.serveWhoAmI(w, r)
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
	case "/v1/push":
		return h.servePush(w, r)
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

	if !sr.Phases[0].Effective.IsZero() {
		return invalidRequestf("effective must not be specified for the first phase; for now")
	}

	m, err := h.c.Pull(r.Context(), 0)
	if err != nil {
		return err
	}

	var phases []control.Phase
	for _, p := range sr.Phases {
		fs, err := control.Expand(m, p.Features...)
		if err != nil {
			return err
		}
		phases = append(phases, control.Phase{
			Effective: p.Effective,
			Features:  fs,
		})
	}

	return h.c.ScheduleNow(r.Context(), sr.Org, phases)
}

func (h *Handler) serveReport(w http.ResponseWriter, r *http.Request) error {
	var rr apitypes.ReportRequest
	if err := trweb.DecodeStrict(r, &rr); err != nil {
		return err
	}

	return h.c.ReportUsage(r.Context(), rr.Org, rr.Feature, control.Report{
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

func (h *Handler) serveWhoAmI(w http.ResponseWriter, r *http.Request) error {
	who, err := h.c.WhoAmI(r.Context())
	if err != nil {
		return err
	}
	return httpJSON(w, apitypes.WhoAmIResponse{
		ProviderID: who.ProviderID,
		Email:      who.Email,
		Created:    who.Created(),
		KeySource:  who.KeySource,
		Isolated:   who.Isolated,
		URL:        who.URL(),
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
				Features:  p.Features,
				Plans:     p.Plans,
				Fragments: p.Fragments(),
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
			Feature: u.Feature.Name(),
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

func (h *Handler) servePush(w http.ResponseWriter, r *http.Request) error {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	fs, err := materialize.FromPricingHuJSON(data)
	if err != nil {
		return err
	}
	var ee []apitypes.PushResult
	_ = h.c.Push(r.Context(), fs, func(f control.Feature, err error) {
		pr := apitypes.PushResult{
			Feature: f.FeaturePlan,
		}
		switch err {
		case nil:
			pr.Status = "ok"
			pr.Reason = "created"
		case control.ErrFeatureExists:
			pr.Status = "ok"
			pr.Reason = "feature already exists"
		default:
			pr.Status = "failed"
			pr.Reason = err.Error()
		}
		ee = append(ee, pr)
	})
	return httpJSON(w, apitypes.PushResponse{Results: ee})
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

type byteCountResponseWriter struct {
	http.ResponseWriter
	n int
}

func (w *byteCountResponseWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.n += n
	return n, err
}
