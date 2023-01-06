package future

import (
	"errors"
	"testing"
)

func Test(t *testing.T) {
	e := errors.New("test error")
	f := Go(func() (bool, error) {
		return true, e
	})
	gv, ge := f.Get()
	if !gv {
		t.Error("expected true")
	}
	if !errors.Is(ge, e) {
		t.Errorf("ge = %v; want %v", ge, e)
	}
}
