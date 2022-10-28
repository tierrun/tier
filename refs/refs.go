package refs

import (
	"encoding"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/exp/slices"
)

// marshlers
var (
	_ encoding.TextMarshaler   = (*Plan)(nil)
	_ encoding.TextMarshaler   = (*Name)(nil)
	_ encoding.TextMarshaler   = (*FeaturePlan)(nil)
	_ encoding.TextUnmarshaler = (*Plan)(nil)
	_ encoding.TextUnmarshaler = (*Name)(nil)
	_ encoding.TextUnmarshaler = (*FeaturePlan)(nil)
)

type ParseError struct {
	ID      string
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s: %s", e.Message, e.ID)
}

type Plan struct {
	name    string
	version string
}

func MustParsePlan(s string) Plan {
	p, err := ParsePlan(s)
	if err != nil {
		panic(err)
	}
	return p
}

func (p Plan) String() string { return "plan:" + p.name + "@" + p.version }
func (p Plan) IsZero() bool   { return p == Plan{} }

func (p *Plan) UnmarshalJSON(b []byte) error {
	return unmarshal(p, ParsePlan, b)
}

func (p *Plan) UnmarshalText(b []byte) error {
	np, err := ParsePlan(string(b))
	if err != nil {
		return err
	}
	*p = np
	return nil
}

func (p Plan) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.String())
}

func (p Plan) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

func ParsePlan(s string) (Plan, error) {
	prefix, rest, hasPrefix := strings.Cut(s, ":")
	if !hasPrefix || prefix != "plan" {
		return Plan{}, invalid("plan name must start with 'plan:'", s)
	}
	name, version, _ := strings.Cut(rest, "@")
	if version == "" {
		return Plan{}, invalid("plan must have version", s)
	}
	if isIllegalName(name) {
		return Plan{}, invalid("plan name must match [a-zA-Z0-9:]+", s)
	}
	if isIllegalVersion(version) {
		return Plan{}, invalid("plan version must match [a-zA-Z0-9]+", s)
	}
	return Plan{name: name, version: version}, nil
}

type Name struct {
	name string
}

func MustParseName(s string) Name {
	n, err := ParseName(s)
	if err != nil {
		panic(err)
	}
	return n
}

func (n Name) String() string              { return "feature:" + n.name }
func (n Name) WithPlan(p Plan) FeaturePlan { return FeaturePlan{name: n.name, plan: p} }
func (n Name) Less(o Name) bool            { return n.name < o.name }

func (fp *Name) UnmarshalJSON(b []byte) error {
	return unmarshal(fp, ParseName, b)
}

func (fp *Name) UnmarshalText(b []byte) error {
	n, err := ParseName(string(b))
	if err != nil {
		return err
	}
	*fp = n
	return nil
}

func (fp Name) MarshalJSON() ([]byte, error) {
	return json.Marshal(fp.String())
}

func (fp Name) MarshalText() ([]byte, error) {
	return []byte(fp.String()), nil
}

func ParseName(s string) (Name, error) {
	prefix, name, hasPrefix := strings.Cut(s, ":")
	if !hasPrefix || prefix != "feature" {
		return Name{}, invalid("feature name must start with 'feature:'", s)
	}
	if isIllegalName(name) {
		return Name{}, invalid("feature name must match [a-zA-Z0-9:]+", s)
	}
	return Name{name: name}, nil
}

type FeaturePlan struct {
	name    string
	version string // empty if plan is non-empty
	plan    Plan
}

func ParseFeaturePlans(s ...string) ([]FeaturePlan, error) {
	fps := make([]FeaturePlan, len(s))
	for _, s := range s {
		fp, err := ParseFeaturePlan(s)
		if err != nil {
			return nil, err
		}
		fps = append(fps, fp)
	}
	return fps, nil
}

func ParseFeaturePlan(s string) (FeaturePlan, error) {
	prefix, rest, hasPrefix := strings.Cut(s, ":")
	if !hasPrefix || prefix != "feature" {
		return FeaturePlan{}, invalid("feature plan must start with 'feature:'", s)
	}
	name, version, hasVersion := strings.Cut(rest, "@")
	if isIllegalName(name) {
		return FeaturePlan{}, invalid("feature plan name must match [a-zA-Z0-9:]+", s)
	}
	if !hasVersion {
		return FeaturePlan{}, invalid("feature plan must have version", s)
	}

	fp := FeaturePlan{name: name}
	if p, err := ParsePlan(version); err == nil {
		fp.plan = p
		return fp, nil
	}
	if isIllegalVersion(version) {
		return FeaturePlan{}, invalid("feature plan version must match [a-zA-Z0-9]+ or be a valid plan", s)
	}
	fp.version = version
	return fp, nil
}

func MustParseFeaturePlan(s string) FeaturePlan {
	fp, err := ParseFeaturePlan(s)
	if err != nil {
		panic(err)
	}
	return fp
}

func MustParseFeaturePlans(s ...string) []FeaturePlan {
	fps, err := ParseFeaturePlans(s...)
	if err != nil {
		panic(err)
	}
	return fps
}

func ByName(a, b FeaturePlan) bool {
	return a.name < b.name
}

func ByPlan(a, b FeaturePlan) bool {
	return a.plan.String() < b.plan.String()
}

func (fp *FeaturePlan) UnmarshalJSON(b []byte) error {
	return unmarshal(fp, ParseFeaturePlan, b)
}

func (fp *FeaturePlan) UnmarshalText(b []byte) error {
	p, err := ParseFeaturePlan(string(b))
	if err != nil {
		return err
	}
	*fp = p
	return nil
}

func (fp FeaturePlan) MarshalJSON() ([]byte, error) {
	return json.Marshal(fp.String())
}

func (fp FeaturePlan) MarshalText() ([]byte, error) {
	return []byte(fp.String()), nil
}

func (fp FeaturePlan) IsZero() bool {
	return fp == FeaturePlan{}
}

func (fp FeaturePlan) String() string {
	return fmt.Sprintf("feature:%s@%s", fp.name, fp.Version())
}

func (fp FeaturePlan) Name() Name             { return Name{name: fp.name} }
func (fp FeaturePlan) Plan() Plan             { return fp.plan }
func (fp FeaturePlan) InPlan(p Plan) bool     { return fp.plan == p }
func (a FeaturePlan) Less(b FeaturePlan) bool { return a.String() < b.String() }

// Version returns the version of the feature plan as it was parsed. This means
// if the version is a plan, the plan identifier is returned.
func (fp FeaturePlan) Version() string {
	if fp.version != "" {
		return fp.version
	}
	return fp.plan.String()
}

func SortGroupedByVersion(fs []FeaturePlan) {
	slices.SortFunc(fs, func(a, b FeaturePlan) bool {
		if a.Version() < b.Version() {
			return true
		}
		return a.Less(b)
	})
}

func invalid(msg string, id string) error {
	return &ParseError{Message: msg, ID: id}
}

func isIllegalName(s string) bool {
	if len(s) == 0 {
		return true
	}
	return strings.IndexFunc(s, isIllegalNameRune) != -1
}

func isIllegalNameRune(r rune) bool {
	return !(r >= 'a' && r <= 'z' ||
		r >= 'A' && r <= 'Z' ||
		r >= '0' && r <= '9' ||
		r == ':')
}

func isIllegalVersion(s string) bool {
	if len(s) == 0 {
		return true
	}
	return strings.IndexFunc(s, isIllegalVersionRune) != -1
}

func isIllegalVersionRune(r rune) bool {
	return !(r >= 'a' && r <= 'z' ||
		r >= 'A' && r <= 'Z' ||
		r >= '0' && r <= '9')
}

func unmarshal[T any](v *T, f func(s string) (T, error), b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	var err error
	*v, err = f(s)
	return err
}
