package repotest

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestSchedJobScripts runs the shell tests of xw's scheduled jobs in
// scripts/lead: golden-freecheck.sh's trap lines (task #350) and
// mainsmall.sh's trap lines and its skip of an unmoved main (task #436).
func TestSchedJobScripts(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, s := range []string{"golden_freecheck_test.sh", "mainsmall_test.sh"} {
		t.Run(s, func(t *testing.T) {
			out, err := exec.Command("bash", filepath.Join(root, "scripts", "lead", s)).CombinedOutput()
			if err != nil {
				t.Fatalf("%s: %v\n%s", s, err, out)
			}
		})
	}
}
