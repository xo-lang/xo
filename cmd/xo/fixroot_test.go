package main

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNoModuleClimbEnds: a command that looks for xo.mod above a directory
// outside any module stops at the volume's root. On Windows the climb from
// C:\... never reached "/" and xo fix, xo check --fix (and so xo mcp's
// check tool with fix) and xo mod looped for good (2026-11-10).
func TestNoModuleClimbEnds(t *testing.T) {
	t.Parallel()
	src := "fn f(n: Int) -> Int {\n  if n > 0 {\n    return 1\n  }\n  else {\n    return 2\n  }\n}\n"
	for _, c := range []struct {
		name string
		args []string
		want string
	}{
		{"fix dry run", []string{"fix", "--dry-run"}, ""},
		{"check fix", []string{"check", "--json", "--fix"}, ""},
		{"mod tidy", []string{"mod", "tidy", "--dir"}, "no xo.mod"},
	} {
		dir := filepath.Join(writeFiles(t, map[string]string{"app/main.xo": src}), "app")
		// a climb that never ends spins: it uses the CPU it is given. A
		// run past 60 s that used little CPU was starved or blocked by a
		// loaded machine (full tier 77, 2026-11-15: check fix took over
		// 60 s beside the whole tier, 0.1 s alone and 3.4 s beside cmd/xo's
		// slowest tests), so it gets one more try at 5 minutes
		var errOut bytes.Buffer
		var err error
		hung := false
		for try, limit := range []time.Duration{60 * time.Second, 5 * time.Minute} {
			ctx, cancel := context.WithTimeout(context.Background(), limit)
			cmd := exec.CommandContext(ctx, xoBin(t), append(append([]string{}, c.args...), dir)...)
			cmd.Dir, cmd.Env = workdir(t), environ(t)
			var out bytes.Buffer
			errOut.Reset()
			cmd.Stdout, cmd.Stderr = &out, &errOut
			err = cmd.Run()
			timedOut := ctx.Err() == context.DeadlineExceeded
			cancel()
			if !timedOut {
				hung = false
				break
			}
			hung = true
			var cpu time.Duration
			if cmd.ProcessState != nil {
				cpu = cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime()
			}
			if try == 0 && cpu < limit/4 {
				t.Logf("xo %s did not end in %v but used %v of CPU (a loaded machine, not a loop): trying again", c.name, limit, cpu.Round(time.Millisecond))
				continue
			}
			t.Errorf("xo %s outside a module did not end in %v (%v of CPU)", c.name, limit, cpu.Round(time.Millisecond))
			break
		}
		if hung {
			continue
		}
		if c.want != "" && !strings.Contains(errOut.String(), c.want) {
			t.Errorf("xo %s: %v, stderr %q, want %q", c.name, err, errOut.String(), c.want)
		}
	}
}
