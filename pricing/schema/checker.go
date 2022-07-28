package schema

import (
	"fmt"
	"strings"

	"tailscale.com/util/multierr"
)

type Checker struct {
	errs []error
}

func (c *Checker) Plan(id string) {
	if !strings.HasPrefix(id, "plan:") {
		c.errs = append(c.errs, fmt.Errorf("plan ID (%q) must start with 'plan:'", id))
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
