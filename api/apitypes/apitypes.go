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

type Coupon struct {
	ID               string `json:"id"`
	Metadata         map[string]string
	Created          time.Time
	AmountOff        int     `json:"amount_off,omitempty"`
	Currency         string  `json:"currency,omitempty"`
	Duration         string  `json:"duration,omitempty"`
	DurationInMonths int     `json:"duration_in_months,omitempty"`
	MaxRedemptions   int     `json:"max_redemptions,omitempty"`
	Name             string  `json:"name,omitempty"`
	PercentOff       float64 `json:"percent_off,omitempty"`
	RedeemBy         time.Time
	TimesRedeemed    int  `json:"times_redeemed,omitempty"`
	Valid            bool `json:"valid,omitempty"`
}

type Phase struct {
	Trial      bool      `json:"trial,omitempty"`
	Effective  time.Time `json:"effective,omitempty"`
	Features   []string  `json:"features,omitempty"`
	Coupon     string    `json:"coupon,omitempty"`
	CouponData *Coupon   `json:"coupon_data,omitempty"`
}

type Taxation struct {
	Automatic bool `json:"automatic,omitempty"`
	CollectID bool `json:"collect_id,omitempty"` // specifics is taxID is collected
}

type Period struct {
	Effective time.Time `json:"effective,omitempty"`
	End       time.Time `json:"end,omitempty"`
}

func (p *Period) IsZero() bool {
	return p.Effective.IsZero() && p.End.IsZero()
}

type PhasesResponse struct {
	Current Period          `json:"current,omitempty"`
	Phases  []PhaseResponse `json:"phases"`
}

// The PhaseResponse is a response with all current phase fields exposed as
// top-level fields. Clients that need all phase data should use the Phases
// field.
type PhaseResponse struct {
	Effective  time.Time          `json:"effective,omitempty"`
	End        time.Time          `json:"end,omitempty"`
	Features   []refs.FeaturePlan `json:"features,omitempty"`
	Plans      []refs.Plan        `json:"plans,omitempty"`
	Fragments  []refs.FeaturePlan `json:"fragments,omitempty"`
	Trial      bool               `json:"trial,omitempty"`
	Tax        Taxation           `json:"tax,omitempty"`
	Current    Period             `json:"current,omitempty"` // not set on PhasesResponse
	Coupon     string             `json:"coupon,omitempty"`
	CouponData *Coupon            `json:"coupon_data,omitempty"`
}

func (pr PhaseResponse) MarshalJSON() ([]byte, error) {
	type Alias PhaseResponse
	return json.Marshal(&struct {
		*Alias
		Effective any `json:"effective,omitempty"`
		End       any `json:"end,omitempty"`
		Current   any `json:"current,omitempty"`
		Tax       any `json:"tax,omitempty"`
		Coupon    any `json:"coupon,omitempty"`
	}{
		Alias:     (*Alias)(&pr),
		Effective: nilIfZero(pr.Effective),
		End:       nilIfZero(pr.End),
		Current:   nilIfZero(pr.Current),
		Tax:       nilIfZero(pr.Tax),
		Coupon:    nilIfZero(pr.Coupon),
	})
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
	UpdateCustomerAddress bool     `json:"update_customer_address"`
	Tax                   Taxation `json:"tax"`
}

type ScheduleRequest struct {
	Org             string   `json:"org"`
	PaymentMethodID string   `json:"payment_method_id"`
	Info            *OrgInfo `json:"info"`
	Phases          []Phase  `json:"phases"`
	Tax             Taxation `json:"tax"`
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

func nilIfZero[T comparable](v T) any {
	if z, ok := any(v).(interface{ IsZero() bool }); ok && z.IsZero() {
		return nil
	}
	var zero T
	if v == zero {
		return nil
	}
	return v
}
