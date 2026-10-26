// Package devgoldens holds the thin Go tests over `xo dev` (decision
// 0209): each runs one harness of the xo the tests use (internal/xotool)
// and fails with the programs that failed. The harness logic (building,
// running, comparing byte for byte, the pass cache, the workers) is in
// Xo, compiler/cli/dev; these tests only keep `go test` tiers and the EC2
// batches running it. -short samples every 25th golden program; the
// default runs all with the pass cache; XO_DEV_FORCE=1 (the full tier,
// releases) ignores the cache.
package devgoldens

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/runfilter"
	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// outcome is one program line of `xo dev goldens --json`.
type outcome struct {
	Name    string `json:"name"`
	Dir     string `json:"dir"`
	Status  string `json:"status"`
	Why     string `json:"why"`
	BuildMS int    `json:"build_ms"`
	RunMS   int    `json:"run_ms"`
}

// summary is its last line.
type summary struct {
	Programs int `json:"programs"`
	Pass     int `json:"pass"`
	Cached   int `json:"cached"`
	Fail     int `json:"fail"`
	WallMS   int `json:"wall_ms"`
}

func jobs() string {
	if v := os.Getenv("XO_DEV_JOBS"); v != "" {
		return v
	}
	return strconv.Itoa(max(1, min(runtime.NumCPU(), 8)))
}

// devGoldens runs `xo dev goldens --json` with args from the repository
// root and fails t with each failing program.
func devGoldens(t *testing.T, args ...string) {
	t.Helper()
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	full := []string{"dev", "goldens", "--json", "-j", jobs()}
	if testing.Short() {
		full = append(full, "--sample=25")
	}
	if os.Getenv("XO_DEV_FORCE") == "1" {
		full = append(full, "--force")
	}
	// -run TestDevGoldensX/NAME (and -skip) choose the programs built,
	// not only the subtests reported: --only keeps the directories that
	// contain /NAME (a longer name that starts with NAME is built too and
	// not reported)
	sel, filtered := runfilter.Sub(t)
	chosen := map[string]bool{}
	if filtered {
		var only []string
		for _, a := range args {
			if strings.HasPrefix(a, "-") {
				continue
			}
			ents, _ := os.ReadDir(filepath.Join(root, a))
			for _, e := range ents {
				if e.IsDir() && sel(e.Name()) {
					chosen[e.Name()] = true
					only = append(only, "/"+e.Name())
				}
			}
		}
		if len(only) == 0 {
			t.Skip("-run selects no program")
		}
		full = append(full, "--only="+strings.Join(only, ","))
	}
	full = append(full, args...)
	cmd := testtmp.Guard(exec.Command(xo, full...))
	cmd.Dir = root
	// xo takes its working directory from $PWD, which go test leaves at
	// the package directory; a wrong one hid gc_config's xo.mod gc lines
	cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "PWD="+root)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	var sum summary
	var fails []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		line := sc.Bytes()
		if bytes.Contains(line, []byte(`"programs":`)) {
			if jerr := json.Unmarshal(line, &sum); jerr != nil {
				t.Fatalf("summary %q: %v", line, jerr)
			}
			continue
		}
		var o outcome
		if jerr := json.Unmarshal(line, &o); jerr != nil {
			t.Fatalf("line %q: %v", line, jerr)
		}
		if filtered && !chosen[filepath.Base(o.Dir)] && o.Name != "(discovery)" {
			continue
		}
		if o.Status == "fail" {
			fails = append(fails, o.Dir+": "+o.Why)
		}
		// one subtest a program, so -run selects programs as it does for
		// the other golden drivers
		t.Run(filepath.Base(o.Dir), func(t *testing.T) {
			if o.Status == "fail" {
				t.Errorf("%s: %s", o.Dir, o.Why)
			}
		})
	}
	if sum.Programs == 0 {
		t.Fatalf("xo %s: no summary (%v)\n%s%s", strings.Join(full, " "), err, out, stderr.String())
	}
	t.Logf("%d programs: %d pass (%d cached), %d fail, %d ms", sum.Programs, sum.Pass, sum.Cached, sum.Fail, sum.WallMS)
	if len(fails) > 0 {
		t.Logf("%d failing:\n%s", len(fails), strings.Join(fails, "\n"))
	}
}

func nativeArm64(t *testing.T) {
	if runtime.GOARCH != "arm64" || (runtime.GOOS != "darwin" && runtime.GOOS != "linux") {
		t.Skip("the arm64 backend builds on darwin/arm64 and linux/arm64 hosts")
	}
}

// TestDevGoldensArm64 is scripts/selfhost-goldens.py's arm64 run.
func TestDevGoldensArm64(t *testing.T) {
	nativeArm64(t)
	// 240 s a build, as for LLVM: crypto_prims timed out at 120 s at load
	// 49 to 71 on the Mac (2026-10-19), 4.0 s when idle
	devGoldens(t, "--backend=arm64", "--timeout=240", "internal/llvm/testdata/golden", "internal/arm64/testdata/run")
}

// TestDevGoldensLLVM is scripts/selfhost-goldens.py with XO1_BACKEND=llvm.
func TestDevGoldensLLVM(t *testing.T) {
	if _, err := xotool.Clang(); err != nil {
		t.Skip(err)
	}
	// 480 s a build, not xo dev's 120: http_tls took 71 s at load 47 on
	// the Mac (2026-10-16), and clang's -O pass dominates under load; at
	// 240 s the hourly at fa8b12204 (2026-10-22) timed out in the -O2 -flto
	// link of http_stream_body_h2 (clang 2.5 minutes, then ld still
	// running at 1.5), xo itself fine
	devGoldens(t, "--backend=llvm", "--timeout=480", "internal/llvm/testdata/golden", "internal/arm64/testdata/run")
}

// TestDevGoldensGo is scripts/selfhost-gogoldens.py.
func TestDevGoldensGo(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no Go toolchain")
	}
	devGoldens(t, "--backend=go", "internal/gen/testdata/golden")
}

// TestDevGoldensELF is scripts/elf-goldens.py on a linux/arm64 host: the
// programs linked by compiler/macho with no C toolchain (and, since
// decision 0234, no C runtime), run directly.
func TestDevGoldensELF(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "arm64" {
		t.Skip("runs the linux/arm64 executables on a linux/arm64 host")
	}
	devGoldens(t, "--backend=arm64", "--flags=--target=linux/arm64 --ld=macho", "internal/llvm/testdata/golden", "internal/arm64/testdata/run")
}

// devRun runs another `xo dev` harness that prints `FAIL` lines and a
// count, from the repository root.
func devRun(t *testing.T, args ...string) {
	t.Helper()
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	cmd := testtmp.Guard(exec.Command(xo, append([]string{"dev"}, args...)...))
	cmd.Dir = root
	// xo takes its working directory from $PWD, which go test leaves at
	// the package directory; a wrong one hid gc_config's xo.mod gc lines
	cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "PWD="+root)
	out, err := cmd.CombinedOutput()
	t.Logf("%s", bytes.TrimSpace(out))
	if err != nil {
		t.Fatalf("xo dev %s: %v", strings.Join(args, " "), err)
	}
}

// TestDevErrorcheck is the error check suite (decision 0217).
func TestDevErrorcheck(t *testing.T) {
	devRun(t, "errorcheck", "-j", jobs(), "test/errorcheck")
}

// TestDevCodegen is the codegen pattern suite (decision 0217).
func TestDevCodegen(t *testing.T) {
	backends := "llvm"
	if runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		backends = "arm64,llvm"
	}
	devRun(t, "codegen", "--backend="+backends, "-j", jobs(), "test/codegen")
}
