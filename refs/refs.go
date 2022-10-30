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
	_ encoding.TextMarshaler   = (*Name)(nil)
	_ encoding.TextMarshaler   = (*FeaturePlan)(nil)
	_ encoding.TextUnmarshaler = (*Name)(nil)
	_ encoding.TextUnmarshaler = (*FeaturePlan)(nil)
)

type ParseError struct {
	ID      string
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s: %q", e.Message, e.ID)
}

type Name struct {
	s string
}

func MustParseName(s string) Name {
	n, err := ParseName(s)
	if err != nil {
		panic(err)
	}
	return n
}

func (n Name) String() string   { return "feature:" + n.s }
func (n Name) GoString() string { return fmt.Sprintf("<%s>", n) }
func (n Name) Less(o Name) bool { return n.s < o.s }
func (n Name) WithVersion(v Version) FeaturePlan {
	return FeaturePlan{name: n, version: v}
}

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
	if isIllegalID(name) {
		return Name{}, invalid("feature name must match [a-zA-Z0-9:]+", s)
	}
	return Name{s: name}, nil
}

type Version struct {
	s string
}

func ParseVersion(s string) (Version, error) {
	if isIllegalID(s) {
		return Version{}, invalid("feature version must match [a-zA-Z0-9:]+", s)
	}
	return Version{s: s}, nil
}

func MustParseVersion(s string) Version {
	v, err := ParseVersion(s)
	if err != nil {
		panic(err)
	}
	return v
}

func MustParseVersions(s ...string) []Version {
	vs := make([]Version, 0, len(s))
	for _, s := range s {
		v, err := ParseVersion(s)
		if err != nil {
			panic(err)
		}
		vs = append(vs, v)
	}
	return vs
}

func (v Version) String() string      { return v.s }
func (v Version) GoString() string    { return fmt.Sprintf("<version %s>", v) }
func (v Version) Less(o Version) bool { return v.s < o.s }

func (v *Version) UnmarshalJSON(b []byte) error {
	return unmarshal(v, ParseVersion, b)
}

func (v *Version) UnmarshalText(b []byte) error {
	pv, err := ParseVersion(string(b))
	if err != nil {
		return err
	}
	*v = pv
	return nil
}

func (v Version) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

func (v Version) MarshalText() ([]byte, error) {
	return []byte(v.String()), nil
}

type FeaturePlan struct {
	name    Name
	version Version
}

func ParseFeaturePlans(s ...string) ([]FeaturePlan, error) {
	fps := make([]FeaturePlan, 0, len(s))
	for _, s := range s {
		fp, err := ParseFeaturePlan(s)
		if err != nil {
			return nil, err
		}
		fps = append(fps, fp)
	}
	return fps, nil
}

func MustParseFeaturePlans(s ...string) []FeaturePlan {
	fps, err := ParseFeaturePlans(s...)
	if err != nil {
		panic(err)
	}
	return fps
}

func ParseFeaturePlan(s string) (FeaturePlan, error) {
	name, version, hasVersion := strings.Cut(s, "@")
	if !hasVersion {
		return FeaturePlan{}, invalid("feature must have version", s)
	}
	n, err := ParseName(name)
	if err != nil {
		return FeaturePlan{}, err
	}
	v, err := ParseVersion(version)
	if err != nil {
		return FeaturePlan{}, err
	}
	return FeaturePlan{name: n, version: v}, nil
}

func MustParseFeaturePlan(s string) FeaturePlan {
	fp, err := ParseFeaturePlan(s)
	if err != nil {
		panic(err)
	}
	return fp
}

func ByName(a, b FeaturePlan) bool {
	return a.name.s < b.name.s
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

func (fp FeaturePlan) String() string   { return fmt.Sprintf("%s@%s", fp.name, fp.Version()) }
func (fp FeaturePlan) GoString() string { return fmt.Sprintf("<%s>", fp) }

func (fp FeaturePlan) Name() Name             { return fp.name }
func (fp FeaturePlan) Version() Version       { return fp.version }
func (fp FeaturePlan) InGroup(v Version) bool { return fp.version == v }
func (a FeaturePlan) Less(b FeaturePlan) bool { return a.String() < b.String() }

func (fp FeaturePlan) IsVersionOf(p Name) bool {
	return fp.name == p
}

func SortGroupedByVersion(fs []FeaturePlan) {
	slices.SortFunc(fs, func(a, b FeaturePlan) bool {
		if a.version.s < b.version.s {
			return true
		}
		return a.Less(b)
	})
}

func invalid(msg string, id string) error {
	return &ParseError{Message: msg, ID: id}
}

func isIllegalID(s string) bool {
	if len(s) == 0 {
		return true
	}
	return strings.IndexFunc(s, isIllegalRune) != -1
}

func isIllegalRune(r rune) bool {
	return !(r >= 'a' && r <= 'z' ||
		r >= 'A' && r <= 'Z' ||
		r >= '0' && r <= '9' ||
		r == ':')
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
