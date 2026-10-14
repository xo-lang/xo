package main

import (
	"bytes"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// TestRTPackCommand: `xo rtpack --key` prints the key of a target's pack,
// `--status` says why no pack is used, and a bad target or a target other
// than the host without --cc is an error (decision 0157).
func TestRTPackCommand(t *testing.T) {
	t.Parallel()
	var out, errb bytes.Buffer
	if code := run(t, []string{"rtpack", "--key", "--target=linux/arm64"}, &out, &errb); code != 0 {
		t.Fatalf("--key: exit %d: %s", code, errb.String())
	}
	// the key itself is the pack manifest's (selfhost/xo rtpack.xo); the
	// Go command it was compared with went with decision 0215
	if !regexp.MustCompile(`^[0-9a-f]{16,64}\n$`).MatchString(out.String()) {
		t.Errorf("--key printed %q, want a hex key", out.String())
	}
	setenv(t, "XO_RTPACK", "off")
	out.Reset()
	if code := run(t, []string{"rtpack", "--status"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "off (XO_RTPACK=off)") {
		t.Errorf("--status: exit %d: %q", code, out.String())
	}
	errb.Reset()
	if code := run(t, []string{"rtpack", "--target=plan9/mips", "-o", t.TempDir()}, &out, &errb); code != 1 || !strings.Contains(errb.String(), "prebuilt runtimes are built for") {
		t.Errorf("bad target: exit %d: %s", code, errb.String())
	}
	other := "linux/arm64"
	if runtime.GOOS+"/"+runtime.GOARCH == other {
		other = "darwin/arm64"
	}
	errb.Reset()
	if code := run(t, []string{"rtpack", "--target=" + other, "-o", t.TempDir()}, &out, &errb); code != 1 || !strings.Contains(errb.String(), "needs a cross compiler") {
		t.Errorf("cross without --cc: exit %d: %s", code, errb.String())
	}
}
