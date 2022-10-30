package materialize

import (
	"testing"

	"tier.run/refs"
)

func TestValidatePlanID(t *testing.T) {
	cases := []struct {
		planID string
		valid  bool
	}{
		{"plan:a", true},
		{"plan:ax", true},
		{"plan:a11", true},
		{"plan:a1a2b3c", true},
		{"plan:aa2b3c1", true},
	}

	for _, tc := range cases {
		planID := refs.MustParseVersion(tc.planID)
		t.Run(tc.planID, func(t *testing.T) {
			m := jsonModel{
				Plans: map[refs.Version]jsonPlan{
					planID: {
						Features: map[refs.Name]jsonFeature{
							refs.MustParseName("feature:x"): {},
						},
					},
				},
			}
			err := validate(m)
			if tc.valid && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tc.valid && err == nil {
				t.Errorf("expected error")
			}
		})
	}
}
