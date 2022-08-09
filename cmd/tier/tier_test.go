package main

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/tierrun/tierx/pricing"
)

func setStdin(t *testing.T, r io.Reader) {
	old := stdin
	stdin = r
	t.Cleanup(func() {
		stdin = old
	})
}

func setStdout(t *testing.T, w io.Writer) {
	old := stdout
	stdout = w
	t.Cleanup(func() {
		stdout = old
	})
}

func setStderr(t *testing.T, w io.Writer) {
	old := stderr
	stderr = w
	t.Cleanup(func() {
		stderr = old
	})
}

func TestBadJSON(t *testing.T) {
	setStdout(t, panicOnWrite)
	setStderr(t, panicOnWrite)
	setStdin(t, strings.NewReader(`{]`))

	err := tier("push", nil)
	if !errors.As(err, &pricing.DecodeError{}) {
		t.Fatalf("err = %v, want DecodeError", err)
	}
}

type pow struct{}

func (w *pow) Write(p []byte) (int, error) { panic("unexpected write") }

var panicOnWrite = &pow{}
