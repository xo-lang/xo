// Package selfdriver runs `xo build|run|test` for the native backends
// through the self hosted driver xo1 (selfhost/xo): decision 0178 switch
// point A, approved by the owner on 2026-10-12.
//
// Which commands: build, run, and test whose backend is native
// (--backend=arm64 or llvm, or --no-gc without a backend) on a
// darwin/arm64 or linux/arm64 host. Everything else, the Go backend
// included, stays with the Go driver. So do the flags xo1 does not
// implement yet (--ci, --hybrid, --real) and a test of a workspace (the
// Go driver takes its modules as one program).
//
// Where xo1 comes from: $XO1 names a binary; else the sources embedded in
// this xo (package selfhost, std, third_party) are written under
// <user cache>/xo/xo1/<key> (key: the SHA-256 of every embedded file)
// and xo1 is built there once by this xo with the arm64 backend, under a
// file lock; later runs reuse it. xo1 reads std and third_party from the
// same directory, so the build is a function of the embedded sources.
//
// Rollback: XO_SELF_DRIVER=0 (or off) runs every command with the Go
// driver, as before the switch.
//
// A program xo1 does not support yet (a construct its SSA builder or an
// emitter skips) is handed back: xo1 writes the reason to the file
// XO1_FALLBACK names and exits 125, and the Go driver builds it (and
// reports its XO1201 diagnostics).
package selfdriver

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/selfhost"
	"github.com/xo-lang/xo/std"
	fiatcrypto "github.com/xo-lang/xo/third_party/fiat-crypto"
	mbedtls "github.com/xo-lang/xo/third_party/mbedtls"
)

// Off reports whether XO_SELF_DRIVER turns the switch off.
func Off() bool {
	v := os.Getenv("XO_SELF_DRIVER")
	return v == "0" || v == "off" || v == "false"
}

// valueFlags are the flags of build, run, and test (cmd/xo) that take a
// value, for finding the backend and the directories the Go flag package
// would find.
var valueFlags = map[string]bool{
	"o": true, "targets": true, "stamp": true, "seed": true, "explore": true, "record": true, "run": true,
	"cases": true, "backend": true, "tzdata": true,
}

// unsupported are the flags xo1 does not implement yet: commands with
// them stay with the Go driver.
var unsupported = map[string]bool{"ci": true, "hybrid": true, "real": true}

// Wants reports whether cmd with args (after the command, module flags
// such as --offline already taken out) runs through xo1.
func Wants(cmd string, args []string) bool {
	if Off() || (cmd != "build" && cmd != "run" && cmd != "test") {
		return false
	}
	if runtime.GOARCH != "arm64" || (runtime.GOOS != "darwin" && runtime.GOOS != "linux") {
		return false
	}
	backend, noGC, ok := "", false, true
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = args[i+1:]
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			rest = args[i:]
			break
		}
		name, value, has := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(a, "-"), "-"), "=")
		if valueFlags[name] && !has && i+1 < len(args) {
			i++
			value = args[i]
		}
		switch {
		case name == "backend":
			backend = value
		case name == "no-gc":
			noGC = !has || value == "true" || value == "1" || value == "t" || value == "T" || value == "TRUE" || value == "True"
		case unsupported[name]:
			ok = false
		}
	}
	native := backend == "arm64" || backend == "llvm" || (backend == "" && noGC)
	if !native || !ok {
		return false
	}
	if cmd == "test" {
		dirs := rest
		if len(dirs) == 0 {
			dirs = []string{"."}
		}
		if ws := mod.WorkspaceDirs(dirs); strings.Join(ws, "\x00") != strings.Join(dirs, "\x00") {
			return false
		}
	}
	return true
}

// ErrFallback reports that xo1 handed the program back to the Go driver.
var ErrFallback = errors.New("xo1 does not support this program yet")

// Run runs args (the command first) with xo1 on the process's standard
// input and the given outputs, and returns its exit status. ErrFallback
// means the Go driver must run the command; any other error is a failure
// to get xo1, already explained.
func Run(args []string, stdout, stderr io.Writer) (int, error) {
	bin, root, err := Binary()
	if err != nil {
		return 1, fmt.Errorf("the self hosted driver xo1 is not available: %v (set XO_SELF_DRIVER=0 to use the Go driver; decision 0178)", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		return 1, err
	}
	marker, err := os.CreateTemp("", "xo1-fallback-*")
	if err != nil {
		return 1, err
	}
	marker.Close()
	os.Remove(marker.Name())
	defer os.Remove(marker.Name())
	xargs := append([]string{args[0], "--std=" + filepath.Join(root, "std"), "--wd=" + wd}, args[1:]...)
	cmd := exec.Command(bin, xargs...)
	env := append(os.Environ(), "PWD="+wd, "XO1_FALLBACK="+marker.Name())
	if os.Getenv("XO_RTPACK") == "" {
		// the prebuilt runtime beside this xo (decision 0157), not beside xo1
		if r := build.RTPackRoot(); r != "" {
			env = append(env, "XO_RTPACK="+r)
		}
	}
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err = cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ee.ExitCode() == 125 {
			if _, serr := os.Stat(marker.Name()); serr == nil {
				return 0, ErrFallback
			}
		}
		return ee.ExitCode(), nil
	}
	if err != nil {
		return 1, err
	}
	return 0, nil
}

// Binary is the xo1 to run and the directory holding its std and
// third_party: $XO1 (with $XO1_ROOT, or the sources of this xo), else
// the xo1 built from the sources embedded in this xo.
func Binary() (bin, root string, err error) {
	root, err = Sources()
	if err != nil {
		return "", "", err
	}
	if b := os.Getenv("XO1"); b != "" {
		if r := os.Getenv("XO1_ROOT"); r != "" {
			root = r
		}
		if fi, err := os.Stat(b); err != nil || fi.IsDir() {
			return "", "", fmt.Errorf("XO1=%s is not a file", b)
		}
		return b, root, nil
	}
	bin = filepath.Join(root, "bin", "xo1")
	if fi, err := os.Stat(bin); err == nil && fi.Mode().IsRegular() {
		return bin, root, nil
	}
	if err := buildXo1(root, bin); err != nil {
		return "", "", err
	}
	return bin, root, nil
}

// embedded is every embedded source xo1 needs, by its path under the
// root directory.
func embedded() (map[string][]byte, error) {
	files := map[string][]byte{}
	add := func(fsys fs.FS, prefix string) error {
		return fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := fs.ReadFile(fsys, p)
			if err != nil {
				return err
			}
			files[path.Join(prefix, p)] = data
			return nil
		})
	}
	for _, s := range []struct {
		fsys   fs.FS
		prefix string
	}{
		{selfhost.Files, "selfhost"},
		{std.Files, "std"},
		{std.NativeFiles, "std"},
		{std.LLRuntime, "std"},
		{mbedtls.Files, "third_party/mbedtls"},
		{fiatcrypto.Files, "third_party/fiat-crypto"},
	} {
		if err := add(s.fsys, s.prefix); err != nil {
			return nil, err
		}
	}
	return files, nil
}

// Sources writes the embedded sources under <user cache>/xo/xo1/<key>
// (once: a complete tree has its stamp file) and returns that directory.
func Sources() (string, error) {
	files, err := embedded()
	if err != nil {
		return "", err
	}
	var names []string
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	h := sha256.New()
	fmt.Fprintf(h, "xo1 sources v1\n")
	for _, n := range names {
		fmt.Fprintf(h, "%s %d\n", n, len(files[n]))
		h.Write(files[n])
	}
	key := hex.EncodeToString(h.Sum(nil))[:16]
	base := os.Getenv("XO1_CACHE")
	if base == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(cache, "xo", "xo1")
	}
	root := filepath.Join(base, key)
	stamp := filepath.Join(root, "sources.ok")
	if _, err := os.Stat(stamp); err == nil {
		return root, nil
	}
	// written into a fresh directory and renamed into place, so a
	// concurrent xo never sees a partial tree
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(base, key+".tmp")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	for _, n := range names {
		p := filepath.Join(tmp, filepath.FromSlash(n))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(p, files[n], 0o644); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, "sources.ok"), nil, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, root); err != nil {
		// another xo wrote it first
		if _, serr := os.Stat(stamp); serr == nil {
			return root, nil
		}
		return "", err
	}
	return root, nil
}

// buildXo1 builds selfhost/xo of root into bin with this xo's arm64
// backend, once across processes (a file lock; a waiting process finds
// the binary when it gets the lock).
func buildXo1(root, bin string) error {
	unlock, err := lockFile(filepath.Join(root, "build.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	if fi, err := os.Stat(bin); err == nil && fi.Mode().IsRegular() {
		return nil
	}
	fmt.Fprintf(os.Stderr, "xo: building the self hosted driver xo1 (once per xo version, a few minutes) into %s\n", bin)
	tmp := bin + ".tmp"
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		return err
	}
	res, err := build.Build([]string{filepath.Join(root, "selfhost", "xo")}, build.Options{Mode: gen.Build, Backend: build.BackendArm64, Out: tmp})
	if err != nil {
		return fmt.Errorf("building xo1: %v", err)
	}
	if res.Failed {
		var b strings.Builder
		for _, d := range res.Diagnostics {
			fmt.Fprintf(&b, "\n%s:%d:%d: %s %s", d.Span.File, d.Span.Line, d.Span.Col, d.Code, d.Message)
		}
		return fmt.Errorf("building xo1: the check failed:%s", b.String())
	}
	return os.Rename(tmp, bin)
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
