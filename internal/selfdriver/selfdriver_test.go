package selfdriver

import (
	"runtime"
	"strings"
	"testing"
)

// TestWants pins which command lines run through xo1 (decision 0178
// switch point A).
func TestWants(t *testing.T) {
	if runtime.GOARCH != "arm64" || (runtime.GOOS != "darwin" && runtime.GOOS != "linux") {
		t.Skip("the native backends run on arm64 hosts")
	}
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
		if got := Wants(c.cmd, strings.Fields(c.args)); got != c.want {
			t.Errorf("%s %s: got %v, want %v", c.cmd, c.args, got, c.want)
		}
	}
	t.Setenv("XO_SELF_DRIVER", "0")
	if Wants("build", []string{"--backend=arm64", "src"}) {
		t.Error("XO_SELF_DRIVER=0 still runs xo1")
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
