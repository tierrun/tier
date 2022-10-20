package apitypes

import "time"

type Phase struct {
	Effective time.Time
	Features  []string
}

type SubscribeRequest struct {
	Org    string
	Phases []Phase
}

type PhasesResponse struct {
	Org    string
	Phases []Phase
}

type ReportRequest struct {
	Org     string
	Feature string
	N       int
	At      time.Time
	Clobber bool
}

type WhoIsResponse struct {
	Org      string `json:"org"`
	StripeID string `json:"stripe_id"`
}

type UsageResponse struct {
	Org   string  `json:"org"`
	Usage []Usage `json:"usage"`
}

type Usage struct {
	Feature string `json:"feature"`
	Used    int    `json:"used"`
	Limit   int    `json:"limit"`
}
