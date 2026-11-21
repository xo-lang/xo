package repotest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSchedJobScripts runs the shell tests of xw's scheduled jobs in
// scripts/lead: golden-freecheck.sh's trap lines (task #350) and
// mainsmall.sh's trap lines and its skip of an unmoved main (task #436),
// macload.sh's overload and panic events with fake probes (task #595),
// and maindarwin.sh's run, state and trap (task #549, the Mac only).
func TestSchedJobScripts(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, s := range []string{"golden_freecheck_test.sh", "mainsmall_test.sh", "macload_test.sh", "maindarwin_test.sh"} {
		t.Run(s, func(t *testing.T) {
			out, err := exec.Command("bash", filepath.Join(root, "scripts", "lead", s)).CombinedOutput()
			if err != nil {
				t.Fatalf("%s: %v\n%s", s, err, out)
			}
		})
	}
	// #553: a setup step that fails names itself and the paths that vanished
	t.Run("golden_freecheck_test.sh setup report", func(t *testing.T) {
		cmd := exec.Command("bash", filepath.Join(root, "scripts", "lead", "golden_freecheck_test.sh"))
		cmd.Env = append(os.Environ(), "FREECHECK_TEST_VANISH=up.git")
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("passed with up.git removed:\n%s", out)
		}
		for _, want := range []string{"FAIL setup checkout clone", "vanished: ", "/up.git\n", "present: "} {
			if !strings.Contains(string(out), want) {
				t.Errorf("the report lacks %q:\n%s", want, out)
			}
		}
		if strings.Contains(string(out), "FAIL missing rev") {
			t.Errorf("went on after the failed setup:\n%s", out)
		}
	})
}
