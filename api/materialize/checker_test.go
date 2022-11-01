package materialize

import (
	"testing"

	"tier.run/api/apitypes"
	"tier.run/refs"
)

func TestValidatePlanID(t *testing.T) {
	cases := []struct {
		planID string
		valid  bool
	}{
		{"plan:a@0", true},
		{"plan:a@x", true},
		{"plan:a@11", true},
		{"plan:a@1a2b3c", true},
		{"plan:a@a2b3c1", true},
	}

	for _, tc := range cases {
		planID := refs.MustParsePlan(tc.planID)
		t.Run(tc.planID, func(t *testing.T) {
			m := apitypes.Model{
				Plans: map[refs.Plan]apitypes.Plan{
					planID: {
						Features: map[refs.Name]apitypes.Feature{
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
