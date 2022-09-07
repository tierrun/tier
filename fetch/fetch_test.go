package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"kr.dev/diff"
	"tier.run/fetch/fetchtest"
)

func TestFetch(t *testing.T) {
	// TODO(bmizerany): test body is closed when needed?
	ctx := context.Background()

	t.Run("string", func(t *testing.T) {
		want := `Hello.`
		c := fetchtest.NewTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, want) // nolint: errcheck
		})
		got, err := Do[string](ctx, c, "GET", "/", nil)
		if err != nil {
			t.Fatal(err)
		}
		diff.Test(t, t.Errorf, got, want)
	})
	t.Run("[]byte", func(t *testing.T) {
		want := []byte(`Hello.`)
		c := fetchtest.NewTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write(want) // nolint: errcheck
		})
		got, err := Do[[]byte](ctx, c, "GET", "/", nil)
		if err != nil {
			t.Fatal(err)
		}
		diff.Test(t, t.Errorf, got, want)
	})
	t.Run("some_struct", func(t *testing.T) {
		out := []byte(`{"name": "world"}`)
		c := fetchtest.NewTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write(out) // nolint: errcheck
		})
		type hello struct {
			Name string
		}
		got, err := Do[hello](ctx, c, "GET", "/", nil)
		if err != nil {
			t.Fatal(err)
		}
		want := hello{"world"}
		diff.Test(t, t.Errorf, got, want)
	})
	t.Run("any", func(t *testing.T) {
		out := []byte(`{"name": "world"}`)
		c := fetchtest.NewTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write(out) // nolint: errcheck
		})
		type hello struct {
			Name string
		}
		got, err := Do[hello](ctx, c, "GET", "/", nil)
		if err != nil {
			t.Fatal(err)
		}
		want := hello{"world"}
		diff.Test(t, t.Errorf, got, want)
	})
	t.Run("int", func(t *testing.T) {
		c := fetchtest.NewTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, "1") // nolint: errcheck
		})
		got, err := Do[int](ctx, c, "GET", "/", nil)
		if err != nil {
			t.Fatal(err)
		}
		want := 1
		diff.Test(t, t.Errorf, got, want)
	})
	t.Run("*bytes.Buffer", func(t *testing.T) {
		c := fetchtest.NewTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, "1") // nolint: errcheck
		})
		got, err := Do[*bytes.Buffer](ctx, c, "GET", "/", nil)
		if err != nil {
			t.Fatal(err)
		}
		want := new(bytes.Buffer)
		want.WriteString("1") // nolint: errcheck
		diff.Test(t, t.Errorf, got, want)
	})
}

func TestFetchOK(t *testing.T) {
	ctx := context.Background()
	t.Run("error_custom", func(t *testing.T) {
		c := fetchtest.NewTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(400)
			io.WriteString(w, `{"code": "problem"}`) // nolint: errcheck
		})
		_, got := OK[int, TE](ctx, c, "GET", "/", nil)
		var want TE
		if !errors.As(got, &want) {
			t.Errorf("err = %T; want %T", got, want)
		}
	})
	t.Run("error_client", func(t *testing.T) {
		c := fetchtest.NewTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(400)
			io.WriteString(w, `notJSON`) // nolint: errcheck
		})

		_, err := OK[int, TE](ctx, c, "GET", "/", nil)
		var got *json.SyntaxError
		if !errors.As(err, &got) {
			t.Errorf("err = %T; want %T", got, &json.SyntaxError{})
		}
	})
}

type TE struct {
	Code string
}

func (e TE) Error() string {
	return "test error!"
}
