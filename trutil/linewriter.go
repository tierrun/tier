package trutil

import (
	"bytes"
	"strings"
)

type LineWriter struct {
	Prefix    string
	Logf      func(string, ...any)
	AutoFlush bool // flush after every write if true

	lineBuf strings.Builder
}

func (lw *LineWriter) Flush() error {
	lw.Logf("%s%s", lw.Prefix, lw.lineBuf.String())
	lw.lineBuf.Reset()
	return nil
}

var newline = []byte{'\n'}

func (lw *LineWriter) Write(p []byte) (n int, err error) {
	if lw.AutoFlush {
		defer lw.Flush()
	}
	p0 := p
	for {
		before, after, hasNewline := bytes.Cut(p, newline)
		lw.lineBuf.Write(before)
		if hasNewline {
			lw.lineBuf.WriteByte('\n')
			if err := lw.Flush(); err != nil {
				return 0, err
			}
			p = after
		} else {
			return len(p0), nil
		}
	}
}
