package schema

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/exp/slices"
	"tailscale.com/util/multierr"
)

func Validate(p *Plan) error {
	var e errors
	e.report(validatePlanID(p.ID))

	if len(p.Features) == 0 {
		e.reportf("plans[%q]: plans must have at least one feature", p.ID)
	}

	for _, f := range p.Features {
		e.report(validateFeatureID(p.ID, f.ID))

		if f.Base > 0 && len(f.Tiers) > 0 {
			e.reportf("plans[%q].features[%q]: base must be zero with tiers", p.ID, f.ID)
		}
		if f.Base < 0 {
			e.reportf("plans[%q].features[%q]: base must be positive", p.ID, f.ID)
		}

		for i, t := range f.Tiers {
			if t.Upto < 0 {
				e.reportf("plans[%q].features[%q].tiers[%d]: upto must be positive", p.ID, f.ID, i)
			}
			if t.Price < 0 {
				e.reportf("plans[%q].features[%q].tiers[%d]: price must be positive", p.ID, f.ID, i)
			}
			if t.Base < 0 {
				e.reportf("plans[%q].features[%q].tiers[%d]: base must be positive", p.ID, f.ID, i)
			}
		}
	}
	return multierr.New(e...)
}

func validatePlanID(id string) error {
	var e errors
	prefix, id, _ := strings.Cut(id, ":")
	if prefix != "plan" {
		e.reportf("plans[%q]: plan names must start with 'plan:'", id)
	}
	name, version, _ := strings.Cut(id, "@")
	if !isAlphaNumeric(name, ":") {
		e.reportf("plans[%q]: plan names must contain only letters, digits, or ':'s", id)
	}
	if version == "" {
		e.reportf("plans[%q]: plan names must have a version", id)
	}
	if !isAlphaNumeric(version, "") {
		e.reportf("plans[%q]: plan versions must contain only letters or digits", id)
	}
	return multierr.New(e...)
}

func validateFeatureID(planID, id string) error {
	var e errors
	prefix, name, _ := strings.Cut(id, ":")
	if prefix != "feature" {
		e.reportf("plans[%q].features[%q]: feature names must start with 'feature:'", planID, id)
	}
	if !isAlphaNumeric(name, ":") {
		e.reportf("plans[%q].features[%q]: feature names must not be empty and contain only letters, digits, or ':'s", planID, id)
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

func isAlphaNumeric(s string, extra string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if slices.Contains([]rune(extra), r) {
			continue
		}
		if !(unicode.IsDigit(r) || unicode.IsLetter(r)) {
			return false
		}
	}
	return true
}
