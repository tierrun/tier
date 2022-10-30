package refs

import (
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"kr.dev/diff"
)

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
}

func TestParseFeaturePlan(t *testing.T) {
	const (
		rePrefixError          = `feature name must start with 'feature:'`
		reNameError            = `feature name must match \[a-zA-Z0-9:\]\+`
		reVersionRequiredError = `feature must have version`
		reVersionError         = `feature version must match \[a-zA-Z0-9:\]\+`
	)

	cases := []struct {
		in       string
		errMatch string
	}{
		{in: "", errMatch: reVersionRequiredError},
		{in: "f", errMatch: reVersionRequiredError},
		{in: "feature", errMatch: reVersionRequiredError},
		{in: "plan:test@0", errMatch: rePrefixError},
		{in: "feature:", errMatch: reVersionRequiredError},
		{in: "feature:foo@", errMatch: reVersionError},
		{in: "feature:foo@", errMatch: reVersionError},
		{in: "feature:foo@_", errMatch: reVersionError},
		{in: "feature:foo@!", errMatch: reVersionError},
		{in: "feature:foo@-", errMatch: reVersionError},
		{in: "feature:foo@plan:", errMatch: reVersionError},
		{in: "feature:fo!@0", errMatch: reNameError},

		{in: "feature:foo:bar@foo"},
		{in: "feature:foo@abc1223"},
		{in: "feature:foo@plan:foo"},
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
}

func TestRoundTrips(t *testing.T) {
	testRoundTrip(t, ParseFeaturePlan, "feature:foo@0")
	testRoundTrip(t, ParseFeaturePlan, "feature:foo@plan:free")
	testRoundTrip(t, ParseName, "feature:foo")
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
	testJSON(t, ParseFeaturePlan, "feature:foo@plan:free")
	testJSON(t, ParseName, "feature:foo")
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
