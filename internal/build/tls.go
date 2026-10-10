package build

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/xo-lang/xo/third_party/mbedtls"
)

// TLS for the native backends (decision 0105): std/llrt/xo_tls.c on Mbed
// TLS. A native program that uses std/http (its generated code calls the
// server or the client of xo_http.c) is compiled with -DXO_TLS and the
// vendored library's headers and linked with its static archive, which
// is built once per C compiler, sysroot, and sanitizer flavor into
// <user cache>/xo/mbedtls/<key>/ (about 3 s with every core). Other
// programs get xo_tls.c's stubs and link no TLS code.

// tlsSymbols are the runtime functions only a program using std/http
// calls (internal/ssa's lowering of std/native/http.xo).
var tlsSymbols = []string{"xo_cl_do", "xo_srv_new", "xo_tls_"}

// needsTLS reports whether generated code (LLVM IR or assembly) calls a
// std/http runtime function, so its link needs the TLS library.
func needsTLS(code []byte) bool {
	for _, sym := range tlsSymbols {
		if bytes.Contains(code, []byte(sym)) {
			return true
		}
	}
	return false
}

// usesTLS reports whether a link of srcs needs the TLS library: some
// generated source (LLVM IR or assembly) names a std/http runtime
// function.
func usesTLS(srcs []string) bool {
	for _, s := range srcs {
		if !strings.HasSuffix(s, ".ll") && !strings.HasSuffix(s, ".s") {
			continue
		}
		data, err := os.ReadFile(s)
		if err != nil {
			continue
		}
		if needsTLS(data) {
			return true
		}
	}
	return false
}

var (
	tlsMu    sync.Mutex
	tlsBuilt = map[string]string{} // key -> library directory
	tlsHash  string                // of the embedded sources
	ccVers   = map[string]string{} // C compiler -> its --version output
)

// tlsSourceHash hashes the embedded library (its compressed sources) and
// configuration.
func tlsSourceHash() (string, error) {
	if tlsHash != "" {
		return tlsHash, nil
	}
	h := sha256.New()
	err := fs.WalkDir(mbedtls.Files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(mbedtls.Files, p)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s %d\n", p, len(data))
		h.Write(data)
		return nil
	})
	if err != nil {
		return "", err
	}
	tlsHash = hex.EncodeToString(h.Sum(nil))
	return tlsHash, nil
}

// tlsFlavor is the part of a compile's flags the library is built with:
// sanitizers, so an instrumented runtime links an instrumented library.
func tlsFlavor(flags []string) []string {
	var out []string
	for _, f := range flags {
		if strings.HasPrefix(f, "-fsanitize") || strings.HasPrefix(f, "-fno-sanitize") {
			out = append(out, f)
		}
	}
	return out
}

// tlsArgs returns the compile flags (before the sources) and the link
// arguments (after them) of a native link with TLS, building the
// library first when the cache has none for cc, sysroot, and the
// flavor of flags.
func tlsArgs(cc string, flags []string, sysroot string) (compile, link []string, err error) {
	dir, err := tlsLibrary(cc, tlsFlavor(flags), sysroot)
	if err != nil {
		return nil, nil, err
	}
	compile = append(compile, "-DXO_TLS")
	compile = append(compile, mbedtls.Defines...)
	for _, inc := range mbedtls.IncludeDirs {
		compile = append(compile, "-I", filepath.Join(dir, filepath.FromSlash(inc)))
	}
	link = append(link, filepath.Join(dir, "libxotls.a"))
	if runtime.GOOS == "darwin" {
		// the system's trust anchors (xo_tls.c: tls_system_roots)
		link = append(link, "-framework", "Security", "-framework", "CoreFoundation")
	}
	return compile, link, nil
}

// tlsLibrary returns the directory holding the extracted library tree
// and libxotls.a for cc, flavor, and sysroot, building it if needed.
func tlsLibrary(cc string, flavor []string, sysroot string) (string, error) {
	tlsMu.Lock()
	defer tlsMu.Unlock()
	src, err := tlsSourceHash()
	if err != nil {
		return "", err
	}
	ver, ok := ccVers[cc]
	if !ok {
		out, _ := exec.Command(cc, "--version").CombinedOutput()
		ver = string(out)
		ccVers[cc] = ver
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%s\n%s\n%s\n%s/%s\n%s\n", src, cc, ver, sysroot, runtime.GOOS, runtime.GOARCH, strings.Join(flavor, " "))
	key := hex.EncodeToString(h.Sum(nil))[:20]
	if d, ok := tlsBuilt[key]; ok {
		return d, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	root := filepath.Join(base, "xo", "mbedtls")
	if d := os.Getenv("XO_TLS_CACHE"); d != "" {
		root = d
	}
	dir := filepath.Join(root, mbedtls.Version+"-"+key)
	if _, err := os.Stat(filepath.Join(dir, "libxotls.a")); err == nil {
		tlsBuilt[key] = dir
		return dir, nil
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	var rb [6]byte
	rand.Read(rb[:])
	tmp := filepath.Join(root, fmt.Sprintf("tmp-%d-%s", os.Getpid(), hex.EncodeToString(rb[:])))
	defer os.RemoveAll(tmp)
	if err := buildTLSLibrary(tmp, cc, flavor, sysroot); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dir); err != nil {
		// another process built it meanwhile
		if _, err2 := os.Stat(filepath.Join(dir, "libxotls.a")); err2 != nil {
			return "", err
		}
	}
	tlsBuilt[key] = dir
	return dir, nil
}

// buildTLSLibrary extracts the library into dir and compiles it into
// dir/libxotls.a.
func buildTLSLibrary(dir, cc string, flavor []string, sysroot string) error {
	var srcs []string
	excluded := map[string]bool{}
	for _, e := range mbedtls.Exclude {
		excluded[e] = true
	}
	inSourceDir := map[string]bool{}
	for _, d := range mbedtls.SourceDirs {
		inSourceDir[d] = true
	}
	tree, err := mbedtls.Tree()
	if err != nil {
		return err
	}
	for _, f := range tree {
		out := filepath.Join(dir, filepath.FromSlash(f.Name))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(out, f.Data, 0o644); err != nil {
			return err
		}
		if strings.HasSuffix(f.Name, ".c") && inSourceDir[path.Dir(f.Name)] && !excluded[f.Name] {
			srcs = append(srcs, f.Name)
		}
	}
	args := []string{"-c", "-O2", "-w", "-fexceptions", "-funwind-tables"}
	args = append(args, flavor...)
	if sysroot != "" {
		args = append(args, "-isysroot", sysroot)
	}
	args = append(args, mbedtls.Defines...)
	for _, inc := range mbedtls.IncludeDirs {
		args = append(args, "-I", filepath.Join(dir, filepath.FromSlash(inc)))
	}
	objDir := filepath.Join(dir, "obj")
	if err := os.MkdirAll(objDir, 0o755); err != nil {
		return err
	}
	objs := make([]string, len(srcs))
	errs := make([]error, len(srcs))
	work := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				obj := filepath.Join(objDir, strings.ReplaceAll(strings.TrimSuffix(srcs[i], ".c"), "/", "_")+".o")
				a := append(append([]string(nil), args...), "-o", obj, filepath.Join(dir, filepath.FromSlash(srcs[i])))
				out, err := exec.Command(cc, a...).CombinedOutput()
				if err != nil {
					errs[i] = fmt.Errorf("compiling the TLS library (%s): %v\n%s", srcs[i], err, out)
				}
				objs[i] = obj
			}
		}()
	}
	for i := range srcs {
		work <- i
	}
	close(work)
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	ar, err := findAr()
	if err != nil {
		return err
	}
	lib := filepath.Join(dir, "libxotls.a")
	out, err := exec.Command(ar, append([]string{"rcs", lib}, objs...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("archiving the TLS library: %v\n%s", err, out)
	}
	return os.RemoveAll(objDir)
}

// findAr finds an archiver: $XO_AR, ar, llvm-ar.
func findAr() (string, error) {
	if a := os.Getenv("XO_AR"); a != "" {
		return exec.LookPath(a)
	}
	for _, a := range []string{"ar", "llvm-ar"} {
		if p, err := exec.LookPath(a); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("the TLS library of std/http needs an archiver: install ar (binutils) or llvm-ar, or set XO_AR")
}
