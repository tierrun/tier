// Package frate implements file-based rate limiting.
package frate

import (
	"os"
	"path/filepath"
	"time"
)

// Limiter is a file-based rate limiter. It is not safe for concurrent use.
type Limiter struct {
	Dir string // defaults to "."

	names []string
	errs  []error

	nowf func() time.Time
}

func (l *Limiter) dir(name string) string {
	dir := l.Dir
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, "frate", "buckets", name)
}

func (l *Limiter) now() time.Time {
	if l.nowf != nil {
		return l.nowf()
	}
	return time.Now()
}

// Touch updates the last-accessed time of the named bucket if the time since
// the current access time is >= d. It returns true if the bucket was touched;
// false otherwise.
//
// If an error occurs, it is recorded and returned by Err.
func (l *Limiter) Touch(name string, d time.Duration) bool {
	info, err := os.Stat(l.dir(name))
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(l.dir(""), 0700)
			if err != nil {
				return l.err(err)
			}
			if err = os.WriteFile(l.dir(name), nil, 0600); err != nil {
				return l.err(err)
			}
			l.names = append(l.names, name)
			return true
		}
		return l.err(err)
	}
	now := l.now()
	if now.Sub(info.ModTime()) < d {
		return false
	}
	if err := os.Chtimes(l.dir(name), now, now); err != nil {
		return l.err(err)
	}
	l.names = append(l.names, name)
	return true
}

func (l *Limiter) err(err error) bool {
	l.errs = append(l.errs, err)
	return false // for convenience
}

// Touched returns the names of the buckets that have been touched successfully.
// The returned slice is not safe to modify, and is only valid until the next
// call Touch.
func (l *Limiter) Touched() []string {
	return l.names
}

// Err returns the last error encountered, if any.
func (l *Limiter) Err() error {
	if len(l.errs) == 0 {
		return nil
	}
	return l.errs[len(l.errs)-1]
}

// Errs returns all errors encountered, if any.
func (l *Limiter) Errs() []error {
	return l.errs
}
