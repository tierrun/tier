package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"tier.run/cmd/tier/frate"
	"tier.run/envknobs"
)

func background() func() {
	v, inBackground := os.LookupEnv("_TIER_BG_TASKS")
	vlogf("background: inBackground=%v", inBackground)
	tasks := strings.Split(v, ",")
	if inBackground {
		if len(tasks) == 0 {
			return nil
		}
		if err := processBackgroundTasks(tasks); err != nil {
			vlogf("background: %v", err)
		}
		return nil
	} else {
		return startBackgroundTasks
	}
}

func startBackgroundTasks() {
	lim := &frate.Limiter{
		Dir: filepath.Join(envknobs.XDGDataHome(), "tier", "buckets"),
	}

	lim.Touch("track", 1*time.Second)
	lim.Touch("update", 24*time.Hour)

	if lim.Err() != nil {
		vlogf("errors touching: %v", lim.Errs())
	}

	if len(lim.Touched()) > 0 {
		exe, err := os.Executable()
		if err != nil {
			vlogf("background: %v", err)
			return
		}

		devNull, err := os.Open(os.DevNull)
		if err != nil {
			vlogf("background: %v", err)
			return
		}
		defer devNull.Close()

		vlogf("background: starting process %v", exe)
		vlogf("background: tracking URL: %v", envknobs.TrackingBaseURL())
		_, err = os.StartProcess(exe, []string{exe, "version"}, &os.ProcAttr{
			Files: []*os.File{devNull, devNull, devNull},
			Env: append(os.Environ(),
				"_TIER_BG_TASKS="+strings.Join(lim.Touched(), ","),
				"_TIER_EVENTS="+vhs.buf.String(),
			),
		})
		if err != nil {
			vlogf("background: error: %v", err)
			return
		}
	}
}

func processBackgroundTasks(tasks []string) error {
	vlogf("background: processing tasks: %q", tasks)
	var g errgroup.Group
	for _, name := range tasks {
		switch name {
		case "track":
			g.Go(sendEvents)
		case "update":
			g.Go(checkForUpdate)
		default:
			vlogf("background: unknown task %q", name)
		}
	}
	return g.Wait()
}
