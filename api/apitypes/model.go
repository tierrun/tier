package apitypes

import (
	"encoding/json"

	"tier.run/refs"
	"tier.run/types/tax"
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

type Divide struct {
	By       int    `json:"by,omitempty"`
	Rounding string `json:"rounding,omitempty"`
}

type Feature struct {
	Title     string       `json:"title,omitempty"`
	Base      float64      `json:"base,omitempty"`
	Mode      string       `json:"mode,omitempty"`
	Aggregate string       `json:"aggregate,omitempty"`
	Tiers     []Tier       `json:"tiers,omitempty"`
	Divide    Divide       `json:"divide"`
	Tax       tax.Settings `json:"tax"`
}

func (v Feature) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Title     string        `json:"title,omitempty"`
		Base      float64       `json:"base,omitempty"`
		Mode      string        `json:"mode,omitempty"`
		Aggregate string        `json:"aggregate,omitempty"`
		Tiers     []Tier        `json:"tiers,omitempty"`
		Divide    *Divide       `json:"divide,omitempty"`
		Tax       *tax.Settings `json:"tax,omitempty"`
	}{
		Title:     v.Title,
		Base:      v.Base,
		Mode:      v.Mode,
		Aggregate: v.Aggregate,
		Tiers:     v.Tiers,
		Divide:    zeroAsNil(v.Divide),
		Tax:       zeroAsNil(v.Tax),
	})
}

// zeroAsNil returns a pointer to v if v is not the zero value for its type.
// If v implements IsZero, it is used to determine if v is the zero value.
func zeroAsNil[T comparable](v T) *T {
	z, ok := any(v).(interface{ IsZero() bool })
	if ok && z.IsZero() {
		return nil
	}
	var zero T
	if v == zero {
		return nil
	}
	return &v
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
