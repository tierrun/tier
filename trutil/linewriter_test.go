package trutil

import (
	"fmt"
	"io"
	"testing"

	"kr.dev/diff"
)

func TestLineWriter(t *testing.T) {
	var writes []string
	lw := &LineWriter{
		Logf: func(format string, args ...any) {
			writes = append(writes, fmt.Sprintf(format, args...))
		},
	}

	chunks := []string{
		"hi", "there", "line", "things\n",
		"are\n",
		"nice\n",
		"yo", "helllkjalksjf             askjf", "\n",
		"", "", "", "\n",
		"boom", "done.\n",
		"many\nlines\none\nchunk\nno ",
		"newline at end of file",
	}

	for _, chunk := range chunks {
		io.WriteString(lw, chunk) // nolint: errcheck
	}

	want := []string{
		"hitherelinethings\n",
		"are\n",
		"nice\n",
		"yohelllkjalksjf             askjf\n",
		"\n",
		"boomdone.\n",
		"many\n",
		"lines\n",
		"one\n",
		"chunk\n",
	}
	diff.Test(t, t.Errorf, writes, want)

	writes = nil
	want = []string{
		"no newline at end of file",
	}
	if err := lw.Flush(); err != nil {
		t.Fatal(err)
	}
	diff.Test(t, t.Errorf, writes, want)
}
