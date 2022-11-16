package fetchtest

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func BaseURL(c *http.Client) string {
	if tr, ok := c.Transport.(*httptestTransport); ok {
		return tr.baseURL
	}
	return ""
}

func NewServer(t *testing.T, h http.HandlerFunc) *http.Client {
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)

	c := s.Client()
	c.Transport = &httptestTransport{
		base:    c.Transport,
		baseURL: s.URL,
	}
	return c
}

func NewTLSServer(t *testing.T, h http.HandlerFunc) *http.Client {
	s := httptest.NewTLSServer(h)
	t.Cleanup(s.Close)

	c := s.Client()
	c.Transport = &httptestTransport{
		base:    c.Transport,
		baseURL: s.URL,
	}
	return c
}

type httptestTransport struct {
	base    http.RoundTripper
	baseURL string
}

func (tr *httptestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context()) // per RoundTrip contract
	u, err := url.Parse(tr.baseURL)
	if err != nil {
		return nil, err
	}
	mergeURLs(r.URL, u)
	return tr.base.RoundTrip(r)
}

func mergeURLs(dst, src *url.URL) {
	// TODO(bmizerany): maybe use reflect to copy all public fields?
	maybeSet(&dst.Scheme, src.Scheme)
	maybeSet(&dst.Opaque, src.Opaque)
	maybeSet(&dst.User, src.User)
	maybeSet(&dst.Host, src.Host)
	maybeSet(&dst.Path, src.Path)
	maybeSet(&dst.RawPath, src.RawPath)
	maybeSet(&dst.ForceQuery, src.ForceQuery)
	maybeSet(&dst.RawQuery, src.RawQuery)
	maybeSet(&dst.Fragment, src.Fragment)
	maybeSet(&dst.RawFragment, src.RawFragment)
}

// Coalesce returns the first non-zero value in a, if any; otherwise it returns
// the zero value of T.
func coalesce[T comparable](a ...T) T {
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
func maybeSet[T comparable](v *T, a T) {
	*v = coalesce(*v, a)
}
