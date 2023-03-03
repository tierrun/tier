package apitypes

import (
	"encoding/json"
	"fmt"
	"time"

	"tier.run/refs"
	"tier.run/types/payment"
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
	Trial     bool      `json:"trial,omitempty"`
	Effective time.Time `json:"effective,omitempty"`
	Features  []string  `json:"features,omitempty"`
}

type PhaseResponse struct {
	Effective time.Time          `json:"effective,omitempty"`
	End       time.Time          `json:"end,omitempty"`
	Features  []refs.FeaturePlan `json:"features,omitempty"`
	Plans     []refs.Plan        `json:"plans,omitempty"`
	Fragments []refs.FeaturePlan `json:"fragments,omitempty"`
	Trial     bool               `json:"trial,omitempty"`
}

func (pr PhaseResponse) MarshalJSON() ([]byte, error) {
	type Alias PhaseResponse
	if pr.End.IsZero() {
		return json.Marshal(&struct {
			*Alias
			End byte `json:"end,omitempty"`
		}{
			Alias: (*Alias)(&pr),
		})
	} else {
		return json.Marshal(&struct {
			*Alias
			End time.Time `json:"end"`
		}{
			Alias: (*Alias)(&pr),
			End:   pr.End,
		})
	}
}

type PaymentMethodsResponse struct {
	Org            string           `json:"org"`
	PaymentMethods []payment.Method `json:"methods"`
}

type InvoiceSettings struct {
	DefaultPaymentMethod string `json:"default_payment_method"`
}

type OrgInfo struct {
	Email       string            `json:"email"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Phone       string            `json:"phone"`
	Created     time.Time         `json:"created"`
	Metadata    map[string]string `json:"metadata"`

	PaymentMethod   string          `json:"payment_method"`
	InvoiceSettings InvoiceSettings `json:"invoice_settings"`
}

type CheckoutRequest struct {
	Org                   string   `json:"org"`
	TrialDays             int      `json:"trial_days"`
	Features              []string `json:"features"`
	SuccessURL            string   `json:"success_url"`
	CancelURL             string   `json:"cancel_url"`
	RequireBillingAddress bool     `json:"require_billing_address"`
}

type ScheduleRequest struct {
	Org             string   `json:"org"`
	PaymentMethodID string   `json:"payment_method_id"`
	Info            *OrgInfo `json:"info"`
	Phases          []Phase  `json:"phases"`
}

// ScheduleResponse is the expected response from a schedule request. It is
// currently empty, reserved for furture use.
type ScheduleResponse struct{}

type CheckoutResponse struct {
	URL string `json:"url"`
}

type ReportRequest struct {
	Org     string    `json:"org"`
	Feature refs.Name `json:"feature"`
	N       int       `json:"n"`
	At      time.Time `json:"at"`
	Clobber bool      `json:"clobber"`
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

type ClockRequest struct {
	ID      string
	Name    string
	Present time.Time
}

type ClockResponse struct {
	ID      string    `json:"id"`
	Link    string    `json:"link"`
	Present time.Time `json:"present"`
	Status  string    `json:"status"`
}
