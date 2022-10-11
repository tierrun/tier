package stroke

import (
	"testing"
	"time"
)

func TestClock(t *testing.T) {
	c := Client(t)
	now := time.Now()
	clock := NewClock(t, c, "test", now)

	want := now.Truncate(time.Second)
	if got := clock.Now(); !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	clock.Advance(now.Add(time.Hour))
	want = want.Add(time.Hour)
	if got := clock.Now(); !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
