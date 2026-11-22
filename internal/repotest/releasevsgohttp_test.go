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
// A fake offload.sh under a temporary HOME records each job's lease, and
// whether its bundle was there (the host script removes it once fetched).
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
	// the host's bundle is a marker file: --put writes it, and a job takes it
	// and removes it, as release-vs-go-http.sh does once fetched, so a lease
	// with no bundle put before it records "nobundle"
	host := filepath.Join(tmp, "hostbundle")
	off := "#!/bin/sh\nif [ \"$1\" = --put ]; then [ -f \"$2\" ] && touch " + host + "; exit 0; fi\n" +
		"b=ok; [ -f " + host + " ] || b=nobundle; rm -f " + host + "\n" +
		"echo \"max=${OFF_MAX_MIN:-none} excl=${OFF_EXCLUSIVE:-} rounds=$8 pads=$9 bundle=$b\" >>" + rec + "\necho 'host load 0.1 0.1 0.1 1/1 1'\n"
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
		if j != "max=30 excl=1 rounds=1 pads=0 1 bundle=ok" {
			t.Errorf("lease %q, want max=30 excl=1 rounds=1 pads=0 1 bundle=ok", j)
		}
	}
}
