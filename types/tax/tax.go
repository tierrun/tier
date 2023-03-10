package tax

type Settings struct {
	Included bool `json:"included,omitempty"`
}

type Applied struct {
	Automatically bool `json:"automatically,omitempty"`
}
