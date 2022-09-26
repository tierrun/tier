package values

// Coalesce returns the first non-zero value in a, if any; otherwise it returns
// the zero value of T.
func Coalesce[T comparable](a ...T) T {
	var zero T
	for _, v := range a {
		if v != zero {
			return v
		}
	}
	return zero
}

// MaybeSet is shorthand for:
//
//	v = Coalesce(v, a)
func MaybeSet[T comparable](v *T, a T) {
	*v = Coalesce(*v, a)
}

type Collection[K comparable, V any] map[K][]V

func (c *Collection[K, V]) Add(key K, v V) {
	if *c == nil {
		*c = make(map[K][]V)
	}
	(*c)[key] = append((*c)[key], v)
}

func MapFunc[F, T any](s []F, f func(F) T) []T {
	if s == nil {
		// preserve nil
		return nil
	}
	tt := make([]T, len(s))
	for i, v := range s {
		tt[i] = f(v)
	}
	return tt
}

func Invert[K, V comparable](m map[K]V) map[V]K {
	out := make(map[V]K, len(m))
	for k, v := range m {
		out[v] = k
	}
	return out
}

// ZeroIf returns the zero value of T if this is equal to that; otherwise it
// returns this. It never returns that.
func ZeroIf[T comparable](this, that T) T {
	if this == that {
		var zero T
		return zero
	}
	return this
}

// MaybeZero is shorthand for
//
//	v = ZeroIf(v, someValue)
func MaybeZero[T comparable](v *T, zero T) {
	*v = ZeroIf(*v, zero)
}
