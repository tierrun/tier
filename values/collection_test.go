package values

import (
	"testing"

	"kr.dev/diff"
)

func TestCollection(t *testing.T) {
	var c Collection[string, int]
	c.Add("a", 1)
	c.Add("a", 2)
	c.Add("b", 2)

	want := Collection[string, int]{
		"a": []int{1, 2},
		"b": []int{2},
	}

	diff.Test(t, t.Errorf, c, want)
}
