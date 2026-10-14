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

// testSeed is a seed name as the bucket has them (seeds/latest.txt of
// 2026-10-13): a version and a commit.
const testSeed = "0.9.0-9b6cd014"

// seedDir writes a seed location (seeds/latest.txt and the host's
// tarball of seed name v holding files) into a new directory, laid out
// as the bucket, and points XO_SEED_URL at it and the cache at a new
// directory.
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
		if strings.Contains(name, "bin/") {
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
	if err := os.WriteFile(filepath.Join(vd, plat+".tar.gz.sha256"), []byte(hex.EncodeToString(sum[:])+"  "+plat+".tar.gz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func seedFiles() map[string]string {
	return map[string]string{
		"xo/bin/" + seedExe(): "#!/bin/sh\necho xo 0.9.0\n",
		"xo/std/prelude.xo":   "// prelude\n",
	}
}

// seedBin is where FetchSeed unpacks seed v's xo.
func seedBin(v string) string {
	base, _ := cacheBase()
	return filepath.Join(base, "seeds", v, HostPlatform(), "tree", "xo", "bin", seedExe())
}

// TestFetchSeedLocal: the newest seed of a local location is checked,
// unpacked into the cache keyed by its name, and reused from there.
func TestFetchSeedLocal(t *testing.T) {
	dir := seedDir(t, testSeed, seedFiles())
	var log bytes.Buffer
	bin, err := FetchSeed(&log)
	if err != nil {
		t.Fatalf("FetchSeed: %v\n%s", err, log.String())
	}
	if want := seedBin(testSeed); bin != want {
		t.Errorf("bin %s, want %s", bin, want)
	}
	if b, _ := os.ReadFile(bin); !strings.Contains(string(b), "xo 0.9.0") {
		t.Errorf("xo/bin/xo is %q", b)
	}
	if fi, err := os.Stat(bin); err != nil || runtime.GOOS != "windows" && fi.Mode()&0o111 == 0 {
		t.Errorf("xo/bin/xo not executable: %v %v", fi.Mode(), err)
	}
	if !isFile(filepath.Join(filepath.Dir(filepath.Dir(bin)), "std", "prelude.xo")) {
		t.Error("xo/std/prelude.xo not unpacked beside bin")
	}
	// the cache serves it once the tarball is gone, and the newest
	// unpacked seed serves when latest.txt cannot be read
	if err := os.RemoveAll(filepath.Join(dir, "seeds", testSeed)); err != nil {
		t.Fatal(err)
	}
	if b2, err := FetchSeed(io.Discard); err != nil || b2 != bin {
		t.Errorf("cached FetchSeed = %s, %v", b2, err)
	}
	os.Remove(filepath.Join(dir, "seeds", "latest.txt"))
	if b3, err := FetchSeed(io.Discard); err != nil || b3 != bin {
		t.Errorf("FetchSeed without latest.txt = %s, %v", b3, err)
	}
}

// TestFetchSeedRejects: a wrong sha256, a missing platform, a name
// outside the tree, and a bad latest.txt fail, and nothing is left
// unpacked.
func TestFetchSeedRejects(t *testing.T) {
	t.Run("sha256", func(t *testing.T) {
		dir := seedDir(t, testSeed, seedFiles())
		p := filepath.Join(dir, "seeds", testSeed, HostPlatform()+".tar.gz.sha256")
		os.WriteFile(p, []byte(strings.Repeat("0", 64)+"  x.tar.gz\n"), 0o644)
		if _, err := FetchSeed(io.Discard); err == nil || !strings.Contains(err.Error(), "sha256") {
			t.Errorf("err = %v, want a sha256 mismatch", err)
		}
		if isFile(seedBin(testSeed)) {
			t.Error("a seed with a wrong sha256 was unpacked")
		}
	})
	t.Run("platform", func(t *testing.T) {
		dir := seedDir(t, testSeed, seedFiles())
		os.Remove(filepath.Join(dir, "seeds", testSeed, HostPlatform()+".tar.gz.sha256"))
		if _, err := FetchSeed(io.Discard); err == nil || !strings.Contains(err.Error(), "no seed "+testSeed+" for "+HostPlatform()) {
			t.Errorf("err = %v, want no seed for the host", err)
		}
	})
	t.Run("outside", func(t *testing.T) {
		f := seedFiles()
		f["../evil"] = "x"
		seedDir(t, testSeed, f)
		if _, err := FetchSeed(io.Discard); err == nil || !strings.Contains(err.Error(), "outside the tree") {
			t.Errorf("err = %v, want a name outside the tree", err)
		}
	})
	t.Run("latest", func(t *testing.T) {
		dir := seedDir(t, testSeed, seedFiles())
		os.WriteFile(filepath.Join(dir, "seeds", "latest.txt"), []byte("../x\n"), 0o644)
		if _, err := FetchSeed(io.Discard); err == nil || !strings.Contains(err.Error(), "not a seed name") {
			t.Errorf("err = %v, want not a seed name", err)
		}
	})
}

// TestSeedOrder: the prebuilt seed comes after the local candidates;
// the go-compiler-final root only with XO_SEED_FALLBACK=chain (last) or
// from-source (alone); XO_SEED_URL=off drops the prebuilt seed.
func TestSeedOrder(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XO_CHECKOUT_CACHE", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	seed := filepath.Join(t.TempDir(), "xo")
	os.WriteFile(seed, []byte("x"), 0o755)
	t.Setenv("XO_SEED", seed)
	t.Setenv("XO_SEED_URL", "")
	t.Setenv("XO_SEED_BUCKET", "")
	t.Setenv("XO_SEED_FALLBACK", "")
	ss, _ := Seeds(root)
	if len(ss) != 2 || ss[0].Path != seed || !strings.Contains(ss[1].Why, "s3://"+DefaultSeedBucket) {
		t.Fatalf("seeds %+v", ss)
	}
	t.Setenv("XO_SEED_FALLBACK", "chain")
	if ss, _ := Seeds(root); len(ss) != 3 || !strings.Contains(ss[2].Why, SeedTag) {
		t.Errorf("XO_SEED_FALLBACK=chain: seeds %+v", ss)
	}
	t.Setenv("XO_SEED_FALLBACK", "from-source")
	if ss, _ := Seeds(root); len(ss) != 1 || !strings.Contains(ss[0].Why, SeedTag) {
		t.Errorf("XO_SEED_FALLBACK=from-source: seeds %+v", ss)
	}
	t.Setenv("XO_SEED_FALLBACK", "yes")
	if _, err := Seeds(root); err == nil {
		t.Error("XO_SEED_FALLBACK=yes: no error")
	}
	t.Setenv("XO_SEED_FALLBACK", "")
	t.Setenv("XO_SEED_URL", "off")
	if ss, _ := Seeds(root); len(ss) != 1 {
		t.Errorf("XO_SEED_URL=off: seeds %+v", ss)
	}
	t.Setenv("XO_SEED_URL", "")
	t.Setenv("XO_SEED_BUCKET", "b/p")
	if u := SeedURL(); u != "s3://b/p" {
		t.Errorf("SeedURL %s", u)
	}
}

// TestFetchFailNoChain: with no local xo and a seed location without
// the host's seed, the build fails with an error naming the location and
// the opt in, and never starts the go-compiler-final root or the chain
// (owner, 2026-10-13).
func TestFetchFailNoChain(t *testing.T) {
	root := t.TempDir()
	loc := t.TempDir()
	os.MkdirAll(filepath.Join(loc, "seeds"), 0o755)
	os.WriteFile(filepath.Join(loc, "seeds", "latest.txt"), []byte(testSeed+"\n"), 0o644)
	t.Setenv("XO_SEED_URL", loc)
	t.Setenv("XO_CHECKOUT_CACHE", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("XO_SEED", "")
	t.Setenv("XO_SEED_FALLBACK", "")
	var log bytes.Buffer
	err := buildLocked(root, filepath.Join(t.TempDir(), "xo"), &log)
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{"opt in", loc + "/seeds/", "XO_SEED_FALLBACK=chain", "no seed " + testSeed + " for " + HostPlatform()} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
	if strings.Contains(log.String(), "seed-chain") || strings.Contains(log.String(), "building the "+SeedTag) {
		t.Errorf("the chain started:\n%s", log.String())
	}
	base, _ := cacheBase()
	if _, err := os.Stat(filepath.Join(base, "seed")); err == nil {
		t.Error("the root seed directory was created")
	}
}

func TestVersionLess(t *testing.T) {
	for _, c := range []struct {
		a, b string
		less bool
	}{
		{"0.9.0", "0.10.0", true}, {"0.10.0", "0.9.0", false}, {"0.10.0", "0.10.1", true},
		{"1.0.0", "0.99.0", false}, {"0.10.0", "0.10.0", false},
		{"0.9.0-9b6cd014", "0.10.0-0123abcd", true}, {"0.10.0-0123abcd", "0.9.0-9b6cd014", false},
	} {
		if got := versionLess(c.a, c.b); got != c.less {
			t.Errorf("versionLess(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

// TestSeedPackHello: scripts/seed-pack.sh packs a built xo, FetchSeed
// unpacks it, and the unpacked xo/bin/xo, with no XO_STD and a working
// directory with no std, finds its own xo/std and runs hello with the
// host's native backend. It needs a built xo: $XO_CLI, else the
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
	cmd.Env = append(os.Environ(), "XO_SEED_ALLOW_DIRTY=1", "XO_SEED_GZIP_LEVEL=1") // the level does not matter here
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	o, err := cmd.Output()
	if err != nil {
		t.Fatalf("seed-pack.sh: %v\n%s%s", err, o, stderr.String())
	}
	name := strings.TrimSpace(string(o))
	man, err := os.ReadFile(filepath.Join(out, "manifest.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"version ", "commit ", "commit_full ", "date ", "layout ", HostPlatform() + " "} {
		if !strings.Contains(string(man), "\n"+want) && !strings.HasPrefix(string(man), want) {
			t.Errorf("manifest.txt has no %q line:\n%s", want, man)
		}
	}
	if !seedNameRE.MatchString(name) {
		t.Fatalf("seed-pack.sh printed %q, want a seed name", name)
	}
	if err := os.Rename(out, filepath.Join(loc, "seeds", name)); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(loc, "seeds", "latest.txt"), []byte(name+"\n"), 0o644)
	t.Setenv("XO_SEED_URL", loc)
	t.Setenv("XO_CHECKOUT_CACHE", t.TempDir())
	t0 := time.Now()
	bin, err := FetchSeed(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("fetch: %s", time.Since(t0))
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
