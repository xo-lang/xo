package repotest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestReleaseVerifyWindowsSqlite (task #244, the owner's 0.51.1 Windows
// evaluation, bug 24): scripts/release-verify.sh fails a windows archive
// with no prebuilt std/sqlite object. 0.51.1's had none (release.sh only
// warned when it could not build one), so a windows/amd64 program using
// std/sqlite asked for clang, against the release notes.
// And an archive whose prebuilt object has no build record fails (the
// SQLite version, the compiler; owner 2026-11-20, xw decision 238).
func TestReleaseVerifyWindowsSqlite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("release-verify.sh checks archives on the release host (darwin, linux)")
	}
	base := []string{"bin/xo.exe", "bin/xo-gobind.exe", "lib/xo/std/prelude.xo", "lib/xo/go.mod", "lib/xo/cmd/xo-gobind/main.go", "LICENSE", "THIRD_PARTY_NOTICES"}
	sq := []string{"lib/xo/sqlite/windows-amd64/sqlite.obj", "lib/xo/sqlite/windows-amd64/sqlite.obj.key", "lib/xo/sqlite/windows-amd64/sqlite.obj.sha256"}
	verify := func(files []string, env ...string) (string, error) {
		dir := t.TempDir()
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		for _, f := range files {
			body := "x"
			if strings.HasSuffix(f, ".build") {
				body = "object: sqlite.obj (windows-amd64)\nsqlite: 3.54.1\ncompiler: clang version 23.1.3\n"
			}
			if err := tw.WriteHeader(&tar.Header{Name: "xo-0.0.0-windows-amd64/" + f, Mode: 0o644, Size: int64(len(body))}); err != nil {
				t.Fatal(err)
			}
			tw.Write([]byte(body))
		}
		tw.Close()
		gz.Close()
		archive := filepath.Join(dir, "xo-0.0.0-windows-amd64.tar.gz")
		if err := os.WriteFile(archive, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("bash", filepath.Join(root, "scripts", "release-verify.sh"), archive, filepath.Join(dir, "out"))
		cmd.Env = append(append(os.Environ(), "XO_VERIFY_UNSIGNED=1"), env...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	out, err := verify(base)
	if err == nil || !strings.Contains(out, "FAIL layout-lib/xo/sqlite/windows-amd64/sqlite.obj\n") {
		t.Errorf("an archive with no prebuilt sqlite object passed (err %v):\n%s", err, out)
	}
	// the object without its build record (xw decision 238)
	out, err = verify(append(append([]string{}, base...), sq...))
	if err == nil || !strings.Contains(out, "FAIL layout-lib/xo/sqlite/windows-amd64/sqlite.obj.build\n") {
		t.Errorf("an archive whose prebuilt sqlite object has no build record passed (err %v):\n%s", err, out)
	}
	sq = append(sq, "lib/xo/sqlite/windows-amd64/sqlite.obj.build")
	out, err = verify(append(append([]string{}, base...), sq...))
	if err != nil || !strings.Contains(out, "PASS layout-lib/xo/sqlite/windows-amd64/sqlite.obj\n") || strings.Contains(out, "FAIL ") {
		t.Errorf("an archive with the prebuilt sqlite object failed (err %v):\n%s", err, out)
	}
	out, err = verify(base, "XO_VERIFY_NO_SQLITE=1")
	if err != nil || strings.Contains(out, "FAIL ") {
		t.Errorf("XO_VERIFY_NO_SQLITE=1 did not allow an archive without it (err %v):\n%s", err, out)
	}
}
