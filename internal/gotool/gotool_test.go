package gotool

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeGo writes a go command that prints version for `go env GOVERSION`
// and counts its runs in a file next to it.
func fakeGo(t *testing.T, version string) (path, calls string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake go command is a shell script")
	}
	dir := t.TempDir()
	path = filepath.Join(dir, "go")
	calls = filepath.Join(dir, "calls")
	script := "#!/bin/sh\necho run >> '" + calls + "'\nif [ \"$1\" = env ] && [ \"$2\" = GOVERSION ]; then echo '" + version + "'; exit 0; fi\nexit 2\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, calls
}

func TestParse(t *testing.T) {
	for _, c := range []struct {
		in       string
		maj, min int
		ok       bool
	}{
		{"go1.26.1", 1, 26, true},
		{"go1.27rc1", 1, 27, true},
		{"go1.24", 1, 24, true},
		{"1.24", 1, 24, true},
		{"devel go1.27-abcdef Tue Oct 6 2026", 1, 27, true},
		{"go2.0.0", 2, 0, true},
		{"", 0, 0, false},
		{"gox", 0, 0, false},
		{"go1.", 0, 0, false},
	} {
		maj, min, ok := Parse(c.in)
		if maj != c.maj || min != c.min || ok != c.ok {
			t.Errorf("Parse(%q) = %d, %d, %v; want %d, %d, %v", c.in, maj, min, ok, c.maj, c.min, c.ok)
		}
	}
}

func TestLine(t *testing.T) {
	for in, want := range map[string]string{
		"go1.25.3":  "1.25",
		"go1.26.1":  TestedGo,
		"go1.27rc1": TestedGo, // newer than tested: stay on the tested line
		"go1.99.0":  TestedGo,
		"go1.22.5":  "1.22", // never above the installed Go
		"weird":     TestedGo,
	} {
		if got := Line(in); got != want {
			t.Errorf("Line(%q) = %q, want %q", in, got, want)
		}
	}
	if Less(TestedGo, MinGo) {
		t.Errorf("TestedGo %s is older than MinGo %s", TestedGo, MinGo)
	}
}

// TestVersionOncePerPath reads the version of a fake go command: the
// command runs once per process however often it is asked, and a go
// command older than MinGo is a *TooOldError.
func TestVersionOncePerPath(t *testing.T) {
	path, calls := fakeGo(t, "go1.22.5")
	t.Setenv("XO_GO", path)
	t.Setenv("XO_GO_LINE", "")
	for range 3 {
		v, err := Version()
		if err != nil || v != "go1.22.5" {
			t.Fatalf("Version() = %q, %v", v, err)
		}
	}
	if got := GoLine(); got != "1.22" {
		t.Errorf("GoLine() = %q, want 1.22", got)
	}
	data, _ := os.ReadFile(calls)
	if n := strings.Count(string(data), "run"); n != 1 {
		t.Errorf("go ran %d times, want 1", n)
	}
	var old *TooOldError
	if err := Check(); !errors.As(err, &old) || old.Version != "go1.22.5" || !strings.Contains(err.Error(), "Go "+MinGo+" or newer") {
		t.Errorf("Check() = %v, want a TooOldError for go1.22.5", err)
	}

	path, _ = fakeGo(t, "go1.27rc1")
	t.Setenv("XO_GO", path)
	if err := Check(); err != nil {
		t.Errorf("Check() with go1.27rc1 = %v", err)
	}
	if got := GoLine(); got != TestedGo {
		t.Errorf("GoLine() with go1.27rc1 = %q, want %q", got, TestedGo)
	}
	t.Setenv("XO_GO_LINE", MinGo)
	if got := GoLine(); got != MinGo {
		t.Errorf("GoLine() with XO_GO_LINE = %q, want %q", got, MinGo)
	}
}

// TestVersionUnreadable: a go command that fails is not reported as too
// old (the build reports its own failure), and the line is TestedGo.
func TestVersionUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake go command is a shell script")
	}
	path := filepath.Join(t.TempDir(), "go")
	os.WriteFile(path, []byte("#!/bin/sh\nexit 3\n"), 0o755)
	t.Setenv("XO_GO", path)
	t.Setenv("XO_GO_LINE", "")
	if _, err := Version(); err == nil {
		t.Error("Version() of a failing go command succeeded")
	}
	if err := Check(); err != nil {
		t.Errorf("Check() = %v, want nil", err)
	}
	if got := GoLine(); got != TestedGo {
		t.Errorf("GoLine() = %q, want %q", got, TestedGo)
	}
}

func TestCacheDir(t *testing.T) {
	t.Setenv("XO_GOCACHE", "/x/y")
	if CacheDir() != "/x/y" || CacheEnv() != "GOCACHE=/x/y" {
		t.Errorf("CacheDir() = %q, CacheEnv() = %q", CacheDir(), CacheEnv())
	}
	t.Setenv("XO_GOCACHE", "")
	if !strings.HasSuffix(CacheDir(), filepath.Join("xo", "gocache")) {
		t.Errorf("default CacheDir() = %q", CacheDir())
	}
}
