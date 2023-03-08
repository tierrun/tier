// Package wants provides utilities learning what HTTP clients want.
package they

import (
	"net/http"
	"regexp"
)

// A reports whether the request is for the given method and pattern. The
// provided pattern is automatically anchored at both ends.
//
// It panics if the pattern is not a valid regular expression.
func Want(r *http.Request, method, pattern string) bool {
	// TODO(bmizerany): memoize the regexp
	exp := regexp.MustCompile("^" + pattern + "$")
	return r.Method == method && exp.MatchString(r.URL.Path)
}
