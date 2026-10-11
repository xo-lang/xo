package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
)

// TestRTPackCommand: `xo rtpack --key` prints the key of a target's pack,
// `--status` says why no pack is used, and a bad target or a target other
// than the host without --cc is an error (decision 0157).
func TestRTPackCommand(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"rtpack", "--key", "--target=linux/arm64"}, &out, &errb); code != 0 {
		t.Fatalf("--key: exit %d: %s", code, errb.String())
	}
	want, _ := build.RTPackKey(build.Target{OS: "linux", Arch: "arm64"})
	if strings.TrimSpace(out.String()) != want {
		t.Errorf("--key printed %q, want %q", out.String(), want)
	}
	t.Setenv("XO_RTPACK", "off")
	out.Reset()
	if code := run([]string{"rtpack", "--status"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "off (XO_RTPACK=off)") {
		t.Errorf("--status: exit %d: %q", code, out.String())
	}
	errb.Reset()
	if code := run([]string{"rtpack", "--target=plan9/mips", "-o", t.TempDir()}, &out, &errb); code != 1 || !strings.Contains(errb.String(), "prebuilt runtimes are built for") {
		t.Errorf("bad target: exit %d: %s", code, errb.String())
	}
	other := build.Target{OS: "linux", Arch: "arm64"}
	if build.Host() == other {
		other = build.Target{OS: "darwin", Arch: "arm64"}
	}
	errb.Reset()
	if code := run([]string{"rtpack", "--target=" + other.String(), "-o", t.TempDir()}, &out, &errb); code != 1 || !strings.Contains(errb.String(), "needs a cross compiler") {
		t.Errorf("cross without --cc: exit %d: %s", code, errb.String())
	}
}
