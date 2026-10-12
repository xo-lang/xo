package selfdriver

import (
	"strings"
	"testing"
)

// TestWants pins which command lines run through xo1 (decision 0178
// switch point A).
func TestWants(t *testing.T) {
	t.Setenv("XO_SELF_DRIVER", "")
	for _, c := range []struct {
		cmd, args string
		want      bool
	}{
		{"build", "--backend=arm64 -o x src", true},
		{"build", "--backend llvm src", true},
		{"run", "--no-gc src -- --backend=go", true},
		{"test", "--no-gc --backend=llvm .", true},
		{"build", "src", false},
		{"build", "--backend=go src", false},
		{"build", "--no-gc=false src", false},
		{"run", "src -- --backend=arm64", false},
		{"build", "--ci --backend=arm64 src", false},
		{"test", "--real --no-gc src", false},
		{"check", "--no-gc src", false},
		{"build", "-o --backend=arm64 src", false},
	} {
		if got := wantsOn("darwin", "arm64", c.cmd, strings.Fields(c.args)); got != c.want {
			t.Errorf("darwin/arm64 %s %s: got %v, want %v", c.cmd, c.args, got, c.want)
		}
	}
	// an x86-64 host: xo1 is built by the LLVM backend and runs the LLVM
	// builds (--no-gc defaults to LLVM there); arm64 cross builds stay
	// with the Go driver
	for _, c := range []struct {
		cmd, args string
		want      bool
	}{
		{"build", "--backend=llvm -o x src", true},
		{"run", "--no-gc src", true},
		{"test", "--no-gc --backend=llvm .", true},
		{"build", "--backend=arm64 src", false},
		{"build", "--no-gc --backend=arm64 src", false},
		{"build", "src", false},
		{"build", "--ci --backend=llvm src", false},
	} {
		for _, goos := range []string{"darwin", "linux"} {
			if got := wantsOn(goos, "amd64", c.cmd, strings.Fields(c.args)); got != c.want {
				t.Errorf("%s/amd64 %s %s: got %v, want %v", goos, c.cmd, c.args, got, c.want)
			}
		}
	}
	for _, h := range [][2]string{{"windows", "amd64"}, {"linux", "riscv64"}} {
		if wantsOn(h[0], h[1], "build", []string{"--backend=llvm", "src"}) {
			t.Errorf("%s/%s runs xo1", h[0], h[1])
		}
	}
	t.Setenv("XO_SELF_DRIVER", "0")
	if Wants("build", []string{"--backend=arm64", "src"}) || wantsOn("linux", "amd64", "build", []string{"--backend=llvm", "src"}) {
		t.Error("XO_SELF_DRIVER=0 still runs xo1")
	}
}

// TestXo1Backend: xo1 is built by the arm64 backend on arm64 hosts and
// by the split LLVM build elsewhere.
func TestXo1Backend(t *testing.T) {
	t.Setenv("XO_LLVM_SPLIT", "")
	if b, s := xo1Backend("arm64"); b != "arm64" || s != nil {
		t.Errorf("arm64 host: %s %v", b, s)
	}
	b, s := xo1Backend("amd64")
	if b != "llvm" || s == nil || s.Modules != 12 || s.LTO != "thin" {
		t.Errorf("amd64 host: %s %+v", b, s)
	}
	t.Setenv("XO_LLVM_SPLIT", "4")
	if _, s := xo1Backend("amd64"); s == nil || s.Modules != 4 {
		t.Errorf("amd64 host, XO_LLVM_SPLIT=4: %+v", s)
	}
}

// TestSources writes the embedded sources once and finds them again.
func TestSources(t *testing.T) {
	t.Setenv("XO1_CACHE", t.TempDir())
	a, err := Sources()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Sources()
	if err != nil || a != b {
		t.Fatalf("second Sources: %s, %v (first %s)", b, err, a)
	}
	for _, p := range []string{"selfhost/xo.mod", "selfhost/xo/main.xo", "std/prelude.xo", "std/llrt/xo_rt.c", "third_party/mbedtls/mbedtls.tgz"} {
		if !exists(a + "/" + p) {
			t.Errorf("%s missing", p)
		}
	}
}
