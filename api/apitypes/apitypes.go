package types

type UsageResponse struct {
	Org   string  `json:"org"`
	Usage []Usage `json:"usage"`
}

// stutter to avoid conflict with tier.Usage for easier searching
type Usage struct {
	Feature string `json:"feature"`
	Used    int    `json:"used"`
	Limit   int    `json:"limit"`
}
