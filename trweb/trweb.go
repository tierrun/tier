package trweb

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type HTTPError struct {
	Source  string `json:"source,omitempty"`
	Status  int    `json:"status"`
	Code    string `json:"code"` // (e.g. "invalid_request")
	Message string `json:"message"`
}

var (
	NotFound         = &HTTPError{Status: 404, Code: "not_found", Message: "Not Found"}
	Unauthorized     = &HTTPError{Status: 401, Code: "unauthorized", Message: "Unauthorized"}
	InternalError    = &HTTPError{Status: 500, Code: "internal_error", Message: "Internal Server Error"}
	MethodNotAllowed = &HTTPError{Status: 405, Code: "method_not_allowed", Message: "Method Not Allowed"}
	InvalidRequest   = &HTTPError{Status: 400, Code: "invalid_request", Message: "Invalid Request"}
)

func Error(status int, code string, message string) error {
	return &HTTPError{"", status, code, message}
}

// WriteError encodes err to w, setting the approriate headers, if the underlying
// type of err is an HTTPError and returns true; otherwise it does nothing and
// returns false.
func WriteError(w http.ResponseWriter, err error) bool {
	var he *HTTPError
	if errors.As(err, &he) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(he.Status)
		e := json.NewEncoder(w)
		e.SetIndent("", "    ")
		_ = e.Encode(he)
		return true
	}
	return false
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("httpError{status:%d code:%q message:%q}",
		e.Status, e.Code, e.Message)
}

// Decode decodes the request body as HuJSON. If an error occurs it might wrap
// the error as an HTTPError.
func Decode(r *http.Request, v any) error {
	return jsonErr(json.NewDecoder(r.Body).Decode(v))
}

// StrictDecode works like Decode but does not allow unknown fields.
func DecodeStrict(r *http.Request, v any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return jsonErr(d.Decode(v))
}

func jsonErr(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, io.EOF) {
		return nil
	}
	switch err.(type) {
	case *json.SyntaxError:
		return &HTTPError{"", 400, "invalid_request", "invalid json syntax"}
	default:
		if msg := err.Error(); strings.HasPrefix(msg, "json: unknown field ") {
			msg = strings.TrimPrefix(msg, "json: ")
			return &HTTPError{Status: 400, Code: "invalid_request", Message: msg}
		}
		return err
	}
}
