package cline

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/exp/slices"
)

var testTier string

func TestMain(m *testing.M, run func()) {
	if os.Getenv("CLINE_TEST_RUN_MAIN") != "" {
		run()
		os.Exit(0)
	}
	os.Setenv("CLINE_TEST_RUN_MAIN", "true")

	// The exit code is captured at the end, but defers the os.Exit to this
	// defer so that all defers that come after this one are run
	var code int
	defer func() {
		os.Exit(code)
	}()

	testExe, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}

	testTempDir, err := os.MkdirTemp("", "tier-test")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(testTempDir)

	testTier = testTempDir + "/tier"

	// symlink the test executable to the temp dir or copy if the host OS
	// does not support symlinks.
	if err := os.Symlink(testExe, testTier); err != nil {
		// Otherwise, copy the bytes.
		src, err := os.Open(testExe)
		if err != nil {
			log.Fatal(err)
		}
		defer src.Close()

		dst, err := os.OpenFile(testTier, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o777)
		if err != nil {
			log.Fatal(err)
		}

		_, err = io.Copy(dst, src)
		if closeErr := dst.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			log.Fatal(err)
		}
	}

	code = m.Run()
	// allow defers to run
}

type Data struct {
	t      *testing.T
	stdin  io.Reader
	stdout bytes.Buffer
	stderr bytes.Buffer
	env    []string
	ran    bool
}

func Test(t *testing.T) *Data {
	return &Data{t: t}
}

func (d *Data) Setenv(name, value string) {
	d.t.Helper()
	d.Unsetenv(name)
	d.env = append(d.env, name+"="+value)
}

func (d *Data) Unsetenv(name string) {
	d.t.Helper()
	if d.env == nil {
		d.env = slices.Clone(os.Environ())
	}
	for i, e := range d.env {
		if strings.HasPrefix(e, name+"=") {
			d.env = slices.Delete(d.env, i, i+1)
			return
		}
	}
}

func (d *Data) RunFail(args ...string) {
	d.t.Helper()
	if status := d.doRun(args...); status == nil {
		d.t.Fatal("succeeded unexpectedly")
	} else {
		d.t.Log("failed as expected:", status)
	}
}

func (d *Data) Run(args ...string) {
	d.t.Helper()
	if err := d.doRun(args...); err != nil {
		d.t.Fatal(err)
	}
}

func (d *Data) doRun(args ...string) error {
	d.t.Helper()
	d.stdout.Reset()
	d.stderr.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, testTier, args...)
	cmd.Stdin = d.stdin
	cmd.Stdout = &d.stdout
	cmd.Stderr = &d.stderr
	cmd.Env = d.env
	status := cmd.Run()
	if d.stdout.Len() > 0 {
		d.t.Logf("standard output:\n%s", d.stdout.String())
	}
	if d.stderr.Len() > 0 {
		d.t.Logf("standard error:\n%s", d.stderr.String())
	}
	d.ran = true
	return status
}

func (d *Data) GrepStdout(match, msg string) {
	d.t.Helper()
	d.doGrep(match, &d.stdout, "output", msg)
}

func (d *Data) GrepStderr(match, msg string) {
	d.t.Helper()
	d.doGrep(match, &d.stderr, "error", msg)
}

func (d *Data) GrepStdoutNot(match, msg string) {
	d.t.Helper()
	d.doGrepNot(match, &d.stdout, "output", msg)
}

func (d *Data) GrepStderrNot(match, msg string) {
	d.t.Helper()
	d.doGrepNot(match, &d.stderr, "error", msg)
}

func (d *Data) GrepBothNot(match, msg string) {
	d.t.Helper()
	if d.doGrepMatch(match, &d.stdout) || d.doGrepMatch(match, &d.stderr) {
		d.t.Errorf("pattern %q found in standard output or standard error: %s", match, msg)
	}
}

func (d *Data) GrepBoth(match, msg string) {
	d.t.Helper()
	if !d.doGrepMatch(match, &d.stdout) && !d.doGrepMatch(match, &d.stderr) {
		d.t.Errorf("pattern %q found in standard output or standard error: %s", match, msg)
	}
}

func (d *Data) SetStdin(r io.Reader) {
	d.stdin = r
}

// doGrep looks for a regular expression in a buffer and fails if it
// is not found. The name argument is the name of the output we are
// searching, "output" or "error". The msg argument is logged on
// failure.
func (d *Data) doGrep(match string, b *bytes.Buffer, name, msg string) {
	d.t.Helper()
	if !d.doGrepMatch(match, b) {
		d.t.Log(msg)
		d.t.Logf("pattern %q not found in standard %s", match, name)
		d.t.FailNow()
	}
}

func (d *Data) doGrepNot(match string, b *bytes.Buffer, name, msg string) {
	d.t.Helper()
	if d.doGrepMatch(match, b) {
		d.t.Log(msg)
		d.t.Logf("pattern %q found in standard %s", match, name)
		d.t.FailNow()
	}
}

func (d *Data) doGrepMatch(match string, b *bytes.Buffer) bool {
	d.t.Helper()
	if !d.ran {
		d.t.Fatal("internal testsuite error: grep called before run")
	}
	re := regexp.MustCompile(match)
	for _, ln := range bytes.Split(b.Bytes(), []byte{'\n'}) {
		if re.Match(ln) {
			return true
		}
	}
	return false

}
