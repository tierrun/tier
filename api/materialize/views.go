// Package materialize provides utilities for materializing views from features
// pushed and pulled to billing engine providers.
package materialize

import (
	"bytes"
	"encoding/json"

	"github.com/tailscale/hujson"
	"kr.dev/errorfmt"
	"tier.run/client/tier"
	"tier.run/refs"
	"tier.run/values"
)

type jsonTier struct {
	Upto  int `json:"upto,omitempty"`
	Price int `json:"price,omitempty"`
	Base  int `json:"base,omitempty"`
}

func (t *jsonTier) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Upto  int `json:"upto,omitempty"`
		Price int `json:"price,omitempty"`
		Base  int `json:"base,omitempty"`
	}{
		Upto:  values.ZeroIf(t.Upto, tier.Inf),
		Price: t.Price,
		Base:  t.Base,
	})
}

func (t *jsonTier) UnmarshalJSON(data []byte) error {
	*t = jsonTier{}
	var v struct {
		Upto  *int
		Price int
		Base  int
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	if v.Upto == nil {
		t.Upto = tier.Inf
	} else {
		t.Upto = *v.Upto
	}
	t.Price = v.Price
	t.Base = v.Base
	return nil
}

type jsonFeature struct {
	Title     string     `json:"title,omitempty"`
	Base      int        `json:"base,omitempty"`
	Mode      string     `json:"mode,omitempty"`
	Aggregate string     `json:"aggregate,omitempty"`
	Tiers     []jsonTier `json:"tiers,omitempty"`
}

type jsonPlan struct {
	Title    string                    `json:"title,omitempty"`
	Interval string                    `json:"interval,omitempty"`
	Currency string                    `json:"currency,omitempty"`
	Features map[refs.Name]jsonFeature `json:"features,omitempty"`
}

type jsonModel struct {
	Plans map[refs.Plan]jsonPlan `json:"plans"`
}

func FromPricingHuJSON(data []byte) (fs []tier.Feature, err error) {
	var debug []string
	dbg := func(k string) {
		debug = append(debug, k)
	}
	defer errorfmt.Handlef("FromPricingHuJSON: %q: %w", &debug, &err)

	data, err = hujson.Standardize(data)
	if err != nil {
		return nil, err
	}

	var m jsonModel
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields() // we use a Decoder to get the DisallowUnknownFields method
	if err := dec.Decode(&m); err != nil {
		dbg("decodeerr")
		return nil, err
	}

	if err := validate(m); err != nil {
		return nil, err
	}

	for plan, p := range m.Plans {
		for feature, f := range p.Features {
			fn := feature.WithPlan(plan)
			ff := tier.Feature{
				FeaturePlan: fn,

				Currency: values.Coalesce(p.Currency, "usd"),
				Interval: values.Coalesce(p.Interval, "@monthly"),

				PlanTitle: values.Coalesce(p.Title, plan.String()),
				Title:     values.Coalesce(f.Title, fn.String()),

				Base: f.Base,

				Mode:      values.Coalesce(f.Mode, "graduated"),
				Aggregate: values.Coalesce(f.Aggregate, "sum"),
			}

			if len(f.Tiers) > 0 {
				ff.Tiers = make([]tier.Tier, len(f.Tiers))
				for i, t := range f.Tiers {
					ff.Tiers[i] = tier.Tier{
						Upto:  t.Upto,
						Price: t.Price,
						Base:  t.Base,
					}
				}
			}

			fs = append(fs, ff)
		}
	}
	return fs, nil
}

func ToPricingJSON(fs []tier.Feature) ([]byte, error) {
	m := jsonModel{
		Plans: make(map[refs.Plan]jsonPlan),
	}
	for _, f := range fs {
		p := m.Plans[f.Plan()]
		p.Title = f.PlanTitle
		p.Currency = f.Currency
		p.Interval = f.Interval

		values.MaybeZero(&p.Currency, "usd")
		values.MaybeZero(&p.Interval, "@monthly")

		if p.Features == nil {
			p.Features = make(map[refs.Name]jsonFeature)
		}

		// TODO(bmizerany): find generic way to clone slices of type
		// types with the same underlying type
		tiers := make([]jsonTier, len(f.Tiers))
		for i, t := range f.Tiers {
			tiers[i] = jsonTier{
				Upto:  t.Upto,
				Price: t.Price,
				Base:  t.Base,
			}
		}

		p.Features[f.FeaturePlan.Name()] = jsonFeature{
			Title: values.ZeroIf(f.Title, f.FeaturePlan.String()),
			Base:  f.Base,
			Tiers: tiers,
		}
		m.Plans[f.Plan()] = p
	}
	return json.MarshalIndent(m, "", "  ")
}
