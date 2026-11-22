package repotest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestReleaseVsGoHTTPLeases checks scripts/release-vs-go.sh's http section
// (task #645): host 1 refuses an exclusive job over 30 minutes, so the
// section asks for one lease per round, each OFF_MAX_MIN=30 and one round,
// instead of one lease of every round under offload.sh's 180-minute default.
// A fake offload.sh under a temporary HOME records each job's lease.
func TestReleaseVsGoHTTPLeases(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	agents := filepath.Join(home, "xo-lead", "agents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := filepath.Join(tmp, "jobs.txt")
	off := "#!/bin/sh\n[ \"$1\" = --put ] && exit 0\necho \"max=${OFF_MAX_MIN:-none} excl=${OFF_EXCLUSIVE:-} rounds=$8 pads=$9\" >>" + rec + "\necho 'host load 0.1 0.1 0.1 1/1 1'\n"
	if err := os.WriteFile(filepath.Join(agents, "offload.sh"), []byte(off), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "xo-lead", "offload.env"), []byte("OFF_SEED=/seed/xo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(tmp, "xo")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho 'xo 0.0.0-fake'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "release-vs-go.sh"), fake, filepath.Join(tmp, "out"))
	cmd.Env = append(os.Environ(), "HOME="+home, "SECTIONS=http", "HTTP_BASE_XO=/base/xo", "HTTP_PADS=0 1", "HTTP_ROUNDS=3", "HTTP_REV=HEAD")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("release-vs-go.sh: %v\n%s", err, out)
	}
	b, err := os.ReadFile(rec)
	if err != nil {
		t.Fatalf("no offload job ran: %v", err)
	}
	jobs := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(jobs) != 3 {
		t.Fatalf("%d leases for 3 rounds, want 3:\n%s", len(jobs), b)
	}
	for _, j := range jobs {
		if j != "max=30 excl=1 rounds=1 pads=0 1" {
			t.Errorf("lease %q, want max=30 excl=1 rounds=1 pads=0 1", j)
		}
	}
}
