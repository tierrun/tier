package version

import (
	"fmt"
	"runtime"
	"strings"
)

func String() string {
	var ret strings.Builder
	ret.WriteString(Short)
	ret.WriteByte('\n')
	if GitCommit != "" {
		var dirty string
		if GitDirty {
			dirty = "-dirty"
		}
		fmt.Fprintf(&ret, "  tier commit: %s%s\n", GitCommit, dirty)
	}
	fmt.Fprintf(&ret, "  go version: %s\n", runtime.Version())
	return strings.TrimSpace(ret.String())
}
