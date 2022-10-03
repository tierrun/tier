package stripe

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("stripe: not found")

type ID string

func (id ID) ProviderID() string { return string(id) }

type Identifiable interface {
	ProviderID() string
}

type Iterator[I Identifiable] struct {
	c *Client

	ctx          context.Context
	method, path string
	f            Form

	data []I

	val     I
	offset  string
	err     error // owned by Next and Err only
	hasMore bool
}

func List[I Identifiable](ctx context.Context, c *Client, method, path string, f Form) *Iterator[I] {
	return &Iterator[I]{
		c:       c,
		hasMore: true,

		ctx:    ctx,
		method: method,
		path:   path,
		f:      f,
	}
}

func (i *Iterator[I]) Err() error { return i.err }
func (i *Iterator[I]) Value() I   { return i.val }

func (i *Iterator[I]) Next() bool {
	for {
		if len(i.data) > 0 {
			i.val, i.data = i.data[0], i.data[1:]
			return true
		}
		if i.hasMore {
			if err := i.refill(); err != nil {
				i.err = err
				return false
			}
			if len(i.data) == 0 {
				// avoid inf loop if refill "succeeds" but returns no data
				return false
			}
			continue
		}
		return false
	}
}

func (i *Iterator[I]) Find(f func(v I) bool) (I, error) {
	for i.Next() {
		if f(i.Value()) {
			return i.Value(), nil
		}
	}
	var zero I
	if err := i.Err(); err != nil {
		return zero, err
	}
	return zero, ErrNotFound
}

func (i *Iterator[I]) refill() error {
	var t struct {
		HasMore bool `json:"has_more"`
		Data    []I
	}

	f := i.f.Clone()
	if i.offset != "" {
		f.Set("starting_after", i.offset)
	}
	f.Set("limit", 100)

	if err := i.c.Do(i.ctx, i.method, i.path, f, &t); err != nil {
		return err
	}

	i.hasMore = t.HasMore
	i.data = t.Data
	if l := len(i.data); l > 0 {
		i.offset = i.data[l-1].ProviderID()
	}
	return nil
}

// Slurp returns each I over all pages ln a list, or an error if any.
func Slurp[I Identifiable](ctx context.Context, c *Client, method, path string, f Form) ([]I, error) {
	// TODO(bmizerany): respect some rate-limiter (maybe in c?)
	f.Set("limit", 100)

	// TODO(bmizerany): grow slice as needed? currently it grows by one per
	// thing. that could be a lot of allocs.

	var tt []I
	l := List[I](ctx, c, method, path, f)
	for l.Next() {
		tt = append(tt, l.Value())
	}
	if err := l.Err(); err != nil {
		return nil, err
	}
	return tt, nil
}
