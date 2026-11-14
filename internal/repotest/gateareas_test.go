package repotest

import (
	"os/exec"
	"strings"
	"testing"
)

// TestGateAreasValid: scripts/gate-areas.txt scopes slow tests of
// internal/llvm, internal/arm64 and cmd/xo to the paths that can break
// them, so a per-push gate skips them for other changes (lead, 2026-11-14,
// owner's test-time plan; the full tier runs every test). A renamed or
// removed test would silently fall out of that list, so every test it names
// must exist in its package, every @area must be defined and every regex
// must compile (scripts/gate-areas.py --check). It also checks a few
// selections: a docs change skips every scoped test, a crypto change runs
// the TLS differential, and a build driver change runs every llvm test.
func TestGateAreasValid(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("no python3")
	}
	cmd := exec.Command("python3", "scripts/gate-areas.py", "--check", "scripts/gate-areas.txt")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gate-areas.py --check: %v\n%s", err, out)
	}
	sel := func(pkg, changed string) string {
		t.Helper()
		note := t.TempDir() + "/note"
		c := exec.Command("python3", "scripts/gate-areas.py", pkg, "scripts/gate-areas.txt", note)
		c.Dir = root
		c.Stdin = strings.NewReader(changed + "\n")
		out, err := c.Output()
		if err != nil {
			t.Fatalf("gate-areas.py %s: %v", pkg, err)
		}
		return strings.TrimSpace(string(out))
	}
	if s := sel("internal/llvm", "knowledgebase/x.md"); !strings.Contains(s, "TestTLSDiff") || !strings.Contains(s, "TestRaceSyncTable") {
		t.Errorf("a docs change: want TestTLSDiff and TestRaceSyncTable skipped, skip pattern %q", s)
	}
	if s := sel("internal/llvm", "std/tls.xo"); strings.Contains(s, "TestTLSDiff|") || strings.Contains(s, "|TestTLSDiff)") {
		t.Errorf("a std/tls change: want TestTLSDiff run, skip pattern %q", s)
	}
	if s := sel("internal/llvm", "compiler/xo/link.xo"); s != "" {
		t.Errorf("a build driver change: want every llvm test run, skip pattern %q", s)
	}
	if s := sel("internal/arm64", "compiler/ssa/inline.xo"); !strings.Contains(s, "TestModuleCheckAuto") {
		t.Errorf("TestModuleCheckAuto: want it skipped in the arm64 step (the chkcache step runs it), skip pattern %q", s)
	}
	// gate.sh's XO_LLVM_SAN_ALL: every sanitizer golden for a scheduler
	// change, the selection's for a compiler pass
	touches := func(changed string) bool {
		c := exec.Command("python3", "scripts/gate-areas.py", "--touches", "scripts/gate-areas.txt", "@rt", "@sched", "@race", "@http", "@llvmbe", "@driver")
		c.Dir = root
		c.Stdin = strings.NewReader(changed + "\n")
		return c.Run() == nil
	}
	if !touches("std/native/llrt/sched.xo") || touches("compiler/ssa/inline.xo") {
		t.Errorf("--touches: want std/native/llrt/sched.xo in the areas and compiler/ssa/inline.xo not")
	}
}
