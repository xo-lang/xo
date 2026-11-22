package repotest

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestIntelMacTestLock runs scripts/intel-mac-test.sh against a fake laptop
// (ssh and scp shims over a temporary HOME) and checks the one-run lock and
// the per-run log of task #652: two runs at once shared logs/COMMIT.log, so
// a second run's lines landed in the first's log. A second run now refuses,
// naming the holder; a run with XO_INTEL_LOCK_WAIT waits for the first and
// writes its own log; a lock whose process on this machine is gone is
// taken over.
func TestIntelMacTestLock(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	tmp := t.TempDir()
	laptop := filepath.Join(tmp, "laptop")
	bin := filepath.Join(tmp, "bin")
	repo := filepath.Join(tmp, "repo")
	for _, d := range []string{filepath.Join(laptop, "xo-ci", "go", "bin"), bin, filepath.Join(repo, "scripts")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, body string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(laptop, "xo-ci", "go", "bin", "go"), "#!/bin/sh\n", 0o755)
	realBase64, err := exec.LookPath("base64")
	if err != nil {
		t.Skip("no base64")
	}
	// ssh HOST CMD...: run CMD with the laptop's HOME, as sshd runs it in
	// the user's shell (options before HOST are skipped).
	write(filepath.Join(bin, "ssh"), `#!/bin/bash
while [ "$1" = -o ]; do shift 2; done
shift
HOME=`+laptop+` exec bash -c "$*"
`, 0o755)
	// scp [-o X] [-q] SRC DST, HOST:PATH being PATH under the laptop's HOME.
	write(filepath.Join(bin, "scp"), `#!/bin/bash
a=()
for x in "$@"; do case "$x" in -q|-o|BatchMode=*) ;; *:*) a+=("`+laptop+`/${x#*:}");; *) a+=("$x");; esac; done
exec cp "${a[@]}"
`, 0o755)
	write(filepath.Join(bin, "pmset"), "#!/bin/sh\necho \"Now drawing from 'AC Power'\"\necho ' -InternalBattery-0 (id=1)\t100%; charged;'\n", 0o755)
	write(filepath.Join(bin, "caffeinate"), "#!/bin/sh\n[ \"$1\" = -i ] && shift\nexec \"$@\"\n", 0o755)
	write(filepath.Join(bin, "sw_vers"), "#!/bin/sh\necho 15.0\n", 0o755)
	write(filepath.Join(bin, "clang"), "#!/bin/sh\necho 'fake clang'\n", 0o755)
	// macOS spells base64's decode -D; GNU base64 wants -d.
	write(filepath.Join(bin, "base64"), "#!/bin/sh\nif [ \"$1\" = -D ]; then shift; exec "+realBase64+" -d \"$@\"; fi\nexec "+realBase64+" \"$@\"\n", 0o755)

	src, err := os.ReadFile(filepath.Join(root, "scripts", "intel-mac-test.sh"))
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(repo, "scripts", "intel-mac-test.sh")
	write(script, string(src), 0o755)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("add", "-A")
	git("commit", "-qm", "fake")

	type run struct {
		cmd    *exec.Cmd
		stderr lockedBuf
		out    string
	}
	gate := filepath.Join(tmp, "go-a")
	var started []*run
	// On any exit, let run a finish and stop what is still running, so a
	// failing test leaves no fake run waiting on its gate file.
	t.Cleanup(func() {
		os.WriteFile(gate, nil, 0o644)
		for _, r := range started {
			if r.cmd.ProcessState == nil {
				r.cmd.Process.Kill()
				r.cmd.Wait()
			}
		}
	})
	start := func(name, command string, env ...string) *run {
		t.Helper()
		r := &run{out: filepath.Join(tmp, name+".log")}
		r.cmd = exec.Command("bash", script, "HEAD", "--", command)
		r.cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "XO_INTEL_HOST=fake@laptop", "XO_INTEL_OUT="+r.out, "XO_INTEL_LOCK_POLL=1")
		r.cmd.Env = append(r.cmd.Env, env...)
		r.cmd.Stderr = &r.stderr
		if err := r.cmd.Start(); err != nil {
			t.Fatal(err)
		}
		started = append(started, r)
		return r
	}
	finish := func(r *run) int {
		t.Helper()
		err := r.cmd.Wait()
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return 0
	}
	lock := filepath.Join(laptop, "xo-ci", "lock")

	// A holds the laptop until the test lets it go (it waits for gate).
	a := start("a", "echo run-a; while [ ! -e "+gate+" ]; do sleep 0.1; done; echo run-a-end")
	deadline := time.Now().Add(60 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(laptop, "xo-ci", "logs")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run a never started its command; stderr:\n%s", a.stderr.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	holder, err := os.ReadFile(filepath.Join(lock, "holder"))
	if err != nil {
		t.Fatalf("run a holds no lock: %v", err)
	}

	// B refuses at once, naming A.
	b := start("b", "echo run-b")
	if rc := finish(b); rc != 3 {
		t.Fatalf("run b while a holds the laptop: exit %d, want 3; stderr:\n%s", rc, b.stderr.String())
	}
	runA := strings.Fields(string(holder))
	if !strings.Contains(b.stderr.String(), "the laptop is in use: ") || !strings.Contains(b.stderr.String(), strings.Join(runA[:4], " ")) {
		t.Errorf("run b's refusal does not name the holder %q:\n%s", holder, b.stderr.String())
	}

	// C waits for A, then runs with its own log.
	c := start("c", "echo run-c", "XO_INTEL_LOCK_WAIT=60")
	deadline = time.Now().Add(30 * time.Second)
	for !strings.Contains(c.stderr.String(), "waiting for the laptop") {
		if time.Now().After(deadline) {
			t.Fatalf("run c did not wait; stderr:\n%s", c.stderr.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	write(gate, "", 0o644)
	if rc := finish(a); rc != 0 {
		t.Fatalf("run a: exit %d; stderr:\n%s", rc, a.stderr.String())
	}
	if rc := finish(c); rc != 0 {
		t.Fatalf("run c: exit %d; stderr:\n%s", rc, c.stderr.String())
	}
	for _, x := range []struct {
		r          *run
		want, deny string
	}{{a, "run-a-end", "run-c"}, {c, "run-c", "run-a"}} {
		got, err := os.ReadFile(x.r.out)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(got)), "\n")
		if !strings.Contains(string(got), x.want+"\n") || strings.Contains(string(got), x.deny) || !strings.HasPrefix(lines[len(lines)-1], "# exit 0 ") {
			t.Errorf("log %s: want %q and no %q, ending in # exit 0; got:\n%s", x.r.out, x.want, x.deny, got)
		}
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Errorf("the lock outlived its runs: %v", err)
	}
	logs, _ := filepath.Glob(filepath.Join(laptop, "xo-ci", "logs", "*.log"))
	if len(logs) != 2 {
		t.Errorf("laptop logs %v, want one per run (2)", logs)
	}

	// A lock left by a process of this machine that is gone is taken over.
	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	host, err := exec.Command("hostname", "-s").Output()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lock, 0o755); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(lock, "holder"), "gone pid "+strconv.Itoa(dead.Process.Pid)+" on "+strings.TrimSpace(string(host))+", abc1234, run r1 since 2026-11-21T00:00:00Z\n", 0o644)
	d := start("d", "echo run-d")
	if rc := finish(d); rc != 0 || !strings.Contains(d.stderr.String(), "taking over a stale lock") {
		t.Errorf("run d over a stale lock: exit %d; stderr:\n%s", rc, d.stderr.String())
	}
}

// lockedBuf is a bytes.Buffer the test may read while a child writes it.
type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}
