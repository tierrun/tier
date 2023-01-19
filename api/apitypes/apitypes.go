package apitypes

import (
	"fmt"
	"time"

	"tier.run/refs"
)

type Error struct {
	Status  int    `json:"status"`
	Code    string `json:"code"` // (e.g. "invalid_request")
	Message string `json:"message"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("httpError{status:%d code:%q message:%q}",
		e.Status, e.Code, e.Message)
}

type Phase struct {
	Trial     bool
	Effective time.Time
	Features  []string
}

type PhaseResponse struct {
	Effective time.Time          `json:"effective,omitempty"`
	Features  []refs.FeaturePlan `json:"features,omitempty"`
	Plans     []refs.Plan        `json:"plans,omitempty"`
	Fragments []refs.FeaturePlan `json:"fragments,omitempty"`
}

type InvoiceSettings struct {
	DefaultPaymentMethod string `json:"default_payment_method"`
}

type OrgInfo struct {
	Email       string            `json:"email"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Phone       string            `json:"phone"`
	Metadata    map[string]string `json:"metadata"`

	PaymentMethod   string          `json:"payment_method"`
	InvoiceSettings InvoiceSettings `json:"invoice_settings"`
}

type CheckoutParams struct {
	SuccessURL string `json:"success_url"`
	CancelURL  string `json:"cancel_url"`
}

type ScheduleRequest struct {
	Org    string
	Info   *OrgInfo
	Phases []Phase

	Checkout *CheckoutParams
}

type ScheduleResponse struct {
	CheckoutURL string `json:"checkout_url,omitempty"`
}

type ReportRequest struct {
	Org     string
	Feature refs.Name
	N       int
	At      time.Time // default is time.Now()
	Clobber bool
}

type WhoIsResponse struct {
	*OrgInfo
	Org      string `json:"org"`
	StripeID string `json:"stripe_id"`
}

type UsageResponse struct {
	Org   string  `json:"org"`
	Usage []Usage `json:"usage"`
}

type Usage struct {
	Feature refs.Name `json:"feature"`
	Used    int       `json:"used"`
	Limit   int       `json:"limit"`
}

func UsageByFeature(a, b Usage) bool {
	return a.Feature.Less(b.Feature)
}

type PushResult struct {
	Feature refs.FeaturePlan `json:"feature"`
	Status  string           `json:"status"`
	Reason  string           `json:"reason"`
}

type PushResponse struct {
	Results []PushResult `json:"results,omitempty"`
}

type WhoAmIResponse struct {
	ProviderID string    `json:"id"`
	Email      string    `json:"email"`
	Created    time.Time `json:"created"`
	KeySource  string    `json:"key_source"`
	Isolated   bool      `json:"isolated"`
	URL        string    `json:"url"`
}
