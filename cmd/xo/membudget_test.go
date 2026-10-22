package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// TestMemBudget checks decision 0349: a native program that keeps 64 MB
// stops under XO_MAX_MEM=32M with status 70, the report and its label,
// and finishes with XO_MAX_MEM=off; xo itself under a tiny budget stops
// the same way, naming its phase.
func TestMemBudget(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs programs")
	}
	dir := writeFiles(t, map[string]string{
		"app/xo.mod": "module app\n",
		"app/main.xo": `use std/debug

fn main(os: Os) {
  debug.mem_label("filling the list")
  var keep: List[Str] = []
  for i in 0..16 {
    keep.push("x".repeat(4194304) + "${i}")
  }
  os.stdio.println("done ${keep.len()} ${debug.mem_mapped() > 0}")
}
`,
	})
	app := filepath.Join(dir, "app")
	for _, be := range nativeBackends(t) {
		bin := filepath.Join(t.TempDir(), "app")
		if code, out := xoOut(t, "build", "--backend="+be, "-o", bin, app); code != 0 {
			t.Fatalf("%s: build: exit %d\n%s", be, code, out)
		}
		runEnv := func(v string) (int, string) {
			cmd := command(t, bin)
			cmd.Env = append(cmd.Environ(), "XO_MAX_MEM="+v)
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			err := cmd.Run()
			return cmd.ProcessState.ExitCode(), out.String() + errText(err)
		}
		code, out := runEnv("32M")
		if code != 70 || !strings.Contains(out, "fatal error: memory budget exceeded: ") || !strings.Contains(out, "more than XO_MAX_MEM (32 MB)") || !strings.Contains(out, "  while: filling the list") || strings.Contains(out, "done") {
			t.Errorf("%s: XO_MAX_MEM=32M: exit %d\n%s", be, code, out)
		}
		if code, out := runEnv("off"); code != 0 || !strings.Contains(out, "done 16 true") {
			t.Errorf("%s: XO_MAX_MEM=off: exit %d\n%s", be, code, out)
		}
	}
	// xo itself: a build under 16 MB stops in its first phases
	setenvCmd := func() (int, string) {
		w := xoCmd(t, "build", "-o", filepath.Join(t.TempDir(), "app"), app)
		cmd := command(t, w[0], w[1:]...)
		cmd.Env = append(cmd.Environ(), "XO_MAX_MEM=16M", "XO_BACKEND_CACHE=off", "XO_FE_CACHE=off")
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		err := cmd.Run()
		return cmd.ProcessState.ExitCode(), out.String() + errText(err)
	}
	code, out := setenvCmd()
	if code != 70 || !strings.Contains(out, "fatal error: memory budget exceeded") || !strings.Contains(out, "  while: ") {
		t.Errorf("xo build under XO_MAX_MEM=16M: exit %d\n%s", code, out)
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	if _, ok := err.(interface{ ExitCode() int }); ok {
		return ""
	}
	return "\n" + err.Error()
}
