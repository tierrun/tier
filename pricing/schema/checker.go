package schema

import (
	"fmt"
	"regexp"
	"strings"

	"tailscale.com/util/multierr"
)

type Checker struct {
	errs []error
}

var (
	planRegexp = regexp.MustCompile(`^plan:[a-z0-9:]+@[a-z0-9]+$`)
)

func (c *Checker) Plan(id string) {
	if !planRegexp.MatchString(id) {
		c.errs = append(c.errs, fmt.Errorf("plan ID (%q) must match %q", id, planRegexp.String()))
	}
}

func (c *Checker) Feature(id string) {
	if !strings.HasPrefix(id, "feature:") {
		c.errs = append(c.errs, fmt.Errorf("feature ID (%q) must start with 'feature:'", id))
	}
}

func (c *Checker) Err() error {
	if len(c.errs) == 0 {
		return nil // maintain untyped nil
	}
	return multierr.New(c.errs...)
}
