package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
)

// TestRTPackXoSame builds the host's prebuilt runtime twice, with this
// package's `xo rtpack` in process and with the binary $XO_CLI names (the
// self hosted CLI, selfhost/xo rtpack.xo), and wants the same files byte
// for byte: every variant's object, libxotls.a, and rtpack.json
// (scripts/release.sh runs `xo rtpack`; decision 0178 switch point B).
// Both packs are then read back by `--status` with $XO_RTPACK. Needs
// $XO_CLI, a C compiler, and a few minutes; skipped with -short.
func TestRTPackXoSame(t *testing.T) {
	cli := strings.Fields(os.Getenv(cliEnv))
	if len(cli) == 0 {
		t.Skip("set XO_CLI to the self hosted xo")
	}
	if testing.Short() {
		t.Skip("two packs of 32 objects and the TLS library; run without -short")
	}
	if _, err := build.CC(); err != nil {
		t.Skip(err)
	}
	host := build.Host()
	ok := false
	for _, tg := range build.RTPackTargets {
		ok = ok || tg == host
	}
	if !ok {
		t.Skipf("no packs for %s", host)
	}
	goDir, xoDir := t.TempDir(), t.TempDir()
	var out, errb bytes.Buffer
	if code := runXo([]string{"rtpack", "-o", goDir}, &out, &errb); code != 0 {
		t.Fatalf("go rtpack: exit %d: %s", code, errb.String())
	}
	var xout, xerr bytes.Buffer
	if code := execCLI(cli, []string{"rtpack", "-o", xoDir}, &xout, &xerr); code != 0 {
		t.Fatalf("XO_CLI rtpack: exit %d: %s", code, xerr.String())
	}
	sub := host.OS + "-" + host.Arch
	if got, want := strings.TrimSpace(xout.String()), filepath.Join(xoDir, sub); got != want {
		t.Errorf("XO_CLI rtpack printed %q, want %q", got, want)
	}
	ents, err := os.ReadDir(filepath.Join(goDir, sub))
	if err != nil {
		t.Fatal(err)
	}
	xents, _ := os.ReadDir(filepath.Join(xoDir, sub))
	if len(xents) != len(ents) {
		t.Errorf("XO_CLI wrote %d files, go %d", len(xents), len(ents))
	}
	same := 0
	for _, e := range ents {
		a, _ := os.ReadFile(filepath.Join(goDir, sub, e.Name()))
		b, err := os.ReadFile(filepath.Join(xoDir, sub, e.Name()))
		if err != nil {
			t.Errorf("%s: %v", e.Name(), err)
			continue
		}
		if !bytes.Equal(a, b) {
			t.Errorf("%s differs (go %d bytes, XO_CLI %d bytes)", e.Name(), len(a), len(b))
			if e.Name() == build.RTPackManifestName {
				t.Logf("go:\n%s\nXO_CLI:\n%s", a, b)
			}
			continue
		}
		same++
	}
	t.Logf("%d of %d files byte identical", same, len(ents))
	// both read the XO_CLI pack back as usable
	t.Setenv("XO_RTPACK", xoDir)
	out.Reset()
	xout.Reset()
	runXo([]string{"rtpack", "--status"}, &out, &errb)
	execCLI(cli, []string{"rtpack", "--status"}, &xout, &xerr)
	if !strings.Contains(out.String(), "prebuilt runtime: "+filepath.Join(xoDir, sub)+" (") {
		t.Errorf("go --status: %s", out.String())
	}
	if xout.String() != out.String() {
		t.Errorf("--status: XO_CLI %q, go %q", xout.String(), out.String())
	}
}
