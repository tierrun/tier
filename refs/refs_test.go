package refs

import (
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"kr.dev/diff"
)

func TestParsePlan(t *testing.T) {
	cases := []struct {
		in       string
		errMatch string
	}{
		{in: "", errMatch: "plan name must start with 'plan:'"},
		{in: "p", errMatch: "plan name must start with 'plan:'"},
		{in: "plan:", errMatch: "plan must have version"},
		{in: "plan:foo", errMatch: "plan must have version"},
		{in: "plan:foo:bar", errMatch: "plan must have version"},
		{in: "plan:foo@", errMatch: "plan must have version"},
		{in: "plan:foo@!", errMatch: `plan version must match \[a-zA-Z0-9\]\+`},
		{in: "plan:foo@-", errMatch: `plan version must match \[a-zA-Z0-9\]\+`},
		{in: "plan:foo@0", errMatch: "^$"},
		{in: "plan:fo!@0", errMatch: `plan name must match \[a-zA-Z0-9:\]\+`},
	}

	for _, tt := range cases {
		re := regexp.MustCompile(tt.errMatch)
		_, err := ParsePlan(tt.in)
		if err == nil {
			if tt.errMatch != "^$" {
				t.Errorf("ParsePlan(%q) = nil, want error", tt.in)
			}
		} else if !re.MatchString(err.Error()) {
			t.Errorf("ParsePlan(%q) = %q, want error matching %q", tt.in, err.Error(), tt.errMatch)
		}
	}

	testValid(t, ParsePlan, []string{
		"plan:foo@0",
		"plan:foo@2",
		"plan:bar@1",
		"plan:bar@x",
	})
}

func TestParseName(t *testing.T) {
	cases := []struct {
		in       string
		errMatch string
	}{
		{in: "", errMatch: "feature name must start with 'feature:'"},
		{in: "f", errMatch: "feature name must start with 'feature:'"},
		{in: "feature:", errMatch: `feature name must match \[a-zA-Z0-9:\]\+`},
		{in: "feature:foo", errMatch: "^$"},
		{in: "feature:foo:bar", errMatch: "^$"},
		{in: "feature:foo@", errMatch: `feature name must match \[a-zA-Z0-9:\]\+`},
		{in: "feature:foo@_", errMatch: `feature name must match \[a-zA-Z0-9:\]\+`},
		{in: "feature:foo@!", errMatch: `feature name must match \[a-zA-Z0-9:\]\+`},
		{in: "feature:foo@-", errMatch: `feature name must match \[a-zA-Z0-9:\]\+`},
		{in: "feature:fo!@0", errMatch: `feature name must match \[a-zA-Z0-9:\]\+`},
	}

	for i, tt := range cases {
		re := regexp.MustCompile(tt.errMatch)
		_, err := ParseName(tt.in)
		if err == nil {
			if tt.errMatch != "^$" {
				t.Errorf("[%d:%q]: expected error", i, tt.in)
			}
		} else if !re.MatchString(err.Error()) {
			t.Errorf("[%d:%q]: err = %q, want error matching %q", i, tt.in, err.Error(), tt.errMatch)
		}
	}

	testValid(t, ParsePlan, []string{
		"plan:foo@0",
		"plan:foo@1",
		"plan:bar@0",
		"plan:bar@1",
	})
}

func TestParseFeaturePlan(t *testing.T) {
	cases := []struct {
		in       string
		errMatch string
	}{
		{in: "", errMatch: "feature plan must start with 'feature:'"},
		{in: "f", errMatch: "feature plan must start with 'feature:'"},
		{in: "feature", errMatch: "feature plan must start with 'feature:'"},
		{in: "plan:test@0", errMatch: "feature plan must start with 'feature:'"},
		{in: "feature:", errMatch: `feature plan name must match \[a-zA-Z0-9:\]\+`},
		{in: "feature:foo@", errMatch: `feature plan version must match \[a-zA-Z0-9\]\+ or be a valid plan`},
		{in: "feature:foo@_", errMatch: `feature plan version must match \[a-zA-Z0-9\]\+ or be a valid plan`},
		{in: "feature:foo@!", errMatch: `feature plan version must match \[a-zA-Z0-9\]\+ or be a valid plan`},
		{in: "feature:foo@-", errMatch: `feature plan version must match \[a-zA-Z0-9\]\+ or be a valid plan`},
		{in: "feature:foo@plan:", errMatch: `feature plan version must match \[a-zA-Z0-9\]\+ or be a valid plan`},
		{in: "feature:foo@plan:0", errMatch: `feature plan version must match \[a-zA-Z0-9\]\+ or be a valid plan`},
		{in: "feature:foo@plan:@", errMatch: `feature plan version must match \[a-zA-Z0-9\]\+ or be a valid plan`},
		{in: "feature:fo!@0", errMatch: `feature plan name must match \[a-zA-Z0-9:\]\+`},

		{in: "feature:foo:bar@foo"},
		{in: "feature:foo@abc1223"},
		{in: "feature:foo@plan:foo@99"},
	}

	for _, tt := range cases {
		re := regexp.MustCompile(tt.errMatch)
		_, err := ParseFeaturePlan(tt.in)
		if err != nil {
			if tt.errMatch == "" {
				t.Errorf("[%q]: unexpected error: %v", tt.in, err)
			} else if !re.MatchString(err.Error()) {
				t.Errorf("[%q]: err = %q, want error matching %q", tt.in, err.Error(), tt.errMatch)
			}
		}
	}

	testValid(t, ParseFeaturePlan, []string{
		"feature:foo@0",
		"feature:foo@1",
		"feature:bar@0",
		"feature:bar@1",
	})

	t.Run("HasPlan", func(t *testing.T) {
		cases := []struct {
			in      string
			hasPlan bool
		}{
			{in: "feature:foo@0", hasPlan: false},
			{in: "feature:foo@1", hasPlan: false},
			{in: "feature:foo@1", hasPlan: false},
			{in: "feature:bar@0", hasPlan: false},
			{in: "feature:bar@plan:free@0", hasPlan: true},
		}

		for _, tt := range cases {
			fp, err := ParseFeaturePlan(tt.in)
			if err != nil {
				t.Errorf("ParseFeaturePlan(%q) = %q, want nil", tt.in, err.Error())
				continue
			}
			if got := fp.Plan().IsZero(); got == tt.hasPlan {
				t.Errorf("%q: Plan().IsZero() = %v, want %v", tt.in, got, tt.hasPlan)
			}
		}
	})
}

func TestRoundTrips(t *testing.T) {
	testRoundTrip(t, ParseFeaturePlan, "feature:foo@0")
	testRoundTrip(t, ParseFeaturePlan, "feature:foo@plan:free@0")
	testRoundTrip(t, ParseName, "feature:foo")
	testRoundTrip(t, ParsePlan, "plan:foo@0")
}

func testRoundTrip[T fmt.Stringer](t *testing.T, parse func(string) (T, error), s string) {
	t.Helper()
	n, err := parse(s)
	if err != nil {
		t.Errorf("%q: %v", s, err)
		return
	}
	if got := n.String(); got != s {
		t.Errorf("%q: got %q", s, got)
	}
}

func testValid[T comparable](t *testing.T, f func(string) (T, error), cases []string) {
	t.Helper()
	t.Run("equality", func(t *testing.T) {
		t.Helper()
		for _, s := range cases {
			a, err := f(s)
			if err != nil {
				t.Errorf("f(%v) = %q, want nil", s, err.Error())
			}
			b, _ := f(s) // should produce same error as a
			if a != b {
				t.Errorf("expected %v == %v", a, b)
			}
		}
	})
}

func TestJSON(t *testing.T) {
	testJSON(t, ParseFeaturePlan, "feature:foo@0")
	testJSON(t, ParseFeaturePlan, "feature:foo@plan:free@0")
	testJSON(t, ParseName, "feature:foo")
	testJSON(t, ParsePlan, "plan:foo@0")
}

func TestAsMapKeyWithJSON(t *testing.T) {
	var got map[FeaturePlan]int
	if err := json.Unmarshal([]byte(`{"feature:foo@0": 1}`), &got); err != nil {
		t.Errorf("unmarshal: %v", err)
	}
	want := map[FeaturePlan]int{
		MustParseFeaturePlan("feature:foo@0"): 1,
	}
	diff.Test(t, t.Errorf, got, want)
}

type comparableStringer interface {
	comparable
	fmt.Stringer
}

func testJSON[T comparableStringer](t *testing.T, f func(string) (T, error), s string) {
	t.Helper()
	n, err := f(s)
	if err != nil {
		t.Errorf("%q: %v", s, err)
		return
	}
	b, err := json.Marshal(n)
	if err != nil {
		t.Errorf("%q: %v", s, err)
		return
	}
	var got T
	if err := json.Unmarshal(b, &got); err != nil {
		t.Errorf("%q: %v", s, err)
		return
	}
	if got != n {
		t.Errorf("%q: got %v, want %v", s, got, n)
	}
}
