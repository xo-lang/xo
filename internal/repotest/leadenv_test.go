package repotest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// leadRunPrefixes are the run settings a gate or tier passes to its own
// steps. A lead script test run inside one inherited them (task #655: tier
// 924's repo step failed tier-plan_test.sh on TIER_XW_PLAN=1, gatelib_test.sh
// on GATE_STALE_OK=1, pushq_env and gate_failfast on the gate's names), so
// leadScriptEnv drops them.
var leadRunPrefixes = []string{"TIER_", "GATE_", "XO_GATE_", "OFFGATE_", "OFF_", "OFFLOAD_ENV=", "PUSHQ_"}

// leadScriptEnv is os.Environ() without the caller's run settings.
func leadScriptEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		drop := false
		for _, p := range leadRunPrefixes {
			if strings.HasPrefix(kv, p) {
				drop = true
				break
			}
		}
		if !drop {
			env = append(env, kv)
		}
	}
	return env
}

// leadScript is exec.Command("bash", args...) with leadScriptEnv.
func leadScript(args ...string) *exec.Cmd {
	cmd := exec.Command("bash", args...)
	cmd.Env = leadScriptEnv()
	return cmd
}

// TestLeadScriptEnv sets a tier's settings (task #655): leadScriptEnv drops
// them, and tier-plan_test.sh (run by TestTierPlan through leadScript)
// passes with them set; it failed that way before. gatelib_test.sh, which
// failed on GATE_STALE_OK=1, is TestLeadShell's (shelltests.txt), which
// uses leadScriptEnv too.
func TestLeadScriptEnv(t *testing.T) {
	set := map[string]string{"TIER_XW_PLAN": "1", "TIER_HOST2_STEPS": "", "GATE_STALE_OK": "1", "XO_GATE_FAILFAST": "0", "OFFGATE_AGENT": "x", "OFFLOAD_ENV": "offload-arm64c.env", "OFF_MAX_MIN": "9", "PUSHQ_RAN": "repo"}
	for k, v := range set {
		t.Setenv(k, v)
	}
	for _, kv := range leadScriptEnv() {
		if k, _, _ := strings.Cut(kv, "="); set[k] != "" || k == "TIER_HOST2_STEPS" {
			t.Errorf("leadScriptEnv kept %s", k)
		}
	}
	out, err := leadScript(filepath.Join("..", "..", "scripts", "lead", "tier-plan_test.sh")).CombinedOutput()
	if err != nil {
		t.Errorf("tier-plan_test.sh with a tier's settings: %v\n%s", err, out)
	}
}
