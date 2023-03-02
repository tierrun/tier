// Package materialize provides utilities for materializing views from features
// pushed and pulled to billing engine providers.
package materialize

import (
	"bytes"
	"encoding/json"

	"github.com/tailscale/hujson"
	"tier.run/api/apitypes"
	"tier.run/control"
	"tier.run/refs"
	"tier.run/values"
)

func FromPricingHuJSON(data []byte) (fs []control.Feature, err error) {
	var debug []string
	dbg := func(k string) {
		debug = append(debug, k)
	}

	data, err = hujson.Standardize(data)
	if err != nil {
		return nil, err
	}

	var m apitypes.Model
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

			divide := values.Coalesce(f.Divide, &apitypes.Divide{})
			ff := control.Feature{
				FeaturePlan: fn,

				Currency: values.Coalesce(p.Currency, "usd"),
				Interval: values.Coalesce(p.Interval, "@monthly"),

				PlanTitle: values.Coalesce(p.Title, plan.String()),
				Title:     values.Coalesce(f.Title, fn.String()),

				Base: f.Base,

				Mode:      values.Coalesce(f.Mode, "graduated"),
				Aggregate: values.Coalesce(f.Aggregate, "sum"),

				TransformDenominator: divide.By,
				TransformRoundUp:     divide.Rounding == "up",
			}

			if len(f.Tiers) > 0 {
				ff.Tiers = make([]control.Tier, len(f.Tiers))
				for i, t := range f.Tiers {
					ff.Tiers[i] = control.Tier{
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

func ToPricingJSON(fs []control.Feature) ([]byte, error) {
	m := apitypes.Model{
		Plans: make(map[refs.Plan]apitypes.Plan),
	}
	for _, f := range fs {
		p := m.Plans[f.Plan()]
		p.Title = f.PlanTitle
		p.Currency = f.Currency
		p.Interval = f.Interval

		values.MaybeZero(&p.Currency, "usd")
		values.MaybeZero(&p.Interval, "@monthly")

		if p.Features == nil {
			p.Features = make(map[refs.Name]apitypes.Feature)
		}

		// TODO(bmizerany): find generic way to clone slices of type
		// types with the same underlying type
		tiers := make([]apitypes.Tier, len(f.Tiers))
		for i, t := range f.Tiers {
			tiers[i] = apitypes.Tier{
				Upto:  t.Upto,
				Price: t.Price,
				Base:  t.Base,
			}
		}

		af := apitypes.Feature{
			Title:     values.ZeroIf(f.Title, f.FeaturePlan.String()),
			Base:      f.Base,
			Mode:      values.ZeroIf(f.Mode, "graduated"),
			Aggregate: values.ZeroIf(f.Aggregate, "sum"),
			Tiers:     tiers,
		}
		if f.TransformDenominator != 0 {
			var round string
			if f.TransformRoundUp {
				round = "up"
			}
			af.Divide = &apitypes.Divide{
				By:       f.TransformDenominator,
				Rounding: round,
			}
		}
		p.Features[f.FeaturePlan.Name()] = af
		m.Plans[f.Plan()] = p
	}
	return json.MarshalIndent(m, "", "  ")
}
