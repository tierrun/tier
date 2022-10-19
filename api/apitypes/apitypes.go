package apitypes

type WhoIsResponse struct {
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
