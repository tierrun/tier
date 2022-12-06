package apitypes

import (
	"encoding/json"

	"tier.run/refs"
	"tier.run/values"
)

const Inf = 1<<63 - 1

type Tier struct {
	Upto  int     `json:"upto,omitempty"`
	Price float64 `json:"price,omitempty"`
	Base  int     `json:"base,omitempty"`
}

func (t *Tier) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Upto  int     `json:"upto,omitempty"`
		Price float64 `json:"price,omitempty"`
		Base  int     `json:"base,omitempty"`
	}{
		Upto:  values.ZeroIf(t.Upto, Inf),
		Price: t.Price,
		Base:  t.Base,
	})
}

func (t *Tier) UnmarshalJSON(data []byte) error {
	*t = Tier{}
	var v struct {
		Upto  *int
		Price float64
		Base  int
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	if v.Upto == nil {
		t.Upto = Inf
	} else {
		t.Upto = *v.Upto
	}
	t.Price = v.Price
	t.Base = v.Base
	return nil
}

type Feature struct {
	Title     string `json:"title,omitempty"`
	Base      int    `json:"base,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Aggregate string `json:"aggregate,omitempty"`
	Tiers     []Tier `json:"tiers,omitempty"`
	PermLink  string `json:"permLink,omitempty"`
}

type Plan struct {
	Title    string                `json:"title,omitempty"`
	Interval string                `json:"interval,omitempty"`
	Currency string                `json:"currency,omitempty"`
	Features map[refs.Name]Feature `json:"features,omitempty"`
}

type Model struct {
	Plans map[refs.Plan]Plan `json:"plans"`
}
