package materialize

import (
	"fmt"

	"tailscale.com/util/multierr"
	"tier.run/api/apitypes"
)

func validate(m apitypes.Model) error {
	var e errors
	for plan, p := range m.Plans {
		if len(p.Features) == 0 {
			e.reportf("plans[%q]: plans must have at least one feature", plan)
		}
		for feature, f := range p.Features {
			if f.Base > 0 && len(f.Tiers) > 0 {
				e.reportf("plans[%q].features[%q]: base must be zero with tiers", plan, feature)
			}
			if f.Base < 0 {
				e.reportf("plans[%q].features[%q]: base must be positive", plan, feature)
			}

			for i, t := range f.Tiers {
				if t.Upto < 1 {
					e.reportf("plans[%q].features[%q].tiers[%d]: upto must be greater than zero", plan, feature, i)
				}
				if t.Price < 0 {
					e.reportf("plans[%q].features[%q].tiers[%d]: price must be positive", plan, feature, i)
				}
				if t.Base < 0 {
					e.reportf("plans[%q].features[%q].tiers[%d]: base must be positive", plan, feature, i)
				}
			}
		}
	}
	return multierr.New(e...)
}

type errors []error

func (e *errors) report(err error) {
	if err != nil {
		*e = append(*e, err)
	}
}

func (e *errors) reportf(format string, args ...any) {
	e.report(fmt.Errorf(format, args...))
}
