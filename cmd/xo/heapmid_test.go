package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// TestHeapMidMaxRange checks that a native program warns at start when
// XO_HEAP_MID_MAX is outside 1024 to 262136 (one word past the cap,
// 262144, used to keep the default with no word) and runs either way.
func TestHeapMidMaxRange(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs programs")
	}
	dir := writeFiles(t, map[string]string{
		"app/xo.mod": "module app\n",
		"app/main.xo": `fn main(os: Os) {
  os.stdio.println("done")
}
`,
	})
	app := filepath.Join(dir, "app")
	const warn = "xo heap: XO_HEAP_MID_MAX ignored: it takes a byte count from 1024 to 262136"
	for _, be := range nativeBackends(t) {
		bin := filepath.Join(t.TempDir(), "app")
		if code, out := xoOut(t, "build", "--backend="+be, "-o", bin, app); code != 0 {
			t.Fatalf("%s: build: exit %d\n%s", be, code, out)
		}
		for _, c := range []struct {
			v    string
			warn bool
		}{
			{"1024", false}, {"65536", false}, {"262136", false},
			{"262144", true}, {"512", true}, {"64K", true}, {"off", true},
		} {
			cmd := command(t, bin)
			cmd.Env = append(cmd.Environ(), "XO_HEAP_MID_MAX="+c.v)
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			err := cmd.Run()
			s := out.String() + errText(err)
			if cmd.ProcessState.ExitCode() != 0 || !strings.Contains(s, "done") || strings.Contains(s, warn) != c.warn {
				t.Errorf("%s: XO_HEAP_MID_MAX=%s: exit %d, want warning %v\n%s", be, c.v, cmd.ProcessState.ExitCode(), c.warn, s)
			}
		}
	}
}
