package apitypes

import (
	"time"

	"tier.run/refs"
)

type Phase struct {
	Effective time.Time
	Features  []string
}

type PhaseResponse struct {
	Effective time.Time          `json:"effective,omitempty"`
	Features  []refs.FeaturePlan `json:"features,omitempty"`
	Plans     []refs.Plan        `json:"plans,omitempty"`
	Fragments []refs.FeaturePlan `json:"fragments,omitempty"`
}

type SubscribeRequest struct {
	Org    string
	Phases []Phase
}

type ReportRequest struct {
	Org     string
	Feature refs.Name
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
	Feature refs.Name `json:"feature"`
	Used    int       `json:"used"`
	Limit   int       `json:"limit"`
}

func UsageByFeature(a, b Usage) bool {
	return a.Feature.Less(b.Feature)
}
