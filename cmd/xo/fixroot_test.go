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
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		cmd := exec.CommandContext(ctx, xoBin(t), append(append([]string{}, c.args...), dir)...)
		cmd.Dir, cmd.Env = workdir(t), environ(t)
		var out, errOut bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errOut
		err := cmd.Run()
		timedOut := ctx.Err() == context.DeadlineExceeded
		cancel()
		if timedOut {
			t.Errorf("xo %s outside a module did not end in 60 s", c.name)
			continue
		}
		if c.want != "" && !strings.Contains(errOut.String(), c.want) {
			t.Errorf("xo %s: %v, stderr %q, want %q", c.name, err, errOut.String(), c.want)
		}
	}
}
