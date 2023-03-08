package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kr/pretty"
	"golang.org/x/exp/slices"
	"tier.run/api/apitypes"
	"tier.run/api/materialize"
	"tier.run/client/tier"
	"tier.run/control"
	"tier.run/refs"
	"tier.run/stripe"
	"tier.run/trweb"
)

func init() {
	// check that all error codes are unique
	seen := map[string]bool{}
	for _, e := range errorLookup {
		if seen[e.Code] {
			panic("duplicate error message")
		}
		if seen[e.Code] {
			panic(fmt.Sprintf("duplicate error code %q", e.Code))
		}
		seen[e.Code] = true
	}
}

// HTTP Errors
//
// TERR1000: invalid requests
// TERR1010: org or features problem
// TERR2000: internal errors
var errorLookup = map[error]*trweb.HTTPError{
	control.ErrOrgNotFound: {
		Status:  400,
		Code:    "org_not_found",
		Message: "org not found",
	},
	control.ErrFeatureNotFound: {
		Status:  400,
		Code:    "feature_not_found",
		Message: "feature not found",
	},
	control.ErrNoFeatures: {
		Status:  400,
		Code:    "TERR1020",
		Message: "feature or plan not found",
	},
	control.ErrUnexpectedMissingOrg: {
		Status:  500,
		Code:    "TERR1050",
		Message: "Stripe reported a customer was created and then reported it did not exist. This might mean you purged your Test Mode and need to reset TIER_PREFIX_KEY=<randomString>.",
	},
	control.ErrFeatureNotMetered: { // TODO(bmizerany): this may be relaxed if we decide to log and accept
		Status:  400,
		Code:    "invalid_request",
		Message: "feature not reportable",
	},
	control.ErrInvalidEmail: {
		Status:  400,
		Code:    "invalid_email",
		Message: "invalid email",
	},
	control.ErrInvalidMetadata: {
		Status:  400,
		Code:    "invalid_metadata",
		Message: "metadata keys must not use reserved prefix ('tier.')",
	},
	stripe.ErrInvalidAPIKey: {
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
	if err != nil {
		h.Logf("%s %s %s %s: %v", r.RemoteAddr, r.Method, r.Host, r.URL, err)
	}

	if isInvalidAccount(err) {
		trweb.WriteError(w, &trweb.HTTPError{
			Status: 401,
			Code:   "account_invalid",
		})
		return
	}

	var ipe *stripe.Error
	if errors.As(err, &ipe) && strings.Contains(ipe.Message, "No such PaymentMethod") {
		trweb.WriteError(w, &trweb.HTTPError{
			Status:  400,
			Code:    "invalid_payment_method",
			Message: ipe.Message,
		})
		return
	}

	var rawStripeErr *stripe.Error
	if errors.As(err, &rawStripeErr) {
		trweb.WriteError(w, &trweb.HTTPError{
			Status:   502,
			Original: rawStripeErr,
		})
		return
	}

	if trweb.WriteError(w, lookupErr(err)) || trweb.WriteError(w, err) {
		return
	}
	var ve *control.ValidationError
	if errors.As(err, &ve) {
		trweb.WriteError(w, &trweb.HTTPError{
			Status:  400,
			Code:    "invalid_request",
			Message: ve.Message,
		})
		return
	}
	var pe *refs.ParseError
	if errors.As(err, &pe) {
		trweb.WriteError(w, &trweb.HTTPError{
			Status:  400,
			Code:    "invalid_request",
			Message: pe.Message,
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
	clockID := r.Header.Get(tier.ClockHeader)
	r = r.Clone(control.WithClock(r.Context(), clockID))

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
	case "/v1/checkout":
		return h.serveCheckout(w, r)
	case "/v1/phase":
		return h.servePhase(w, r)
	case "/v1/pull":
		return h.servePull(w, r)
	case "/v1/push":
		return h.servePush(w, r)
	case "/v1/payment_methods":
		return h.servePaymentMethods(w, r)
	case "/v1/clock":
		return h.serveClock(w, r)
	default:
		return trweb.NotFound
	}
}

func (h *Handler) serveCheckout(w http.ResponseWriter, r *http.Request) error {
	var cr apitypes.CheckoutRequest
	if err := trweb.DecodeStrict(r, &cr); err != nil {
		return err
	}
	m, err := h.c.Pull(r.Context(), 0)
	if err != nil {
		return err
	}
	fs, err := control.ExpandPlans(m, cr.Features...)
	if err != nil {
		return err
	}
	link, err := h.c.Checkout(r.Context(), cr.Org, cr.SuccessURL, &control.CheckoutParams{
		TrialDays:             cr.TrialDays,
		Features:              fs,
		CancelURL:             cr.CancelURL,
		RequireBillingAddress: cr.RequireBillingAddress,
	})
	if err != nil {
		return err
	}
	return httpJSON(w, &apitypes.CheckoutResponse{URL: link})
}

func (h *Handler) serveSubscribe(w http.ResponseWriter, r *http.Request) error {
	var sr apitypes.ScheduleRequest
	if err := trweb.DecodeStrict(r, &sr); err != nil {
		return err
	}
	if sr.Info != nil {
		info := infoToOrgInfo(sr.Info)
		if err := h.c.PutCustomer(r.Context(), sr.Org, info); err != nil {
			return err
		}
	}
	if len(sr.Phases) == 0 {
		return nil
	}

	var phases []control.Phase
	if len(sr.Phases) > 0 {
		m, err := h.c.Pull(r.Context(), 0)
		if err != nil {
			return err
		}

		for _, p := range sr.Phases {
			fs, err := control.Expand(m, p.Features...)
			if err != nil {
				return err
			}
			phases = append(phases, control.Phase{
				Trial:     p.Trial,
				Effective: p.Effective,
				Features:  fs,
			})
		}
	}

	return h.c.Schedule(r.Context(), sr.Org, control.ScheduleParams{
		PaymentMethod: sr.PaymentMethodID,
		Phases:        phases,
	})
}

func (h *Handler) serveReport(w http.ResponseWriter, r *http.Request) error {
	var rr apitypes.ReportRequest
	if err := trweb.DecodeStrict(r, &rr); err != nil {
		return err
	}

	return h.c.ReportUsage(r.Context(), rr.Org, rr.Feature, control.Report{
		N:       rr.N,
		At:      rr.At,
		Clobber: rr.Clobber,
	})
}

func (h *Handler) serveWhoIs(w http.ResponseWriter, r *http.Request) error {
	org := r.FormValue("org")
	stripeID, err := h.c.WhoIs(r.Context(), org)
	if err != nil {
		return err
	}

	res := &apitypes.WhoIsResponse{Org: org, StripeID: stripeID}
	inc := r.URL.Query()["include"]
	if slices.Contains(inc, "info") {
		info, err := h.c.LookupOrg(r.Context(), org)
		if err != nil {
			return err
		}
		res.OrgInfo = &apitypes.OrgInfo{
			Email:           info.Email,
			Name:            info.Name,
			Description:     info.Description,
			Phone:           info.Phone,
			Created:         info.CreatedAt(),
			Metadata:        info.Metadata,
			PaymentMethod:   info.PaymentMethod,
			InvoiceSettings: apitypes.InvoiceSettings(info.InvoiceSettings),
		}
	}

	return httpJSON(w, res)
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

	for i, p := range ps {
		if p.Current {
			var end time.Time
			if i+1 < len(ps) {
				end = ps[i+1].Effective
			}
			return httpJSON(w, apitypes.PhaseResponse{
				Effective: p.Effective,
				End:       end,
				Features:  p.Features,
				Plans:     p.Plans,
				Fragments: p.Fragments(),
				Trial:     p.Trial,
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

func (h *Handler) servePaymentMethods(w http.ResponseWriter, r *http.Request) error {
	org := r.FormValue("org")

	pms, err := h.c.LookupPaymentMethods(r.Context(), org)
	if err != nil {
		return err
	}

	return httpJSON(w, apitypes.PaymentMethodsResponse{
		Org:            org,
		PaymentMethods: pms,
	})
}

func (h *Handler) serveClock(w http.ResponseWriter, r *http.Request) error {
	writeResp := func(c *control.Clock) error {
		return httpJSON(w, apitypes.ClockResponse{
			ID:      c.ID(),
			Link:    c.Link(),
			Present: c.Present(),
			Status:  c.Status(),
		})
	}

	switch r.Method {
	case "GET":
		clockID := r.FormValue("id")
		c := h.c.ClockFromID(clockID)
		if err := c.Sync(r.Context()); err != nil {
			return err
		}
		return writeResp(c)
	case "POST":
		var v apitypes.ClockRequest
		if err := trweb.DecodeStrict(r, &v); err != nil {
			return err
		}

		if v.ID == "" {
			c, err := h.c.NewClock(r.Context(), v.Name, v.Present)
			if err != nil {
				return err
			}
			return writeResp(c)
		} else {
			c := h.c.ClockFromID(v.ID)
			if err := c.Advance(r.Context(), v.Present); err != nil {
				return err
			}
			return writeResp(c)
		}
	default:
		return trweb.MethodNotAllowed
	}
}

func httpJSON(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "\t")
	return enc.Encode(v)
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

func infoToOrgInfo(info *apitypes.OrgInfo) *control.OrgInfo {
	return &control.OrgInfo{
		Email:           info.Email,
		Name:            info.Name,
		Description:     info.Description,
		Phone:           info.Phone,
		Metadata:        info.Metadata,
		PaymentMethod:   info.PaymentMethod,
		InvoiceSettings: control.InvoiceSettings(info.InvoiceSettings),
	}
}
