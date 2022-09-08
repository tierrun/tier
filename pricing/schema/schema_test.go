package schema

import "testing"

func TestValidatePlanID(t *testing.T) {
	cases := []struct {
		planID string
		valid  bool
	}{
		// invalid
		{"", false},
		{"x", false},
		{"plan:", false},
		{"plan:a", false},
		{"plan:a@_", false},
		{"plan:a@1_1", false},
		{"plan:a@1:1", false},
		{"plan:a:b@11", true},
		{"plan:_a:b@11", false},
		{"plan:a_b:c@11", false},
		{"flan:a@0", false},
		{":a@0", false},

		// valid
		{"plan:a@0", true},
		{"plan:a@x", true},
		{"plan:a@11", true},
		{"plan:a@1a2b3c", true},
		{"plan:a@a2b3c1", true},
	}

	for _, tc := range cases {
		t.Run(tc.planID, func(t *testing.T) {
			p := &Plan{
				ID:       tc.planID,
				Features: Features{{ID: "feature:x"}},
			}
			err := Validate(p)
			if tc.valid && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tc.valid && err == nil {
				t.Errorf("expected error")
			}
		})
	}

}
