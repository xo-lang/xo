package build

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/gen"
)

func TestRTPackVariant(t *testing.T) {
	for _, c := range []struct {
		flags []string
		tls   bool
		want  string
		ok    bool
	}{
		{[]string{"-O2"}, false, "rt.o", true},
		{[]string{"-O2", "-DXO_TIME", "-DXO_FS", "-DXO_STDIN", "-DXO_PROC"}, true, "rt-time-fs-stdin-proc-tls.o", true},
		{[]string{"-O2", "-DXO_PROC", "-DXO_FS"}, false, "rt-fs-proc.o", true},
		{[]string{"-O2", "-flto", "-Wno-override-module"}, false, "", false},
		{[]string{"-O2", "-fsanitize=thread"}, false, "", false},
		{[]string{"-O2", "-I", "inc"}, false, "", false},
		{[]string{"-DXO_FS"}, false, "", false},
	} {
		v, ok := rtPackVariant(c.flags, c.tls)
		if ok != c.ok || (ok && v.File() != c.want) {
			t.Errorf("rtPackVariant(%q, %v) = %s %v, want %s %v", c.flags, c.tls, v.File(), ok, c.want, c.ok)
		}
	}
	seen := map[string]bool{}
	for _, v := range RTVariants() {
		if seen[v.File()] {
			t.Errorf("two variants named %s", v.File())
		}
		seen[v.File()] = true
		if w, ok := rtPackVariant(append([]string{"-O2"}, v.Defines()...), v.TLS); !ok || w != v {
			t.Errorf("the flags of %s give %s %v", v.File(), w.File(), ok)
		}
	}
	if len(seen) != 32 {
		t.Errorf("%d variants, want 32", len(seen))
	}
}

func TestRTPackKey(t *testing.T) {
	a, err := RTPackKey(Target{"darwin", "arm64"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := RTPackKey(Target{"linux", "arm64"})
	again, _ := RTPackKey(Target{"darwin", "arm64"})
	if a == b || a != again || len(a) != 64 {
		t.Errorf("keys %s, %s, %s", a, b, again)
	}
}

// TestNormalizeAr rewrites the times, owners, and modes of an archive's
// members and keeps their names, sizes, and data (an odd sized member is
// padded).
func TestNormalizeAr(t *testing.T) {
	hdr := func(name, date, uid, gid, mode string, size int) string {
		pad := func(s string, n int) string { return s + strings.Repeat(" ", n-len(s)) }
		return pad(name, 16) + pad(date, 12) + pad(uid, 6) + pad(gid, 6) + pad(mode, 8) + pad(strconv.Itoa(size), 10) + "`\n"
	}
	in := "!<arch>\n" + hdr("a.o/", "1760000000", "501", "20", "100644", 3) + "abc\n" + hdr("bb.o/", "1760000001", "501", "20", "100600", 4) + "defg"
	out := "!<arch>\n" + hdr("a.o/", "0", "0", "0", "644", 3) + "abc\n" + hdr("bb.o/", "0", "0", "0", "644", 4) + "defg"
	p := filepath.Join(t.TempDir(), "x.a")
	os.WriteFile(p, []byte(in), 0o644)
	if err := normalizeAr(p); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != out {
		t.Errorf("got\n%q\nwant\n%q", got, out)
	}
	os.WriteFile(p, []byte("not an archive"), 0o644)
	if err := normalizeAr(p); err == nil {
		t.Error("no error for a file that is not an archive")
	}
}

// TestClangErrorRuntimeHint: a failed local compile of the runtime names
// what to install and the prebuilt runtime.
func TestClangErrorRuntimeHint(t *testing.T) {
	msg := (&ClangError{Output: "xo_rt.c:1:1: error: boom", Runtime: true}).Error()
	for _, want := range []string{"the C compiler failed on the C runtime", "xo_rt.c:1:1: error: boom", "XO_RTPACK", "no prebuilt runtime matched", "install"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
	if msg := (&ClangError{Output: "x"}).Error(); strings.Contains(msg, "XO_RTPACK") {
		t.Errorf("a failure on generated code mentions the prebuilt runtime:\n%s", msg)
	}
}

// rtPackTestHost skips unless prebuilt runtimes exist for the host and a
// C compiler is found.
func rtPackTestHost(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("compiles the runtime")
	}
	ok := false
	for _, tg := range RTPackTargets {
		ok = ok || tg == Host()
	}
	if !ok {
		t.Skipf("no prebuilt runtime for %s", Host())
	}
	if _, err := CC(); err != nil {
		t.Skip("no C compiler")
	}
	if _, err := findAr(); err != nil {
		t.Skip("no archiver")
	}
}

// TestRTPackDeterministic builds a pack twice from two work directories
// and requires every file, the objects, the TLS archive, and the
// manifest, to be byte for byte the same (decision 0157).
func TestRTPackDeterministic(t *testing.T) {
	rtPackTestHost(t)
	vs := []RTVariant{{}, {Time: true, FS: true, Stdin: true, Proc: true, TLS: true}}
	var dirs []string
	for i := 0; i < 2; i++ {
		dir, err := BuildRTPack(RTPackOptions{Target: Host(), Out: t.TempDir(), Work: filepath.Join(t.TempDir(), "work"+strconv.Itoa(i)), variants: vs})
		if err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, dir)
	}
	ents, err := os.ReadDir(dirs[0])
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	want := []string{RTPackTLSLib, "rt-time-fs-stdin-proc-tls.o", "rt.o", RTPackManifestName}
	if strings.Join(names, " ") != strings.Join(want, " ") {
		t.Errorf("pack files %q, want %q", names, want)
	}
	for _, n := range names {
		a, err1 := os.ReadFile(filepath.Join(dirs[0], n))
		b, err2 := os.ReadFile(filepath.Join(dirs[1], n))
		if err1 != nil || err2 != nil {
			t.Fatal(err1, err2)
		}
		if !bytes.Equal(a, b) {
			t.Errorf("%s differs between two builds", n)
		}
	}
	var m rtPackManifest
	data, _ := os.ReadFile(filepath.Join(dirs[0], RTPackManifestName))
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if key, _ := RTPackKey(Host()); m.Key != key || m.Target != Host().String() || len(m.Files) != 3 || m.Compiler == "" {
		t.Errorf("manifest %+v", m)
	}
}

// TestRTPackUse links a native program with a pack's object: no local
// compile of the runtime (the object cache stays empty), and the same
// output. A pack whose key differs (an edited runtime) or whose object
// fails its checksum is not used: the runtime is compiled locally.
func TestRTPackUse(t *testing.T) {
	rtPackTestHost(t)
	if NativeBackend() != BackendArm64 {
		t.Skip("the prebuilt runtime is the arm64 backend's")
	}
	root := t.TempDir()
	dir, err := BuildRTPack(RTPackOptions{Target: Host(), Out: root, variants: []RTVariant{{}}})
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "hello")
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "main.xo"), []byte("fn main(os: Os) {\n  os.stdio.println(\"hi ${[3, 1].len()}\")\n}\n"), 0o644)
	objects := func(dir string) int {
		n := 0
		filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
			if err == nil && filepath.Ext(p) == ".o" {
				n++
			}
			return nil
		})
		return n
	}
	// buildWith builds and runs the program with the pack root and returns
	// the number of runtime objects compiled locally
	buildWith := func(t *testing.T, root string) int {
		t.Helper()
		t.Setenv("XO_RTPACK", root)
		cache := t.TempDir()
		t.Setenv("XO_RTOBJ_CACHE", cache)
		t.Setenv("XO_BUILD_DIR", t.TempDir())
		res, err := Build([]string{src}, Options{Mode: gen.Build, Backend: BackendArm64, NoGC: true})
		if err != nil || res.Failed {
			t.Fatalf("build: %v %+v", err, res)
		}
		out, err := exec.Command(res.Binaries[Host()]).Output()
		if err != nil || string(out) != "hi 2\n" {
			t.Fatalf("run: %v %q", err, out)
		}
		return objects(cache)
	}
	t.Run("used", func(t *testing.T) {
		if n := buildWith(t, root); n != 0 {
			t.Errorf("%d runtime objects compiled with a matching pack", n)
		}
		t.Setenv("XO_RTPACK", root)
		if s := RTPackStatus(); !strings.Contains(s, dir) {
			t.Errorf("status %q", s)
		}
	})
	t.Run("off", func(t *testing.T) {
		if n := buildWith(t, "off"); n != 1 {
			t.Errorf("%d runtime objects compiled with XO_RTPACK=off, want 1", n)
		}
	})
	// copy the pack to a new root, changing it with edit
	variant := func(t *testing.T, edit func(dir string)) string {
		r := t.TempDir()
		d := filepath.Join(r, filepath.Base(dir))
		if err := copyDir(dir, d); err != nil {
			t.Fatal(err)
		}
		edit(d)
		return r
	}
	t.Run("other key", func(t *testing.T) {
		r := variant(t, func(d string) {
			p := filepath.Join(d, RTPackManifestName)
			data, _ := os.ReadFile(p)
			var m rtPackManifest
			json.Unmarshal(data, &m)
			m.Key = strings.Repeat("0", 64)
			data, _ = json.Marshal(m)
			os.WriteFile(p, data, 0o644)
		})
		if n := buildWith(t, r); n != 1 {
			t.Errorf("%d runtime objects compiled with a pack of another key, want 1", n)
		}
		t.Setenv("XO_RTPACK", r)
		if s := RTPackStatus(); !strings.Contains(s, "other runtime sources or flags") {
			t.Errorf("status %q", s)
		}
	})
	t.Run("bad checksum", func(t *testing.T) {
		r := variant(t, func(d string) {
			p := filepath.Join(d, "rt.o")
			data, _ := os.ReadFile(p)
			data[len(data)/2] ^= 1
			os.WriteFile(p, data, 0o644)
		})
		if n := buildWith(t, r); n != 1 {
			t.Errorf("%d runtime objects compiled with a corrupt pack, want 1", n)
		}
	})
}
