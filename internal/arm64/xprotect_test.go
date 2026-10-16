package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// xprotectGoldens are the programs one XProtect rule matched while the
// empty list sat at page offset 0x880 of selfhost/macho's __DATA
// (gotchas/xprotect-false-positive-empty-list.md).
var xprotectGoldens = []string{"http_stream", "net_only", "http_conn_fault"}

// TestEmptyListAtPageStart: in a program selfhost/macho links, the
// immortal empty list is the first data symbol, at page offset 0, so
// code that loads it into consecutive registers cannot form the byte
// pattern of XProtect's MACOS.SOMA.AUENB rule (`add x11, x11, #0x880;
// adrp x12; add x12`).
func TestEmptyListAtPageStart(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("selfhost/macho links darwin/arm64 programs")
	}
	if _, err := exec.LookPath("nm"); err != nil {
		t.Skip("no nm")
	}
	dir := filepath.Join("..", "llvm", "testdata", "golden", "http_stream")
	res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendArm64, Flags: []string{"--ld=macho"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("build failed:\n%s", res.Output)
	}
	out, err := exec.Command("nm", res.Binaries[xotool.Host()]).Output()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range strings.Split(string(out), "\n") {
		f := strings.Fields(l)
		if len(f) == 3 && f[2] == "_xo_empty_list" {
			found = true
			a, err := strconv.ParseUint(f[0], 16, 64)
			if err != nil {
				t.Fatal(err)
			}
			if a&0xfff != 0 {
				t.Errorf("_xo_empty_list at page offset %#x, want 0", a&0xfff)
			}
		}
	}
	if !found {
		t.Error("no _xo_empty_list in the program")
	}
}

// TestXProtectRules scans the programs XProtect once flagged, linked by
// selfhost/macho and by cc, with the Mac's XProtect YARA rules, when
// yara and the rules are present (yara is not installed by default:
// brew install yara).
func TestXProtectRules(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("the Mac's rules on its arm64 programs")
	}
	yara, err := exec.LookPath("yara")
	if err != nil {
		t.Skip("no yara")
	}
	var rules []string
	for _, p := range []string{
		"/private/var/protected/xprotect/XProtect.bundle/Contents/Resources/XProtect.yara",
		"/Library/Apple/System/Library/CoreServices/XProtect.bundle/Contents/Resources/XProtect.yara",
	} {
		if _, err := os.Stat(p); err == nil {
			rules = append(rules, p)
		}
	}
	if len(rules) == 0 {
		t.Skip("no XProtect rules")
	}
	for _, name := range xprotectGoldens {
		for _, ld := range []string{"macho", "cc"} {
			dir := filepath.Join("..", "llvm", "testdata", "golden", name)
			res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendArm64, Flags: []string{"--ld=" + ld}})
			if err != nil {
				t.Fatal(err)
			}
			if res.Failed {
				t.Fatalf("%s --ld=%s: build failed:\n%s", name, ld, res.Output)
			}
			for _, r := range rules {
				out, err := exec.Command(yara, "-w", r, res.Binaries[xotool.Host()]).CombinedOutput()
				if err != nil {
					t.Fatalf("yara %s: %v\n%s", r, err, out)
				}
				if s := strings.TrimSpace(string(out)); s != "" {
					t.Errorf("%s --ld=%s matches an XProtect rule (%s):\n%s", name, ld, r, s)
				}
			}
		}
	}
}
