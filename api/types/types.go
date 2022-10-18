package types

import "time"

type UsageResponse struct {
	Org   string               `json:"org"`
	Usage []UsageUsageResponse `json:"usage"`
}

// stutter to avoid conflict with tier.Usage for easier searching
type UsageUsageResponse struct {
	Feature string    `json:"feature"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Used    int       `json:"used"`
	Limit   int       `json:"limit"`
}
