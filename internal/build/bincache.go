package build

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The binary cache (XO_BIN_CACHE, off by default) keeps linked programs
// by the hash of everything their link reads: the generated sources and
// the runtime written into the build directory, the compiler arguments,
// and the toolchain's identity. A build whose hash is cached copies the
// binary instead of running go build or clang. The test suites turn it on
// (EnableTestBinCache): the same golden program is built by several tests
// and modes with byte identical generated code, and again on every run;
// `go build` with every package cached still links for about a second, and
// a native build compiles the C runtime with -O2 -flto each time.
//
// Entries are written to a temporary name and renamed into place, so
// concurrent test processes may share the directory; within a process,
// builds of the same key wait for each other (one builds, the others
// hit). The directory is trimmed to XO_BIN_CACHE_MAX bytes (default
// 6 GiB), least recently used first, at most once a minute.

// binCacheMaxDefault is the cap when XO_BIN_CACHE_MAX is not set.
const binCacheMaxDefault = 6 << 30

// BinCacheDir is the binary cache directory: $XO_BIN_CACHE, or "" (off)
// when it is unset or "off".
func BinCacheDir() string {
	d := os.Getenv("XO_BIN_CACHE")
	if d == "off" {
		return ""
	}
	return d
}

// EnableTestBinCache turns the binary cache on for a test process: it
// sets XO_BIN_CACHE to <user cache>/xo/testbin unless the variable is
// set (XO_BIN_CACHE=off keeps it off, so every build runs the
// toolchain).
func EnableTestBinCache() {
	if _, ok := os.LookupEnv("XO_BIN_CACHE"); ok {
		return
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return
	}
	os.Setenv("XO_BIN_CACHE", filepath.Join(base, "xo", "testbin"))
}

// binKey accumulates the inputs of a link.
type binKey struct{ h hash.Hash }

func newBinKey(kind string) *binKey {
	k := &binKey{h: sha256.New()}
	// v1: the layout of the key itself; bump it when its inputs change
	k.str("xo-bincache v1 " + kind)
	return k
}

func (k *binKey) str(s string) { fmt.Fprintf(k.h, "%d:%s\n", len(s), s) }

func (k *binKey) strs(ss []string) {
	k.str(strconv.Itoa(len(ss)))
	for _, s := range ss {
		k.str(s)
	}
}

// tree adds every regular file under dir (path relative to dir and its
// contents), skipping the top level directories skip accepts.
func (k *binKey) tree(dir string, skip func(name string) bool) error {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir && filepath.Dir(p) == dir && skip(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("bincache: %s is not a regular file", p)
		}
		files = append(files, p)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		k.str(filepath.ToSlash(rel))
		k.str(string(data))
	}
	return nil
}

func (k *binKey) sum() string { return hex.EncodeToString(k.h.Sum(nil)) }

var (
	binMu    sync.Mutex
	binLocks = map[string]*sync.Mutex{}
	toolIDs  = map[string]string{}
)

// toolID identifies a toolchain by the output of a command (go env, cc
// --version), once per process.
func toolID(name string, args ...string) string {
	key := name + " " + strings.Join(args, " ")
	binMu.Lock()
	id, ok := toolIDs[key]
	binMu.Unlock()
	if ok {
		return id
	}
	out, err := exec.Command(name, args...).CombinedOutput()
	id = string(out)
	if err != nil {
		id = "error " + err.Error()
	}
	if p, err := exec.LookPath(name); err == nil {
		if fi, err := os.Stat(p); err == nil {
			id += fmt.Sprintf("\n%s %d %d", p, fi.Size(), fi.ModTime().UnixNano())
		}
	}
	binMu.Lock()
	toolIDs[key] = id
	binMu.Unlock()
	return id
}

func keyLock(key string) *sync.Mutex {
	binMu.Lock()
	defer binMu.Unlock()
	m := binLocks[key]
	if m == nil {
		m = &sync.Mutex{}
		binLocks[key] = m
	}
	return m
}

// BinCacheStats counts hits and misses of this process (tests).
var BinCacheStats struct {
	sync.Mutex
	Hits, Misses int
}

// withBinCache runs link, which writes bin, unless the cache holds the
// binary of key: then bin is a copy of it. A successful link is added to
// the cache. With the cache off, or key "" (the inputs could not be
// hashed), it only runs link.
func withBinCache(key, bin string, link func() error) error {
	dir := BinCacheDir()
	if dir == "" || key == "" {
		return link()
	}
	// the linkers name the ad hoc code signature of a macOS binary after
	// its file: the same inputs give the same bytes only under the same
	// name
	sum := sha256.Sum256([]byte(key + "\x00" + filepath.Base(bin)))
	key = hex.EncodeToString(sum[:])
	m := keyLock(key)
	m.Lock()
	defer m.Unlock()
	entry := filepath.Join(dir, key[:2], key)
	if fetchBin(entry, bin) {
		BinCacheStats.Lock()
		BinCacheStats.Hits++
		BinCacheStats.Unlock()
		return nil
	}
	BinCacheStats.Lock()
	BinCacheStats.Misses++
	BinCacheStats.Unlock()
	// bin may be a hard link to an entry: never write through it
	os.Remove(bin)
	if err := link(); err != nil {
		return err
	}
	storeBin(entry, bin)
	trimCacheDir(dir, binCacheMax())
	return nil
}

// fetchBin makes bin a hard link to (or else a copy of) entry. The link is
// made under a temporary name and renamed over bin, so a program already
// at bin (run by another test) never goes missing.
func fetchBin(entry, bin string) bool {
	fi, err := os.Stat(entry)
	if err != nil || !fi.Mode().IsRegular() {
		return false
	}
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		return false
	}
	now := time.Now()
	os.Chtimes(entry, now, now) // least recently used trimming
	tmp := fmt.Sprintf("%s.tmp%d.%d", bin, os.Getpid(), time.Now().UnixNano())
	if os.Link(entry, tmp) == nil {
		if os.Rename(tmp, bin) == nil {
			return true
		}
		os.Remove(tmp)
		return false
	}
	return copyFile(entry, bin) == nil
}

// storeBin adds bin to the cache as entry: a hard link (or copy) under a
// temporary name, renamed into place.
func storeBin(entry, bin string) {
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		return
	}
	tmp := fmt.Sprintf("%s.tmp%d.%d", entry, os.Getpid(), time.Now().UnixNano())
	if os.Link(bin, tmp) != nil && copyFile(bin, tmp) != nil {
		os.Remove(tmp)
		return
	}
	if os.Rename(tmp, entry) != nil {
		os.Remove(tmp)
	}
}

// copyFile copies src to dst (through a temporary name renamed over it),
// for a link across file systems (on Linux /tmp is often a tmpfs apart
// from the user cache). The file is written while holding
// syscall.ForkLock for reading, so no other goroutine forks while the
// write descriptor is open: a child forked then holds the descriptor
// until it execs, and exec of dst in that window failed with "text file
// busy" (ETXTBSY) in parallel golden tests on Linux (2026-10-12, gotcha
// linux-etxtbsy-copied-binary).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := fmt.Sprintf("%s.copy%d.%d", dst, os.Getpid(), time.Now().UnixNano())
	syscall.ForkLock.RLock()
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		syscall.ForkLock.RUnlock()
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	syscall.ForkLock.RUnlock()
	if err == nil {
		err = os.Rename(tmp, dst)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// binCacheMax is the cap in bytes: XO_BIN_CACHE_MAX, else 6 GiB.
func binCacheMax() int64 {
	if v, err := strconv.ParseInt(os.Getenv("XO_BIN_CACHE_MAX"), 10, 64); err == nil && v > 0 {
		return v
	}
	return binCacheMaxDefault
}

// trimCacheDir removes the least recently used entries once a cache is
// over its cap, down to three quarters of it, at most once a minute (a
// stamp file). Removing an entry another process is linking or copying
// is safe: the link fails (the build runs) or the open copy completes.
func trimCacheDir(dir string, limit int64) {
	stamp := filepath.Join(dir, "trim-stamp")
	if fi, err := os.Stat(stamp); err == nil && time.Since(fi.ModTime()) < time.Minute {
		return
	}
	_ = os.WriteFile(stamp, nil, 0o644)
	type entry struct {
		path string
		size int64
		mod  time.Time
	}
	var files []entry
	var total int64
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || p == stamp {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		// a leftover temporary file of a crashed process
		if strings.Contains(d.Name(), ".tmp") || strings.Contains(d.Name(), ".copy") {
			if time.Since(info.ModTime()) > time.Hour {
				os.Remove(p)
			}
			return nil
		}
		files = append(files, entry{p, info.Size(), info.ModTime()})
		total += info.Size()
		return nil
	})

	if total <= limit {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	for _, f := range files {
		if total <= limit/4*3 {
			break
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
}

// goBinKey is the cache key of a go build in dir with args (without the
// output path) and env: every file of the build directory but the
// binaries and the native backends' subdirectories, and the go command's
// identity.
func goBinKey(dir string, args []string, t Target, race bool) string {
	if BinCacheDir() == "" {
		return ""
	}
	k := newBinKey("go")
	k.str(toolID(goTool(), "env", "GOVERSION", "GOROOT", "GOEXPERIMENT", "GOARM64", "GOAMD64", "GOARM", "GODEBUG", "GOTOOLCHAIN"))
	k.strs(args)
	k.str(t.String())
	if race {
		// cgo: the C compiler and its flags
		k.str("race")
		for _, v := range []string{"CC", "CGO_CFLAGS", "CGO_LDFLAGS", "CGO_CPPFLAGS"} {
			k.str(v + "=" + os.Getenv(v))
		}
		k.str(toolID("cc", "--version"))
	}
	err := k.tree(dir, func(name string) bool {
		return name == "bin" || strings.HasPrefix(name, "llvm") || strings.HasPrefix(name, "arm64")
	})
	if err != nil {
		return ""
	}
	return k.sum()
}

// nativeBinKey is the cache key of a native link of srcs, all in dir,
// with cc and flags: every file of dir but its bin directory, the C
// compiler's identity, the macOS SDK, and the TLS library sources when
// the program uses TLS.
func nativeBinKey(dir, cc string, flags, srcs []string) string {
	if BinCacheDir() == "" {
		return ""
	}
	k := newBinKey("native")
	k.str(toolID(cc, "--version"))
	k.strs(flags)
	for _, s := range srcs {
		rel, err := filepath.Rel(dir, s)
		if err != nil || strings.HasPrefix(rel, "..") {
			return ""
		}
		k.str(filepath.ToSlash(rel))
	}
	sysrootMu.Lock()
	chosen, root := sysrootChosen, sysroot
	sysrootMu.Unlock()
	if !chosen {
		// the SDK the last process linked with, which this one tries
		// first (ccBuild)
		if memo := sysrootMemo(); memo != "" {
			data, _ := os.ReadFile(memo)
			root = strings.TrimSpace(string(data))
		}
	}
	k.str("sysroot " + root + " " + os.Getenv("XO_SYSROOT"))
	// a prebuilt runtime the link may use (decision 0157)
	k.str("rtpack " + rtPackID())
	if usesTLS(srcs) {
		tlsMu.Lock()
		h, err := tlsSourceHash()
		tlsMu.Unlock()
		if err != nil {
			return ""
		}
		k.str("tls " + h)
	}
	if err := k.tree(dir, func(name string) bool { return name == "bin" }); err != nil {
		return ""
	}
	return k.sum()
}
