package repotest

import (
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

var updateClosure = flag.Bool("update-closure", false, "rewrite scripts/lead/xo-closure.txt from xo build --list-inputs compiler/xo")

// TestXoClosureCurrent: scripts/lead/xo-closure.txt lists the std files
// compiler/xo reads, as `xo build --list-inputs compiler/xo` gives them,
// so gate_required_steps asks for selfbuild and seedbuild exactly when a
// change touches one, and the std fast lane (gate step stdfast) takes the
// rest (lead, owner direction 2026-11-15). -update-closure rewrites it.
func TestXoClosureCurrent(t *testing.T) {
	r, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo := os.Getenv("XO_CLI")
	if xo == "" {
		if xo, err = xotool.Ensure(r, io.Discard); err != nil {
			t.Skipf("no xo: %v", err)
		}
	}
	out, err := exec.Command(xo, "build", "--list-inputs", filepath.Join(r, "compiler", "xo")).Output()
	if err != nil {
		t.Skipf("xo build --list-inputs: %v (an xo without it)", err)
	}
	var want []string
	for _, l := range strings.Split(string(out), "\n") {
		// std/native, std/llrt and std/rt are in the closure whole (gatelib
		// gate_required_steps), so the list names the other std files only:
		// it then changes when a top-level std module enters the closure, not
		// with each new llrt file (2026-11-15: tlscache, tlsech and tlsquic
		// in one hour)
		if strings.HasPrefix(l, "std/") && !strings.HasPrefix(l, "std/native/") && !strings.HasPrefix(l, "std/llrt/") && !strings.HasPrefix(l, "std/rt/") {
			want = append(want, l)
		}
	}
	path := filepath.Join(r, "scripts", "lead", "xo-closure.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	head, got := "", []string{}
	for _, l := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(l, "#"):
			head += l + "\n"
		case l != "":
			got = append(got, l)
		}
	}
	if *updateClosure {
		if err := os.WriteFile(path, []byte(head+strings.Join(want, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("scripts/lead/xo-closure.txt is not compiler/xo's std closure (%d lines, xo build --list-inputs gives %d): run go test ./internal/repotest -run TestXoClosureCurrent -update-closure", len(got), len(want))
	}
}
