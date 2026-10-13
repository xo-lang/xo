package selfdriver

import (
	"bytes"
	"os"
	"path/filepath"
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
	t.Setenv("XO_LLVM_LTO", "")
	if b, s := xo1Backend("arm64"); b != "arm64" || s != nil {
		t.Errorf("arm64 host: %s %v", b, s)
	}
	b, s := xo1Backend("amd64")
	if b != "llvm" || s == nil || s.Modules != 12 || s.LTO != "none" {
		t.Errorf("amd64 host: %s %+v", b, s)
	}
	t.Setenv("XO_LLVM_SPLIT", "4")
	t.Setenv("XO_LLVM_LTO", "thin")
	if _, s := xo1Backend("amd64"); s == nil || s.Modules != 4 || s.LTO != "thin" {
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

// TestBinaryKey: the built xo1 is keyed on its own inputs, so a change to
// a std module xo1 does not use (or to the C runtime, or third_party)
// reuses the cached binary beside a new source tree, and a change under
// selfhost/, to a std module xo1 uses, or of the xo version builds a new
// one.
func TestBinaryKey(t *testing.T) {
	files, err := embedded()
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]bool{}
	var stdIn []string
	for _, n := range BinaryInputs(files) {
		inputs[n] = true
		if strings.HasPrefix(n, "std/") {
			stdIn = append(stdIn, n)
		}
	}
	t.Logf("%d inputs, of std: %s", len(inputs), strings.Join(stdIn, " "))
	for _, n := range []string{"selfhost/xo.mod", "selfhost/xo/main.xo", "selfhost/arm64/isel.xo", "std/prelude.xo", "std/native/prelude.xo", "std/native/rt.xo", "std/crypto.xo", "std/native/crypto.xo", "std/native/rtcrypto.xo", "std/http.xo", "std/native/http.xo", "std/native/llrt/text.xo"} {
		if !inputs[n] {
			t.Errorf("%s is not an input of xo1", n)
		}
	}
	for _, n := range []string{"std/llrt/xo_rt.c", "std/llrt/xo_win.c", "std/toml.xo", "std/native/toml.xo", "third_party/mbedtls/mbedtls.tgz"} {
		if _, ok := files[n]; !ok {
			t.Fatalf("%s is not embedded", n)
		}
		if inputs[n] {
			t.Errorf("%s is an input of xo1", n)
		}
	}

	base := t.TempDir()
	t.Setenv("XO1_CACHE", base)
	t.Setenv("XO1", "")
	edit := func(name string) map[string][]byte {
		m := map[string][]byte{}
		for k, v := range files {
			m[k] = v
		}
		m[name] = append(append([]byte{}, files[name]...), "\n// edited\n"...)
		return m
	}
	// a fake xo1 in the binary's place: binary finds it without building
	key := BinaryKey(files)
	fake := filepath.Join(base, "bin", key, "xo1")
	if err := os.MkdirAll(filepath.Dir(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fake, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	bin, root, err := binary(files)
	if err != nil || bin != fake {
		t.Fatalf("binary: %s, %v, want %s", bin, err, fake)
	}
	for _, n := range []string{"std/toml.xo", "std/llrt/xo_rt.c", "third_party/mbedtls/mbedtls.tgz"} {
		m := edit(n)
		if BinaryKey(m) != key {
			t.Errorf("a change to %s changes xo1's key", n)
			continue
		}
		b, r, err := binary(m)
		if err != nil || b != fake {
			t.Errorf("after a change to %s: binary %s, %v, want the cached %s", n, b, err, fake)
			continue
		}
		if r == root {
			t.Errorf("after a change to %s: the same source tree %s", n, r)
		}
		data, err := os.ReadFile(filepath.Join(r, filepath.FromSlash(n)))
		if err != nil || !bytes.Equal(data, m[n]) {
			t.Errorf("the source tree beside xo1 does not hold the changed %s: %v", n, err)
		}
	}
	for _, n := range []string{"selfhost/arm64/isel.xo", "selfhost/xo.mod", "std/native/prelude.xo", "std/native/http.xo"} {
		if BinaryKey(edit(n)) == key {
			t.Errorf("a change to %s keeps xo1's key", n)
		}
	}
	old := Version
	Version = "test-other"
	defer func() { Version = old }()
	if BinaryKey(files) == key {
		t.Error("another xo version keeps xo1's key")
	}
}
