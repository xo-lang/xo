package repotest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTierCheck runs scripts/lead/tier-check_test.sh: tier-check.py, the
// release readiness check of decision 0730 (task #567), on small tier logs
// in tier.sh's formats (every step PASS, FAIL or NO-RESULT; a no-result
// step counts as failing).
func TestTierCheck(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("no python3")
	}
	s := filepath.Join("..", "..", "scripts", "lead", "tier-check_test.sh")
	out, err := leadScript(s).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", s, err, out)
	}
}

// TestTierMac runs scripts/lead/tier-mac_test.sh: tier.sh's Mac steps
// with a stub heavy.sh and offload-gate.sh (tasks #560, #582): a memory
// stop reruns the steps one at a time with llvm on host 3, and each Mac run
// is filed as a gate log with its steps' peaks for xw gates. Nothing ran
// that script before #582.
func TestTierMac(t *testing.T) {
	s := filepath.Join("..", "..", "scripts", "lead", "tier-mac_test.sh")
	out, err := leadScript(s).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", s, err, out)
	}
}

// TestTierPlan runs scripts/lead/tier-plan_test.sh: tier.sh's host split
// (task #592): linux llvm and stdcov on host 3, the rest on host 2, and
// linux llvm never on host 2 (24 GB OOM-kills clang).
func TestTierPlan(t *testing.T) {
	s := filepath.Join("..", "..", "scripts", "lead", "tier-plan_test.sh")
	out, err := leadScript(s).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", s, err, out)
	}
}

// TestTierKeep runs scripts/lead/tier-keep_test.sh (task #596): a failed
// tier step's detail file, on the Mac or a host, is kept in
// notes/lead/tierN-fail.
func TestTierKeep(t *testing.T) {
	s := filepath.Join("..", "..", "scripts", "lead", "tier-keep_test.sh")
	out, err := leadScript(s).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", s, err, out)
	}
}

// TestTierMove runs scripts/lead/tier-move_test.sh (task #706): a linux
// tier step with no slot, or that no host took, runs on another arm64
// pass host (tier 928 dropped linux llvm and sdk-stress on host 2).
func TestTierMove(t *testing.T) {
	s := filepath.Join("..", "..", "scripts", "lead", "tier-move_test.sh")
	cmd := leadScript(s)
	cmd.Env = noGitIdentity(t, cmd.Env)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", s, err, out)
	}
}

// noGitIdentity: env with no git user identity, as on host 3 (task #711:
// TestTierMove failed there with "unable to auto-detect email address"):
// an empty HOME, no system config, no identity variables, and
// user.useConfigOnly so git does not guess one from the host name.
func noGitIdentity(t *testing.T, env []string) []string {
	var out []string
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		switch {
		case k == "HOME" || k == "XDG_CONFIG_HOME" || k == "EMAIL" || strings.HasPrefix(k, "GIT_"):
			continue
		}
		out = append(out, kv)
	}
	return append(out, "HOME="+t.TempDir(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=user.useConfigOnly", "GIT_CONFIG_VALUE_0=true")
}

// TestOffloadTierUnknownStep runs scripts/lead/offload_tier_step_test.sh
// (task #714): offload-tier.sh refuses a STEP it has no step for, which
// ran nothing and exited 0 for tier 928's sdk-stress rerun.
func TestOffloadTierUnknownStep(t *testing.T) {
	s := filepath.Join("..", "..", "scripts", "lead", "offload_tier_step_test.sh")
	out, err := leadScript(s).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", s, err, out)
	}
}

// TestTierHeavyClass: every heavy.sh run in scripts/lead/tier.sh is bulk
// class with HEAVY_DARWIN=1 (task #556): a tier step runs for minutes on
// the Mac, and the docs step (docs-check.sh, 12+ min in tier 918) ran as
// gate class, which raised the wrong-class alert (#97144) every tier.
func TestTierHeavyClass(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "scripts", "lead", "tier.sh"))
	if err != nil {
		t.Fatal(err)
	}
	src := strings.ReplaceAll(string(b), "\\\n", " ")
	n := 0
	for i, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") || !strings.Contains(line, "agents/heavy.sh\" \"") {
			continue
		}
		n++
		if !strings.Contains(line, "HEAVY_CLASS=bulk") || !strings.Contains(line, "HEAVY_DARWIN=1") {
			t.Errorf("tier.sh logical line %d runs heavy.sh without HEAVY_CLASS=bulk HEAVY_DARWIN=1: %s", i+1, strings.TrimSpace(line))
		}
	}
	if n == 0 {
		t.Fatal("found no heavy.sh run in tier.sh")
	}
}
