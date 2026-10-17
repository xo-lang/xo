package llvm

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestZonesWithoutGo (decision 0094's addendum): a native build of a
// program that names time zones takes their data from std's IANA
// database (std/tzdata/zoneinfo.zip), with no go command on PATH and no
// XO_ZONEINFO: a constant name embeds that zone, a name known at run time
// the whole database.
func TestZonesWithoutGo(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two native programs")
	}
	if runtime.GOARCH != "arm64" || (runtime.GOOS != "darwin" && runtime.GOOS != "linux") {
		t.Skip("the arm64 backend links without cc on darwin/arm64 and linux/arm64 only")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Skip(err)
	}
	// PATH without any directory that has a go command
	var dirs []string
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		if _, err := os.Stat(filepath.Join(d, "go")); err == nil {
			continue
		}
		dirs = append(dirs, d)
	}
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "PATH=") || strings.HasPrefix(kv, "XO_ZONEINFO=") || strings.HasPrefix(kv, "GOROOT=") || strings.HasPrefix(kv, "XO_GO=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "PATH="+strings.Join(dirs, string(os.PathListSeparator)), "XO_BIN_CACHE=off")
	if p, err := exec.LookPath("go"); err == nil {
		// LookPath used this process's PATH; check the one the build gets
		for _, d := range dirs {
			if filepath.Dir(p) == d {
				t.Fatalf("go is still on the build's PATH: %s", p)
			}
		}
	}
	for _, c := range []struct {
		dir  string
		args []string
		want []string
	}{
		{"constant", nil, []string{"Europe/Berlin: Wed, 01 Jul 2026 14:00:00 CEST 2h0m0s"}},
		{"dynamic", []string{"Asia/Tokyo", "America/New_York", "Nowhere/Nothing"}, []string{"Asia/Tokyo: Wed, 01 Jul 2026 21:00:00 JST 9h0m0s", "America/New_York: Wed, 01 Jul 2026 08:00:00 EDT -4h0m0s", "Nowhere/Nothing: "}},
	} {
		t.Run(c.dir, func(t *testing.T) {
			bin := filepath.Join(t.TempDir(), c.dir)
			cmd := exec.Command(xo, "build", "--backend=arm64", "-o", bin, ".")
			cmd.Dir = filepath.Join("testdata", "tzdata", c.dir)
			cmd.Env = env
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("xo build without go: %v\n%s", err, b)
			}
			out, err := exec.Command(bin, c.args...).CombinedOutput()
			if err != nil {
				t.Fatalf("run: %v\n%s", err, out)
			}
			for _, w := range c.want {
				if !strings.Contains(string(out), w) {
					t.Errorf("output lacks %q:\n%s", w, out)
				}
			}
		})
	}
}
