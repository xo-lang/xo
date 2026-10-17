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
// empty list sat at page offset 0x880 of compiler/macho's __DATA
// (gotchas/xprotect-false-positive-empty-list.md).
var xprotectGoldens = []string{"http_stream", "net_only", "http_conn_fault"}

// TestEmptyListAtPageStart: in a program compiler/macho links, the
// immortal empty list is the first data symbol, at page offset 0, so
// code that loads it into consecutive registers cannot form the byte
// pattern of XProtect's MACOS.SOMA.AUENB rule (`add x11, x11, #0x880;
// adrp x12; add x12`).
func TestEmptyListAtPageStart(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("compiler/macho links darwin/arm64 programs")
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
// compiler/macho and by cc, with the Mac's XProtect YARA rules: with
// yara when it is on PATH (brew install yara), else in the Linux
// container of scripts/docker-linux.sh (its image carries yara) when
// Docker runs; XO_XPROTECT_DOCKER=off turns the container off.
func TestXProtectRules(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("the Mac's rules on its arm64 programs")
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
	scan := yaraScanner(t)
	work := t.TempDir()
	var bins []string
	for _, name := range xprotectGoldens {
		for _, ld := range []string{"macho", "cc"} {
			dir := filepath.Join("..", "llvm", "testdata", "golden", name)
			out := filepath.Join(work, "bin", name+"-"+ld)
			res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendArm64, Out: out, Flags: []string{"--ld=" + ld}})
			if err != nil {
				t.Fatal(err)
			}
			if res.Failed {
				t.Fatalf("%s --ld=%s: build failed:\n%s", name, ld, res.Output)
			}
			bins = append(bins, out)
		}
	}
	for i, r := range rules {
		dst := filepath.Join(work, "rules", strconv.Itoa(i)+".yara")
		data, err := os.ReadFile(r)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if s := scan(work, dst, bins); s != "" {
			t.Errorf("a program matches an XProtect rule (%s):\n%s", r, s)
		}
	}
}

// yaraScanner returns a function that runs yara with the rules file on
// each binary (all under work) and returns its matches, or skips the
// test when neither yara nor Docker is available.
func yaraScanner(t *testing.T) func(work, rules string, bins []string) string {
	t.Helper()
	if yara, err := exec.LookPath("yara"); err == nil {
		return func(work, rules string, bins []string) string {
			var b strings.Builder
			for _, f := range bins {
				out, err := exec.Command(yara, "-w", rules, f).CombinedOutput()
				if err != nil {
					t.Fatalf("yara: %v\n%s", err, out)
				}
				b.Write(out)
			}
			return strings.TrimSpace(b.String())
		}
	}
	if os.Getenv("XO_XPROTECT_DOCKER") == "off" {
		t.Skip("no yara, and XO_XPROTECT_DOCKER=off")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("no yara and no docker")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("no yara, and Docker is not running")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "docker-linux.sh"))
	if err != nil {
		t.Fatal(err)
	}
	return func(work, rules string, bins []string) string {
		rel := func(p string) string {
			r, err := filepath.Rel(work, p)
			if err != nil {
				t.Fatal(err)
			}
			return "/work/" + filepath.ToSlash(r)
		}
		cmd := ""
		for _, f := range bins {
			cmd += "yara -w " + rel(rules) + " " + rel(f) + " || exit 1; "
		}
		out, err := exec.Command(script, "arm64", work, cmd).CombinedOutput()
		if err != nil {
			t.Fatalf("yara in docker: %v\n%s", err, out)
		}
		var keep []string
		for _, l := range strings.Split(string(out), "\n") {
			// a match is "RULE PATH" (rule names have several prefixes);
			// docker build's progress lines (a new image) are not
			if strings.Contains(l, " /work/bin/") {
				keep = append(keep, l)
			}
		}
		return strings.Join(keep, "\n")
	}
}
