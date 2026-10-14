package xotool

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// seedDir writes a seed location (seeds/latest.txt and the host's
// tarball of version v holding files) into a new directory, as
// scripts/seed-pack.sh and the upload lay it out, and points
// XO_SEED_URL at it and the cache at a new directory.
func seedDir(t *testing.T, v string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XO_SEED_URL", "file://"+dir)
	t.Setenv("XO_CHECKOUT_CACHE", t.TempDir())
	vd := filepath.Join(dir, "seeds", v)
	if err := os.MkdirAll(vd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "seeds", "latest.txt"), []byte(v+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for name, body := range files {
		mode := int64(0o644)
		if strings.HasPrefix(name, "bin/") {
			mode = 0o755
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	zw.Close()
	plat := HostPlatform()
	if err := os.WriteFile(filepath.Join(vd, plat+".tar.gz"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buf.Bytes())
	if err := os.WriteFile(filepath.Join(vd, plat+".sha256"), []byte(hex.EncodeToString(sum[:])+"  "+plat+".tar.gz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func seedFiles() map[string]string {
	return map[string]string{
		"bin/" + seedExe():         "#!/bin/sh\necho xo 0.10.0\n",
		"lib/xo/std/prelude.xo":    "// prelude\n",
		"lib/xo/third_party/x.txt": "x\n",
	}
}

// TestFetchSeedLocal: the newest seed of a local location is checked,
// unpacked into the cache keyed by version, and reused from there.
func TestFetchSeedLocal(t *testing.T) {
	dir := seedDir(t, "0.10.0", seedFiles())
	var log bytes.Buffer
	bin, err := FetchSeed(&log)
	if err != nil {
		t.Fatalf("FetchSeed: %v\n%s", err, log.String())
	}
	base, _ := cacheBase()
	want := filepath.Join(base, "seeds", "0.10.0", HostPlatform(), "tree", "bin", seedExe())
	if bin != want {
		t.Errorf("bin %s, want %s", bin, want)
	}
	if b, _ := os.ReadFile(bin); !strings.Contains(string(b), "xo 0.10.0") {
		t.Errorf("bin/xo is %q", b)
	}
	if fi, err := os.Stat(bin); err != nil || runtime.GOOS != "windows" && fi.Mode()&0o111 == 0 {
		t.Errorf("bin/xo not executable: %v %v", fi.Mode(), err)
	}
	if !isFile(filepath.Join(filepath.Dir(filepath.Dir(bin)), "lib", "xo", "std", "prelude.xo")) {
		t.Error("lib/xo/std/prelude.xo not unpacked")
	}
	// the cache serves it once the location is gone, and the newest
	// unpacked seed serves when latest.txt cannot be read
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "seeds"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "seeds", "latest.txt"), []byte("0.10.0\n"), 0o644)
	if b2, err := FetchSeed(io.Discard); err != nil || b2 != bin {
		t.Errorf("cached FetchSeed = %s, %v", b2, err)
	}
	os.Remove(filepath.Join(dir, "seeds", "latest.txt"))
	if b3, err := FetchSeed(io.Discard); err != nil || b3 != bin {
		t.Errorf("FetchSeed without latest.txt = %s, %v", b3, err)
	}
}

// TestFetchSeedRejects: a wrong sha256, a missing platform, and a name
// outside the tree fail, and nothing is left unpacked.
func TestFetchSeedRejects(t *testing.T) {
	t.Run("sha256", func(t *testing.T) {
		dir := seedDir(t, "0.10.0", seedFiles())
		p := filepath.Join(dir, "seeds", "0.10.0", HostPlatform()+".sha256")
		os.WriteFile(p, []byte(strings.Repeat("0", 64)+"  x.tar.gz\n"), 0o644)
		if _, err := FetchSeed(io.Discard); err == nil || !strings.Contains(err.Error(), "sha256") {
			t.Errorf("err = %v, want a sha256 mismatch", err)
		}
		base, _ := cacheBase()
		if isFile(filepath.Join(base, "seeds", "0.10.0", HostPlatform(), "tree", "bin", seedExe())) {
			t.Error("a seed with a wrong sha256 was unpacked")
		}
	})
	t.Run("platform", func(t *testing.T) {
		dir := seedDir(t, "0.10.0", seedFiles())
		os.Remove(filepath.Join(dir, "seeds", "0.10.0", HostPlatform()+".sha256"))
		if _, err := FetchSeed(io.Discard); err == nil || !strings.Contains(err.Error(), "no seed 0.10.0 for "+HostPlatform()) {
			t.Errorf("err = %v, want no seed for the host", err)
		}
	})
	t.Run("outside", func(t *testing.T) {
		f := seedFiles()
		f["../evil"] = "x"
		seedDir(t, "0.10.0", f)
		if _, err := FetchSeed(io.Discard); err == nil || !strings.Contains(err.Error(), "outside the tree") {
			t.Errorf("err = %v, want a name outside the tree", err)
		}
	})
	t.Run("version", func(t *testing.T) {
		dir := seedDir(t, "0.10.0", seedFiles())
		os.WriteFile(filepath.Join(dir, "seeds", "latest.txt"), []byte("../x\n"), 0o644)
		if _, err := FetchSeed(io.Discard); err == nil || !strings.Contains(err.Error(), "not a version") {
			t.Errorf("err = %v, want not a version", err)
		}
	})
}

// TestSeedOrder: the prebuilt seed comes after the local candidates and
// before the go-compiler-final root, and XO_SEED_URL=off drops it.
func TestSeedOrder(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XO_CHECKOUT_CACHE", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	seed := filepath.Join(t.TempDir(), "xo")
	os.WriteFile(seed, []byte("x"), 0o755)
	t.Setenv("XO_SEED", seed)
	t.Setenv("XO_SEED_URL", "")
	t.Setenv("XO_SEED_BUCKET", "")
	ss, _ := Seeds(root)
	if len(ss) != 3 || ss[0].Path != seed || !strings.Contains(ss[1].Why, "s3://"+DefaultSeedBucket) || ss[2].Resolve == nil {
		t.Fatalf("seeds %+v", ss)
	}
	t.Setenv("XO_SEED_URL", "off")
	if ss, _ := Seeds(root); len(ss) != 2 {
		t.Errorf("XO_SEED_URL=off: seeds %+v", ss)
	}
	t.Setenv("XO_SEED_URL", "")
	t.Setenv("XO_SEED_BUCKET", "b/p")
	if u := SeedURL(); u != "s3://b/p" {
		t.Errorf("SeedURL %s", u)
	}
}

func TestVersionLess(t *testing.T) {
	for _, c := range []struct {
		a, b string
		less bool
	}{{"0.9.0", "0.10.0", true}, {"0.10.0", "0.9.0", false}, {"0.10.0", "0.10.1", true}, {"1.0.0", "0.99.0", false}, {"0.10.0", "0.10.0", false}} {
		if got := versionLess(c.a, c.b); got != c.less {
			t.Errorf("versionLess(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

// TestSeedPackHello: scripts/seed-pack.sh packs a built xo, FetchSeed
// unpacks it, and the unpacked bin/xo, with no XO_STD and a working
// directory with no std, finds its own lib/xo/std and runs hello with
// the host's native backend. It needs a built xo: $XO_CLI, else the
// checkout's bin/xo (`go run ./scripts/xobuild`); skipped without one.
func TestSeedPackHello(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("seed-pack.sh is a bash script; Windows seeds are not packed yet")
	}
	root := repoRoot(t)
	xo := filepath.Join(root, "bin", "xo")
	if f := strings.Fields(os.Getenv("XO_CLI")); len(f) > 0 {
		xo = f[0]
	}
	if !isFile(xo) {
		t.Skipf("no built xo (%s): go run ./scripts/xobuild, or set XO_CLI", xo)
	}
	loc := t.TempDir()
	out := filepath.Join(loc, "seeds", "pack")
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "seed-pack.sh"), xo, out)
	cmd.Env = append(os.Environ(), "XO_SEED_ALLOW_DIRTY=1")
	o, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("seed-pack.sh: %v\n%s", err, o)
	}
	man, err := os.ReadFile(filepath.Join(out, "manifest.txt"))
	if err != nil {
		t.Fatal(err)
	}
	v := ""
	for _, l := range strings.Split(string(man), "\n") {
		if s, ok := strings.CutPrefix(l, "version "); ok {
			v = s
		}
	}
	if v == "" || !strings.Contains(string(man), "sha256 "+HostPlatform()+" ") {
		t.Fatalf("manifest.txt:\n%s", man)
	}
	if err := os.Rename(out, filepath.Join(loc, "seeds", v)); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(loc, "seeds", "latest.txt"), []byte(v+"\n"), 0o644)
	t.Setenv("XO_SEED_URL", loc)
	t.Setenv("XO_CHECKOUT_CACHE", t.TempDir())
	t0 := time.Now()
	bin, err := FetchSeed(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pack and fetch: %s", time.Since(t0))
	work := t.TempDir()
	os.MkdirAll(filepath.Join(work, "hello"), 0o755)
	os.WriteFile(filepath.Join(work, "hello", "main.xo"), []byte("fn main(os: Os) uses (stdio) {\n  os.stdio.println(\"hello from the seed\")\n}\n"), 0o644)
	backend, env := hostBackend()
	run := exec.Command(bin, "run", "--backend="+backend, "./hello")
	run.Dir = work
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "XO_STD=") && !strings.HasPrefix(e, "XO_RTPACK=") && !strings.HasPrefix(e, "PWD=") {
			run.Env = append(run.Env, e)
		}
	}
	run.Env = append(append(run.Env, "PWD="+work), env...)
	o, err = run.CombinedOutput()
	if err != nil || !strings.Contains(string(o), "hello from the seed") {
		t.Fatalf("unpacked seed running hello: %v\n%s", err, o)
	}
}
