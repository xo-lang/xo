package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDefaultBackendAmd64 builds a program for linux/amd64 and
// windows/amd64 with no --backend (decision 0270: the native backend of
// the targets' architecture) and with --backend=amd64, to the same output
// path: the binaries must be the same. The build read the --backend flag
// rather than the backend it chose for the lowering flags (soft_cpu, the
// Windows link without a C runtime), so a default amd64 build differed
// (gotchas/default-backend-read-from-flag.md). cpu_prims uses cpu.xo's
// carry arithmetic, which soft_cpu lowers.
func TestDefaultBackendAmd64(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	t.Parallel()
	dir := filepath.Join(repoRoot, "internal", "llvm", "testdata", "golden", "cpu_prims")
	chdir(t, dir)
	for _, tg := range []string{"linux/amd64", "windows/amd64"} {
		out := filepath.Join(t.TempDir(), "prog")
		build := func(flags ...string) []byte {
			var stdout, stderr bytes.Buffer
			args := append(append([]string{"build"}, flags...), "--targets="+tg, "-o", out, ".")
			if code := run(t, args, &stdout, &stderr); code != 0 {
				t.Fatalf("xo %v: exit %d\n%s%s", args, code, stdout.String(), stderr.String())
			}
			// -o NAME for windows writes NAME.exe
			got := out
			if strings.HasPrefix(tg, "windows/") {
				got += ".exe"
			}
			b, err := os.ReadFile(got)
			if err != nil {
				t.Fatal(err)
			}
			return b
		}
		def := build()
		amd := build("--backend=amd64")
		if !bytes.Equal(def, amd) {
			t.Errorf("%s: the default backend's binary (%d bytes) differs from --backend=amd64's (%d bytes)", tg, len(def), len(amd))
		}
	}
}

// TestTargetsAllNative builds a program with --targets=all and no
// --backend: the six targets a native backend builds, each with its own
// (0.25.2 failed with "the arm64 backend builds for darwin/arm64 and
// linux/arm64, not linux/amd64"), each binary under its own name in -o's
// directory (the native backend named every target's binary alike, so
// darwin/arm64's and linux/arm64's overwrote each other), a note that
// windows/arm64 needs --backend=go, and exit 0 (owner, 2026-10-22,
// decision 0270 addendum); windows/arm64 named with no --backend is an
// error after the rest are built; and -o NAME for windows/amd64 writes
// NAME.exe (0.25.2 wrote NAME).
func TestTargetsAllNative(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	t.Parallel()
	// the binaries are named after the directory
	dir := filepath.Join(t.TempDir(), "hello")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte("fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)
	out := filepath.Join(dir, "out")
	var stdout, stderr bytes.Buffer
	if code := run(t, []string{"build", "--targets=all", "--json", "-o", out, "."}, &stdout, &stderr); code != 0 {
		t.Fatalf("xo build --targets=all: exit %d\n%s%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "windows/arm64 needs --backend=go") {
		t.Errorf("stderr has no note that windows/arm64 needs --backend=go:\n%s", stderr.String())
	}
	var rep struct {
		Binaries map[string]string `json:"binaries"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("--json: %v\n%s", err, stdout.String())
	}
	// each target's file and its format's magic
	want := map[string]struct{ file, magic string }{
		"darwin/arm64":  {"hello-darwin-arm64", "\xcf\xfa\xed\xfe\x0c\x00\x00\x01"},
		"darwin/amd64":  {"hello-darwin-amd64", "\xcf\xfa\xed\xfe\x07\x00\x00\x01"},
		"linux/arm64":   {"hello-linux-arm64", "\x7fELF"},
		"linux/amd64":   {"hello-linux-amd64", "\x7fELF"},
		"windows/amd64": {"hello-windows-amd64.exe", "MZ"},
		"wasip1/wasm":   {"hello-wasip1-wasm.wasm", "\x00asm"},
	}
	if len(rep.Binaries) != len(want) {
		t.Errorf("binaries %v, want %d targets", rep.Binaries, len(want))
	}
	for tg, w := range want {
		p := filepath.Join(out, w.file)
		if rep.Binaries[tg] != p {
			t.Errorf("%s: reported %q, want %q", tg, rep.Binaries[tg], p)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("%s: %v", tg, err)
			continue
		}
		if !bytes.HasPrefix(b, []byte(w.magic)) {
			t.Errorf("%s: %s starts % x, want % x", tg, p, b[:min(len(b), 8)], w.magic)
		}
	}
	// windows/arm64 named: the others built, then an error
	stdout.Reset()
	stderr.Reset()
	if code := run(t, []string{"build", "--targets=linux/arm64,windows/arm64", "-o", filepath.Join(dir, "out2"), "."}, &stdout, &stderr); code != 1 {
		t.Errorf("xo build --targets=linux/arm64,windows/arm64: exit %d, want 1\n%s%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "no native backend builds windows/arm64: build it with --backend=go") {
		t.Errorf("windows/arm64 named: stderr does not name --backend=go:\n%s", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "out2", "hello-linux-arm64")); err != nil {
		t.Errorf("windows/arm64 named: linux/arm64 not built: %v", err)
	}
	// -o NAME for windows: NAME.exe
	stdout.Reset()
	stderr.Reset()
	exe := filepath.Join(dir, "app")
	if code := run(t, []string{"build", "--targets=windows/amd64", "-o", exe, "."}, &stdout, &stderr); code != 0 {
		t.Fatalf("xo build --targets=windows/amd64 -o app: exit %d\n%s%s", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(exe + ".exe"); err != nil {
		t.Errorf("-o app for windows/amd64: %v (want app.exe)", err)
	}
	if _, err := os.Stat(exe); err == nil {
		t.Errorf("-o app for windows/amd64 wrote app too")
	}
}

// TestTargetsAllSkipsUnsupported: --targets=all on a program a target
// cannot support (a network program and WASI preview1, which has no
// sockets) builds the other targets, notes the skipped one with its
// reason, and exits 0 (owner, 2026-10-23, field report 0.26.0: it exited
// 1); --targets=wasip1/wasm named alone still fails, with the network
// reason (it picked the arm64 backend and named that instead).
func TestTargetsAllSkipsUnsupported(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	t.Parallel()
	src, err := os.ReadFile(filepath.Join("..", "..", "internal", "llvm", "testdata", "golden", "conn_iface", "main.xo"))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "netprog")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{"xo.mod": []byte("module netprog\n"), "main.xo": src} {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, dir)
	var stdout, stderr bytes.Buffer
	if code := run(t, []string{"build", "--targets=all", "--json", "-o", filepath.Join(dir, "out"), "."}, &stdout, &stderr); code != 0 {
		t.Fatalf("xo build --targets=all: exit %d, want 0\n%s%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "xo build: skipped wasip1/wasm: ") || !strings.Contains(stderr.String(), "no sockets") {
		t.Errorf("stderr has no note skipping wasip1/wasm for its sockets:\n%s", stderr.String())
	}
	var rep struct {
		Binaries map[string]string `json:"binaries"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("--json: %v\n%s", err, stdout.String())
	}
	for _, tg := range []string{"darwin/arm64", "darwin/amd64", "linux/arm64", "linux/amd64", "windows/amd64"} {
		if _, err := os.Stat(rep.Binaries[tg]); err != nil {
			t.Errorf("%s: no binary (%v): %v", tg, err, rep.Binaries)
		}
	}
	if _, ok := rep.Binaries["wasip1/wasm"]; ok {
		t.Errorf("wasip1/wasm reported built: %v", rep.Binaries)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(t, []string{"build", "--targets=wasip1/wasm", "-o", filepath.Join(dir, "outw"), "."}, &stdout, &stderr); code == 0 {
		t.Errorf("xo build --targets=wasip1/wasm: exit 0, want a failure\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "does not support the network") || strings.Contains(stderr.String(), "the wasm backend: the wasm backend") {
		t.Errorf("--targets=wasip1/wasm: the error does not name the network once:\n%s", stderr.String())
	}
}

// TestTargetsWasiAlone: --targets=wasip1/wasm alone, and the spec's
// spelling wasi/wasm32 (spec 12.1), build a WebAssembly module with the
// wasm backend (field report 0.26.0, b06: 0.26.0 picked the host's
// backend and failed; fixed with caddf03ac).
func TestTargetsWasiAlone(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "hi")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte("fn main(os: Os) uses (stdio) {\n  os.stdio.println(\"hi\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)
	for _, tg := range []string{"wasip1/wasm", "wasi/wasm32"} {
		out := filepath.Join(dir, strings.ReplaceAll(tg, "/", "-")+".wasm")
		var stdout, stderr bytes.Buffer
		if code := run(t, []string{"build", "--targets=" + tg, "-o", out, "."}, &stdout, &stderr); code != 0 {
			t.Errorf("--targets=%s: exit %d\n%s%s", tg, code, stdout.String(), stderr.String())
			continue
		}
		b, err := os.ReadFile(out)
		if err != nil || !bytes.HasPrefix(b, []byte("\x00asm")) {
			t.Errorf("--targets=%s: %s is not a WebAssembly module (%v)", tg, out, err)
		}
	}
}
