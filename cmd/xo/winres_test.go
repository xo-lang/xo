package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf16"
)

// utf16le is s as the UTF-16LE bytes a VS_VERSIONINFO string holds.
func utf16le(s string) []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}

// TestBuildWindowsVersionInfo: --versioninfo, --icon and --manifest
// (decision 0560) put goversioninfo's resources into a windows/amd64
// build. An edit to the JSON builds again (the run cache keeps the file
// among its inputs), a bad JSON is XO1517, and the flags with no windows
// target are a usage error. On Windows, PowerShell reads the version
// information back.
func TestBuildWindowsVersionInfo(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds programs")
	}
	ico, err := filepath.Abs("../../compiler/winres/testdata/resource/icon.ico")
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	setenv(t, "XO_BUILD_DIR", filepath.Join(tmp, "build"))
	setenv(t, "XO_BIN_CACHE", filepath.Join(tmp, "bin"))
	setenv(t, "XO_BACKEND_CACHE_DIR", filepath.Join(tmp, "backend"))
	setenv(t, "XO_WHOLE_BUILD", "on")
	dir := filepath.Join(tmp, "prog")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("xo.mod", "module winres\n")
	write("main.xo", "fn main(os: Os) {\n  os.stdio.println(\"res\")\n}\n")
	vi := func(company string) string {
		return `{"FixedFileInfo": {"FileVersion": {"Major": 1, "Minor": 2, "Patch": 3, "Build": 4}},
 "StringFileInfo": {"CompanyName": "` + company + `", "ProductName": "Winres Test"},
 "VarFileInfo": {"Translation": {"LangID": "0409", "CharsetID": "04B0"}}}`
	}
	write("vi.json", vi("Acme Test Company"))
	write("app.manifest", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0"><description>winres test manifest</description></assembly>
`)
	chdir(t, dir)
	exe := filepath.Join(dir, "prog.exe")
	build := func(what string, wantCode int, flags ...string) string {
		t.Helper()
		var so, se bytes.Buffer
		args := append([]string{"build", "--backend=amd64"}, flags...)
		if code := run(t, append(args, "-o", filepath.Join(dir, "prog"), "."), &so, &se); code != wantCode {
			t.Fatalf("%s: exit %d, want %d\n%s%s", what, code, wantCode, so.String(), se.String())
		}
		return so.String() + se.String()
	}
	has := func(what string, b []byte, s string, want bool) {
		t.Helper()
		if bytes.Contains(b, utf16le(s)) != want {
			t.Fatalf("%s: %q in the .exe is %v, want %v", what, s, !want, want)
		}
	}
	res := []string{"--targets=windows/amd64", "--versioninfo=vi.json", "--icon=" + ico, "--manifest=app.manifest"}
	build("first", 0, res...)
	b, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	has("first", b, "Acme Test Company", true)
	has("first", b, "Winres Test", true)
	if !bytes.Contains(b, []byte("winres test manifest")) {
		t.Fatal("first: the manifest is not in the .exe")
	}
	icon, err := os.ReadFile(ico)
	if err != nil {
		t.Fatal(err)
	}
	// the first image (at the offset its directory entry gives) is an
	// RT_ICON resource
	off := int(icon[18]) | int(icon[19])<<8 | int(icon[20])<<16 | int(icon[21])<<24
	if !bytes.Contains(b, icon[off:off+64]) {
		t.Fatal("first: the icon's image is not in the .exe")
	}

	if runtime.GOOS == "windows" {
		// Windows itself reads the resources: the shell's VersionInfo
		out, err := exec.Command("powershell", "-NoProfile", "-Command",
			"$v = (Get-Item '"+exe+"').VersionInfo; $v.CompanyName + '|' + $v.ProductName + '|' + $v.FileVersionRaw").CombinedOutput()
		if err != nil {
			t.Fatalf("powershell: %v\n%s", err, out)
		}
		if got := strings.TrimSpace(string(out)); got != "Acme Test Company|Winres Test|1.2.3.4" {
			t.Fatalf("Windows reads %q, want Acme Test Company|Winres Test|1.2.3.4", got)
		}
	}

	write("vi.json", vi("Other Company"))
	build("edited JSON", 0, res...)
	if b, err = os.ReadFile(exe); err != nil {
		t.Fatal(err)
	}
	has("edited JSON", b, "Other Company", true)
	has("edited JSON", b, "Acme Test Company", false)

	write("vi.json", `{"FixedFileInfo": `)
	if out := build("bad JSON", 1, res...); !strings.Contains(out, "XO1517") {
		t.Fatalf("bad JSON: no XO1517 in\n%s", out)
	}
	build("no windows target", 2, "--targets=linux/arm64", "--versioninfo=vi.json")
}
