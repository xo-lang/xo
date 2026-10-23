package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// crossDir writes a module named name with files into a new temporary
// directory and makes it the working directory.
func crossDir(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files["xo.mod"] = "module " + name + "\n"
	for f, s := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, dir)
	return dir
}

// TestCrossUseCFailsLoudly: a cross build of a `use c` program links
// with cc, which xo does not do for another target; the build fails
// with a message naming the target (Intel field report b04 on 0.26.0:
// exit 0, "binaries": {}, and only an assembly file written), and
// --targets=all skips that target with a note.
func TestCrossUseCFailsLoudly(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	t.Parallel()
	dir := crossDir(t, "usec", map[string]string{
		"addc.h": "int addc_add(int a, int b);\n",
		"addc.c": "#include \"addc.h\"\nint addc_add(int a, int b) { return a + b; }\n",
		"main.xo": "use c \"addc.h\" link \"addc.c\"\n\nfn main(os: Os) {\n  os.stdio.println(\"${addc.addc_add(40, 2)}\")\n}\n",
	})
	tg := "linux/amd64"
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		tg = "linux/arm64"
	}
	out := filepath.Join(dir, "out")
	var stdout, stderr bytes.Buffer
	if code := run(t, []string{"build", "--json", "--targets=" + tg, "-o", out, "."}, &stdout, &stderr); code != 1 {
		t.Fatalf("xo build --targets=%s: exit %d, want 1\n%s%s", tg, code, stdout.String(), stderr.String())
	}
	if want := tg + " links with a C compiler"; !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr does not say %q:\n%s", want, stderr.String())
	}
	var rep struct {
		Ok bool `json:"ok"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil || rep.Ok {
		t.Errorf("--json: want ok false, got %v\n%s", err, stdout.String())
	}
}

// TestDarwinFromAnyHost: a darwin program with no C source links with
// Xo's Mach-O linker on any host (Windows field report W5 on 0.26.0:
// from Windows, darwin targets were reported built and only their
// assembly written). On a darwin host this is the host's own link.
func TestDarwinFromAnyHost(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	t.Parallel()
	dir := crossDir(t, "hello", map[string]string{
		"main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
	})
	out := filepath.Join(dir, "out")
	var stdout, stderr bytes.Buffer
	if code := run(t, []string{"build", "--json", "--targets=darwin/arm64,darwin/amd64", "-o", out, "."}, &stdout, &stderr); code != 0 {
		t.Fatalf("xo build: exit %d\n%s%s", code, stdout.String(), stderr.String())
	}
	var rep struct {
		Binaries map[string]string `json:"binaries"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("--json: %v\n%s", err, stdout.String())
	}
	// MH_MAGIC_64 and the CPU type: arm64 0x0100000c, x86_64 0x01000007
	for tg, cpu := range map[string]byte{"darwin/arm64": 0x0c, "darwin/amd64": 0x07} {
		p := rep.Binaries[tg]
		b, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("%s: reported %q: %v", tg, p, err)
			continue
		}
		if len(b) < 8 || !bytes.Equal(b[:4], []byte{0xcf, 0xfa, 0xed, 0xfe}) || b[4] != cpu || b[7] != 0x01 {
			t.Errorf("%s: %s starts % x, want a 64 bit Mach-O of cpu %#x", tg, p, b[:min(len(b), 8)], cpu)
		}
	}
}
