package frate

import (
	"testing"
	"time"

	"kr.dev/diff"
)

func TestTouch(t *testing.T) {
	c := &clock{present: time.Now()}
	lim := &Limiter{Dir: t.TempDir(), nowf: c.now}
	if !lim.Touch("foo", 0) {
		t.Error("expected touch")
	}
	if lim.Touch("foo", +1*time.Minute) {
		t.Error("expected no touch")
	}
	c.advance(-2 * time.Minute)
	if lim.Touch("foo", +1*time.Minute) {
		t.Error("expected touch")
	}

	if !lim.Touch("bar", 10*time.Hour) {
		t.Error("expected touch")
	}

	got := lim.Touched()
	want := []string{"foo", "bar"}
	diff.Test(t, t.Errorf, got, want)
}

type clock struct {
	present time.Time
}

func (c *clock) advance(d time.Duration) {
	c.present = c.present.Add(d)
}

func (c *clock) now() time.Time {
	return c.present
}
