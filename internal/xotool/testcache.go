package xotool

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
)

// Caches the test binaries keep across runs (bench/results/short-tests-2026-10-14.md):
// the C compile cache (below), the `use go` bindings (TestGobindDir), and
// content keyed program directories (TestProgramDir).
//
// The C compile cache:
// the runtime tests of internal/llvm compile the same C drivers with
// sanitizers on every run (http_diff.c with AddressSanitizer took about
// 5 s, three times per run). With XO_CC_CACHE set (EnableTestCCache sets
// it to <user cache>/xo/testcc), CompileC keeps each linked binary under
// the hash of everything that decides it: clang and its version, the
// target, $XO_SYSROOT, the flags, every source's contents, the contents
// of every header clang -MM reports for the C sources, and the TLS
// library's source hash when the program links it. A hit copies the
// binary (and its .dSYM on macOS) to bin. Off ("" or "off") by default:
// xo itself never uses it.

const ccCacheEnv = "XO_CC_CACHE"

// EnableTestCCache turns the C compile cache on for a test binary
// (TestMain), unless XO_CC_CACHE is already set, and removes entries
// unused for XO_CACHE_MAX_AGE (testCacheAge).
func EnableTestCCache() {
	if _, ok := os.LookupEnv(ccCacheEnv); ok {
		return
	}
	c, err := os.UserCacheDir()
	if err != nil {
		return
	}
	dir := filepath.Join(c, "xo", "testcc")
	os.Setenv(ccCacheEnv, dir)
	trimCCCache(dir, testCacheAge())
}

func ccCacheDir() string {
	d := os.Getenv(ccCacheEnv)
	if d == "off" {
		return ""
	}
	return d
}

// CompileC compiles and links C and LLVM IR sources into bin with clang
// (compileC), through the cache of XO_CC_CACHE when it is set.
func CompileC(flags, srcs []string, bin string) error {
	dir := ccCacheDir()
	if dir == "" {
		return compileC(flags, srcs, bin)
	}
	key, err := ccKey(flags, srcs)
	if err != nil {
		// no key (a source clang -MM cannot read): compile, uncached
		return compileC(flags, srcs, bin)
	}
	entry := filepath.Join(dir, key)
	name := filepath.Base(bin)
	if isFile(filepath.Join(entry, name)) {
		if err := copyEntry(entry, name, bin); err == nil {
			now := time.Now()
			os.Chtimes(entry, now, now)
			return nil
		}
	}
	if err := compileC(flags, srcs, bin); err != nil {
		return err
	}
	storeEntry(dir, entry, name, bin)
	return nil
}

var (
	ccKeyVersMu sync.Mutex
	ccKeyVers   = map[string]string{} // clang -> its --version output
)

// ccVersion is clang's --version output (once per path).
func ccVersion(cc string) string {
	ccKeyVersMu.Lock()
	defer ccKeyVersMu.Unlock()
	ver, ok := ccKeyVers[cc]
	if !ok {
		out, _ := exec.Command(cc, "--version").CombinedOutput()
		ver = string(out)
		ccKeyVers[cc] = ver
	}
	return ver
}

// ccKey is the cache key of a CompileC call.
func ccKey(flags, srcs []string) (string, error) {
	clang, err := Clang()
	if err != nil {
		return "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "cc1\n%s\n%s\n%s/%s\n%s\n%q\n", clang, ccVersion(clang), runtime.GOOS, runtime.GOARCH, os.Getenv("XO_SYSROOT"), flags)
	if usesTLS(srcs) {
		th, err := tlsSourceHash()
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "tls %s %v\n", th, tlsFlavor(flags))
	}
	var compile []string
	for _, f := range flags {
		if !strings.HasPrefix(f, "-l") && !strings.HasPrefix(f, "-Wl,") {
			compile = append(compile, f)
		}
	}
	for _, s := range srcs {
		data, err := os.ReadFile(s)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "src %s %d\n", filepath.Base(s), len(data))
		h.Write(data)
		if !strings.HasSuffix(s, ".c") {
			continue
		}
		deps, err := ccDeps(clang, compile, s)
		if err != nil {
			return "", err
		}
		for _, d := range deps {
			data, err := os.ReadFile(d)
			if err != nil {
				// a header clang did not find (-MG): its absence is
				// part of the key
				fmt.Fprintf(h, "missing %s\n", d)
				continue
			}
			// by name and contents, not by path, so worktrees share
			// entries
			fmt.Fprintf(h, "dep %s %d\n", filepath.Base(d), len(data))
			h.Write(data)
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:32], nil
}

// ccDeps lists the user headers (not system ones) that src includes
// under the compile flags, as clang -MM -MG reports them.
func ccDeps(clang string, flags []string, src string) ([]string, error) {
	args := append(append([]string{}, flags...), "-MM", "-MG", src)
	var out, errb bytes.Buffer
	cmd := exec.Command(clang, args...)
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("clang -MM %s: %v: %s", src, err, errb.String())
	}
	text := strings.ReplaceAll(out.String(), "\\\n", " ")
	_, rest, ok := strings.Cut(text, ":")
	if !ok {
		return nil, fmt.Errorf("clang -MM %s: no rule", src)
	}
	var deps []string
	for _, f := range strings.Fields(rest) {
		if f == filepath.Clean(src) || f == src {
			continue
		}
		deps = append(deps, f)
	}
	return deps, nil
}

// copyEntry copies the entry's binary (and its .dSYM) to bin.
func copyEntry(entry, name, bin string) error {
	if err := copyExe(filepath.Join(entry, name), bin); err != nil {
		return err
	}
	if d := filepath.Join(entry, name+".dSYM"); isDir(d) {
		os.RemoveAll(bin + ".dSYM")
		copyTree(d, bin+".dSYM")
	}
	return nil
}

// storeEntry puts bin (and its .dSYM) into the cache at entry, written
// in a temporary directory and renamed, so readers see whole entries.
func storeEntry(dir, entry, name, bin string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	tmp, err := os.MkdirTemp(dir, "tmp-")
	if err != nil {
		return
	}
	defer os.RemoveAll(tmp)
	if copyTree(bin, filepath.Join(tmp, name)) != nil {
		return
	}
	if isDir(bin + ".dSYM") {
		if copyTree(bin+".dSYM", filepath.Join(tmp, name+".dSYM")) != nil {
			return
		}
	}
	// another process may have stored the same entry first: keep it
	os.Rename(tmp, entry)
}

// copyExe copies the executable src to dst with cp, outside this process:
// a test binary runs many tests at once, and a child another goroutine
// forks while this process holds dst open for writing keeps the
// descriptor until its exec, so running dst right after fails with
// "text file busy" on Linux (gotchas/etxtbsy-exec-after-copy.md; Go
// issue 22315). Windows has no such error and no cp: Go copies there.
func copyExe(src, dst string) error {
	if runtime.GOOS == "windows" {
		return copyTree(src, dst)
	}
	tmp := fmt.Sprintf("%s.cc-%d-%d", dst, os.Getpid(), time.Now().UnixNano())
	if out, err := exec.Command("cp", src, tmp).CombinedOutput(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("cp %s: %v %s", src, err, out)
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// copyTree copies a file or a directory tree, keeping modes.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.CreateTemp(filepath.Dir(target), ".cc-*")
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			os.Remove(out.Name())
			return err
		}
		out.Close()
		os.Chmod(out.Name(), info.Mode().Perm())
		return os.Rename(out.Name(), target)
	})
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// TestGobindDir is a `use go` binding cache (XO_GOBIND_DIR) for test
// binaries that is kept across runs: <user cache>/xo/testgobind/<key>,
// keyed by the xo binary's contents (the binder is xo's
// selfhost/cli/bind) and the go command's version (the bindings read Go's
// export data). A fresh directory per run bound every `use go` golden
// again (about 0.8 s each) and so missed XO_BIN_CACHE as well. Entries
// unused for XO_CACHE_MAX_AGE are removed (testCacheAge).
func TestGobindDir() (string, error) {
	bin, err := Path()
	if err != nil {
		return "", err
	}
	f, err := os.Open(bin)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	gover, err := exec.Command("go", "env", "GOVERSION", "GOROOT").Output()
	if err != nil {
		return "", err
	}
	fmt.Fprintf(h, "\ngobind v1\n%s/%s\n%s", runtime.GOOS, runtime.GOARCH, gover)
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(base, "xo", "testgobind")
	trimCCCache(root, testCacheAge())
	dir := filepath.Join(root, hex.EncodeToString(h.Sum(nil))[:20])
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	now := time.Now()
	os.Chtimes(dir, now, now)
	return dir, nil
}

// TestProgramDir is a directory holding files (slash paths to contents)
// that stays the same across runs and worktrees:
// <user cache>/xo/testprog/<kind>/<hash of the files>. A program built
// from a fresh t.TempDir() never reuses a binary of XO_BIN_CACHE, because
// the build information records the program's directory; one built here
// does, while its generated code is the same. The files are written
// once, each under a temporary name and renamed, so concurrent tests see
// whole files. Callers keep their outputs elsewhere (t.TempDir()).
// Entries unused for XO_CACHE_MAX_AGE are removed (testCacheAge).
func TestProgramDir(kind string, files map[string]string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	var names []string
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, n := range names {
		fmt.Fprintf(h, "%s %d\n%s", n, len(files[n]), files[n])
	}
	root := filepath.Join(base, "xo", "testprog", kind)
	testProgTrim.Do(func() { trimCCCache(root, testCacheAge()) })
	dir := filepath.Join(root, hex.EncodeToString(h.Sum(nil))[:20])
	for _, n := range names {
		p := filepath.Join(dir, filepath.FromSlash(n))
		if data, err := os.ReadFile(p); err == nil && string(data) == files[n] {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return "", err
		}
		if err := writeAtomicUnique(p, []byte(files[n])); err != nil {
			return "", err
		}
	}
	now := time.Now()
	os.Chtimes(dir, now, now)
	return dir, nil
}

var testProgTrim sync.Once

// writeAtomicUnique writes p through a temporary file of its own (safe
// for concurrent writers of the same p) renamed into place.
func writeAtomicUnique(p string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(p), ".w-*")
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return err
	}
	os.Chmod(f.Name(), 0o644)
	return os.Rename(f.Name(), p)
}

// The fresh directories Build makes for a binary without Options.Out,
// and TempDir's: callers never removed them, and on 2026-10-14 $TMPDIR
// held 19,201 xo-build-* directories (about 70 GB). They now live under
// one directory per process (testtmp.Dir("xo-builds-"), which records
// its owner): CleanupTemp removes it (TestMain, after m.Run), and a run
// killed before that leaves it to the next sweep, which removes the
// directories of processes that have exited
// (gotchas/xotool-build-temp-leak.md).
var (
	tempMu   sync.Mutex
	tempRoot string
)

const tempPrefix = "xo-builds-"

// TempDir makes a new directory prefix* under this process's temporary
// root (created on first use).
func TempDir(prefix string) (string, error) {
	tempMu.Lock()
	defer tempMu.Unlock()
	if tempRoot == "" {
		d, err := testtmp.Dir(tempPrefix)
		if err != nil {
			return "", err
		}
		tempRoot = d
	}
	return os.MkdirTemp(tempRoot, prefix)
}

// CleanupTemp removes this process's temporary root with everything
// Build and TempDir made in it.
func CleanupTemp() {
	tempMu.Lock()
	defer tempMu.Unlock()
	if tempRoot != "" {
		os.RemoveAll(tempRoot)
		tempRoot = ""
	}
}

// testCacheAge is how long an unused entry of these caches stays:
// XO_CACHE_MAX_AGE (a Go duration, 0 keeps everything) as for xo's
// caches (decision 0223), default 24 hours, and never under two hours
// (an entry in use is touched at most when used, the same margin as
// selfhost/prune).
func testCacheAge() time.Duration {
	age := 24 * time.Hour
	if v := os.Getenv("XO_CACHE_MAX_AGE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			age = d
		}
	}
	if age <= 0 {
		return 0
	}
	return max(age, 2*time.Hour)
}

// trimCCCache removes the entries (and leftover temporary directories)
// of dir not used within age (nothing when age is 0).
func trimCCCache(dir string, age time.Duration) {
	if age <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || time.Since(info.ModTime()) < age {
			continue
		}
		os.RemoveAll(filepath.Join(dir, e.Name()))
	}
}
