// Package devgoldens holds the thin Go tests over `xo dev` (decision
// 0209): each runs one harness of the xo the tests use (internal/xotool)
// and fails with the programs that failed. The harness logic (building,
// running, comparing byte for byte, the pass cache, the workers) is in
// Xo, selfhost/cli/dev; these tests only keep `go test` tiers and the EC2
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
	"time"

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
	full = append(full, args...)
	cmd := exec.Command(xo, full...)
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
		if o.Status == "fail" {
			fails = append(fails, o.Dir+": "+o.Why)
		}
	}
	if sum.Programs == 0 {
		t.Fatalf("xo %s: no summary (%v)\n%s%s", strings.Join(full, " "), err, out, stderr.String())
	}
	t.Logf("%d programs: %d pass (%d cached), %d fail, %d ms", sum.Programs, sum.Pass, sum.Cached, sum.Fail, sum.WallMS)
	if len(fails) > 0 {
		t.Fatalf("%d failing:\n%s", len(fails), strings.Join(fails, "\n"))
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
	devGoldens(t, "--backend=arm64", "internal/llvm/testdata/golden", "internal/arm64/testdata/run")
}

// TestDevGoldensLLVM is scripts/selfhost-goldens.py with XO1_BACKEND=llvm.
func TestDevGoldensLLVM(t *testing.T) {
	if _, err := xotool.Clang(); err != nil {
		t.Skip(err)
	}
	devGoldens(t, "--backend=llvm", "internal/llvm/testdata/golden", "internal/arm64/testdata/run")
}

// TestDevGoldensGo is scripts/selfhost-gogoldens.py.
func TestDevGoldensGo(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no Go toolchain")
	}
	devGoldens(t, "--backend=go", "internal/gen/testdata/golden")
}

// TestDevGoldensELF is scripts/elf-goldens.py on a linux/arm64 host: the
// programs linked by selfhost/macho with no C toolchain, run directly.
// --ld=macho links the prebuilt runtime of a pack (decision 0157), which
// a checkout does not have (only a release's lib/xo/rtpack), so the test
// builds the linux/arm64 pack of the checkout's sources first (elfPack)
// and points XO_RTPACK at it.
func TestDevGoldensELF(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "arm64" {
		t.Skip("runs the linux/arm64 executables on a linux/arm64 host")
	}
	t.Setenv("XO_RTPACK", elfPack(t))
	devGoldens(t, "--backend=arm64", "--flags=--target=linux/arm64 --ld=macho", "internal/llvm/testdata/golden", "internal/arm64/testdata/run")
}

// elfPack is the root of a linux/arm64 runtime pack of the checkout's
// runtime sources: <user cache>/xo/devgoldens/rtpack/<pack key>, built
// with `xo dev rtpack` and the host's C compiler when it is not there
// yet. It skips t when the host has no C compiler or archiver to build
// one (the message of `xo dev rtpack`), and fails it on any other error.
func elfPack(t *testing.T) string {
	t.Helper()
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xoDev := func(args ...string) ([]byte, error) {
		cmd := exec.Command(xo, append([]string{"dev", "rtpack"}, args...)...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "PWD="+root, "XO_RTPACK=off")
		return cmd.CombinedOutput()
	}
	out, err := xoDev("--key", "--target=linux/arm64")
	key := strings.TrimSpace(string(out))
	if err != nil || key == "" || strings.ContainsAny(key, " /\n") {
		t.Fatalf("xo dev rtpack --key: %v\n%s", err, out)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(cache, "xo", "devgoldens", "rtpack")
	pack := filepath.Join(base, key)
	if _, err := os.Stat(filepath.Join(pack, "linux-arm64", "rtpack.json")); err == nil {
		return pack
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	tmp, err := os.MkdirTemp(base, "build-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	start := time.Now()
	if out, err := xoDev("--target=linux/arm64", "-o", tmp); err != nil {
		s := string(out)
		if strings.Contains(s, "needs a C compiler") || strings.Contains(s, "needs an archiver") {
			t.Skipf("no linux/arm64 runtime pack for --ld=macho, and this host cannot build one: %s", strings.TrimSpace(s))
		}
		t.Fatalf("xo dev rtpack --target=linux/arm64: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(tmp, "linux-arm64", "rtpack.json")); err != nil {
		t.Fatalf("xo dev rtpack wrote no linux-arm64/rtpack.json: %v", err)
	}
	// another process may have built the same key meanwhile; either is
	// the same pack
	if err := os.Rename(tmp, pack); err != nil {
		if _, serr := os.Stat(filepath.Join(pack, "linux-arm64", "rtpack.json")); serr != nil {
			t.Fatalf("installing the pack: %v", err)
		}
	}
	t.Logf("built the linux/arm64 runtime pack %s in %s", pack, time.Since(start).Round(time.Millisecond))
	return pack
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
	cmd := exec.Command(xo, append([]string{"dev"}, args...)...)
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
