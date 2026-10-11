package build

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The C runtime of the native backends (std/llrt/xo_rt.c and the files it
// includes) is most of the time of a native build: 0.66 s of 0.8 s for a
// small program with -O2 -flto, 1.1 s with -O2 on 2026-10-09. A link
// therefore compiles it once per compiler, flags, SDK, and sources into an
// object under <user cache>/xo/rtobj (XO_RTOBJ_CACHE: another directory,
// or off) and links that object. Clang compiles each source of a link to
// its own object anyway, so the binary is the same; with -flto the
// object holds bitcode and link time optimization still sees the whole
// program. The cache is trimmed to 512 MiB, least recently used first.

const rtObjMax = 512 << 20

// rtObjDir is the runtime object cache, or "" when it is off.
func rtObjDir() string {
	d := os.Getenv("XO_RTOBJ_CACHE")
	switch d {
	case "off":
		return ""
	case "":
		base, err := os.UserCacheDir()
		if err != nil {
			return ""
		}
		return filepath.Join(base, "xo", "rtobj")
	}
	return d
}

// runtimeSource is the index in srcs of the C runtime (a file named
// xo_rt.c) when the object cache is on, or -1: none, the cache off, a
// compile only call (-c), or flags that could read files the cache key
// does not cover (-I, -include, ...).
func runtimeSource(srcs, flags []string) int {
	if rtObjDir() == "" {
		return -1
	}
	return runtimeIndex(srcs, flags)
}

// runtimeIndex is runtimeSource without the cache: the index of the
// runtime a link compiles, which a prebuilt object may replace
// (rtpack.go).
func runtimeIndex(srcs, flags []string) int {
	for _, f := range flags {
		if f == "-c" || f == "-S" || f == "-E" || strings.HasPrefix(f, "-I") || strings.HasPrefix(f, "-include") ||
			strings.HasPrefix(f, "-iquote") || strings.HasPrefix(f, "-isystem") || strings.HasPrefix(f, "-o") {
			return -1
		}
	}
	for i, s := range srcs {
		if filepath.Base(s) == "xo_rt.c" {
			return i
		}
	}
	return -1
}

// runtimeObject returns the object of src (the runtime) compiled with cc,
// flags, the SDK root, and the extra compile arguments (TLS), compiling it
// when the cache has none. Its key covers every .c and .h file of src's
// directory, where the runtime's includes are, except the program's zone
// data (xo_tzdata.c, compiled as a source of its own). A failed compile returns clang's
// output and error; "" without an error means compile the source in the
// link instead.
func runtimeObject(cc string, flags []string, root string, extra []string, src string) (string, string, error) {
	cache := rtObjDir()
	k := newBinKey("rtobj")
	k.str(toolID(cc, "--version"))
	k.strs(flags)
	k.str("sysroot " + root)
	k.strs(extra)
	if usesTLSFlags(extra) {
		tlsMu.Lock()
		h, err := tlsSourceHash()
		tlsMu.Unlock()
		if err != nil {
			return "", "", nil
		}
		k.str("tls " + h)
	}
	dir := filepath.Dir(src)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", "", nil
	}
	for _, e := range ents {
		n := e.Name()
		// the zone data is a source of its own, not part of the runtime
		// (decision 0157), and an older build directory may hold a stale
		// xo_tzdata.h
		if e.IsDir() || !(strings.HasSuffix(n, ".c") || strings.HasSuffix(n, ".h")) || strings.HasPrefix(n, "xo_tzdata.") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			return "", "", nil
		}
		k.str(n)
		k.str(string(data))
	}
	k.str("main " + filepath.Base(src))
	key := k.sum()
	obj := filepath.Join(cache, key[:2], key+".o")
	m := keyLock("rtobj " + key)
	m.Lock()
	defer m.Unlock()
	if fi, err := os.Stat(obj); err == nil && fi.Mode().IsRegular() {
		now := time.Now()
		os.Chtimes(obj, now, now)
		return obj, "", nil
	}
	if err := os.MkdirAll(filepath.Dir(obj), 0o755); err != nil {
		return "", "", nil
	}
	tmp := fmt.Sprintf("%s.tmp%d.%d", obj, os.Getpid(), time.Now().UnixNano())
	args := append(append([]string(nil), flags...), "-fexceptions", "-funwind-tables", "-c", "-o", tmp)
	if root != "" {
		args = append(args, "-isysroot", root)
	}
	args = append(append(args, extra...), src)
	cmd := exec.Command(cc, args...)
	var b bytes.Buffer
	cmd.Stdout, cmd.Stderr = &b, &b
	if err := cmd.Run(); err != nil {
		os.Remove(tmp)
		return "", b.String(), err
	}
	if err := os.Rename(tmp, obj); err != nil {
		os.Remove(tmp)
		return "", "", nil
	}
	trimCacheDir(cache, rtObjMax)
	return obj, "", nil
}

// usesTLSFlags reports whether compile arguments are TLS's (tlsArgs).
func usesTLSFlags(args []string) bool {
	for _, a := range args {
		if a == "-DXO_TLS" {
			return true
		}
	}
	return false
}
