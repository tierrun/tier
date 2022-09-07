package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"golang.org/x/exp/maps"
)

func Do[R any](ctx context.Context, c *http.Client, method, urlStr string, body any, opts ...any) (R, error) {
	var zero R

	var isJSON bool
	var p io.Reader
	switch v := body.(type) {
	case io.Reader:
		p = v
	case string:
		p = strings.NewReader(v)
	default:
		data, err := json.Marshal(body)
		if err != nil {
			return zero, err
		}
		p = bytes.NewReader(data)
		isJSON = true
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, p)
	if err != nil {
		return zero, err
	}

	if isJSON {
		// Set this header now so that below the client's desired
		// content-type (if any) can clobber the default.
		req.Header.Set("Content-Type", "application/json")
	}

	for _, opt := range opts {
		switch v := opt.(type) {
		case http.Header:
			maps.Copy(req.Header, v)
		case *url.Userinfo:
			pass, _ := v.Password()
			req.SetBasicAuth(v.Username(), pass)
		}
	}

	res, err := c.Do(req)
	if err != nil {
		return zero, err
	}

	if fetchDebug {
		res.Body = struct {
			io.Reader
			io.Closer
		}{
			Reader: io.TeeReader(res.Body, os.Stderr),
			Closer: res.Body,
		}
	}

	return interpretDesiredResponse[R](res)
}

var fetchDebug, _ = strconv.ParseBool(os.Getenv("FETCH_DEBUG"))

func OK[R any, E error](ctx context.Context, c *http.Client, method, urlStr string, body any, opts ...any) (R, error) {
	var zero R
	res, err := Do[*http.Response](ctx, c, method, urlStr, body, opts...)
	if err != nil {
		return zero, err
	}

	// TODO(bmizerany): consider taking an option specifying the range of
	// status codes that result in an error.
	if res.StatusCode == 200 {
		return interpretDesiredResponse[R](res)
	}

	var e E
	if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
		return zero, err
	}
	return zero, e
}

type NotFoundError struct {
	err error
}

func (e *NotFoundError) Unwrap() error { return e.err }
func (e *NotFoundError) Error() string { return e.err.Error() }

func interpretDesiredResponse[R any](res *http.Response) (R, error) {
	t := func(v any) R {
		return v.(R)
	}

	var zero R

	switch any(zero).(type) {
	case *http.Response:
		// caller is responsible for closing
		return t(res), nil
	case *bytes.Buffer:
		defer res.Body.Close()
		b := new(bytes.Buffer)
		_, err := b.ReadFrom(res.Body)
		return t(b), err
	case struct{}:
		res.Body.Close()
		return zero, nil
	case string:
		defer res.Body.Close()
		var b strings.Builder
		_, err := io.Copy(&b, res.Body)
		return t(b.String()), err
	case []byte:
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		return t(data), err // mimic same behavior as io.ReadAll
	default:
		// TODO(bmizerany): check content-type to see if it is safe to
		// unmarshal from json? not everyone sets the header correctly.
		var j R
		defer res.Body.Close()
		if err := json.NewDecoder(res.Body).Decode(&j); err != nil {
			return zero, err
		}
		return j, nil
	}
}
