package trweb

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"kr.dev/diff"
)

func TestDecode(t *testing.T) {
	t.Run("bad syntax", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/", strings.NewReader("boom"))
		var v json.RawMessage
		got := Decode(r, &v)
		want := &HTTPError{
			Status:  400,
			Code:    "invalid_request",
			Message: "invalid json syntax",
		}
		diff.Test(t, t.Errorf, got, want)
	})

	t.Run("strict unknown field", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"a": {"notAField": 1}}`))
		var v struct {
			A struct{}
		}
		got := DecodeStrict(r, &v)
		want := &HTTPError{
			Status:  400,
			Code:    "invalid_request",
			Message: `unknown field "notAField"`,
		}
		diff.Test(t, t.Errorf, got, want)
	})

	t.Run("unmarshal error", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"a": {"notAField": 1}}`))
		var v struct {
			A int
		}
		got := DecodeStrict(r, &v)
		var he *HTTPError
		if errors.As(got, &he) {
			t.Errorf("unexpected HTTPError: %v", got)
		}
	})
}
