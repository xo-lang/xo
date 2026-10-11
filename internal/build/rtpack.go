package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/xo-lang/xo/std"
	"github.com/xo-lang/xo/third_party/mbedtls"
)

// Prebuilt runtime objects (decision 0157). A release of xo ships, next to
// the binary, the C runtime of the arm64 backend (std/llrt) compiled for
// its target: one object per variant (the std parts a program compiles
// in: XO_TIME, XO_FS, XO_STDIN, XO_PROC, and TLS) and the TLS library
// (libxotls.a), with a manifest, rtpack.json, in
//
//	<pack root>/<os>-<arch>/
//
// where the pack root is $XO_RTPACK ("off": never use one), else
// lib/xo/rtpack beside the directory of the xo binary (bin/xo of a release
// archive). A link of the host uses the pack's object when the
// manifest's key equals the key this xo computes from its embedded
// runtime sources, the compile flags of a pack, and the TLS library's
// sources, and the object's SHA-256 is the manifest's; any other link
// (other flags, the LLVM backend's LTO, sanitizers, an edited runtime)
// compiles the runtime locally into the object cache (rtobj.go).

// rtPackFormat is the version of the pack layout and manifest.
const rtPackFormat = 1

// RTPackManifestName is the manifest of a pack directory.
const RTPackManifestName = "rtpack.json"

// RTPackTLSLib is the TLS library of a pack.
const RTPackTLSLib = "libxotls.a"

// RTPackTargets are the targets packs are built for: the arm64 backend's
// (the LLVM backend needs clang anyway and links the runtime as bitcode
// for LTO, which ties it to that clang).
var RTPackTargets = []Target{{"darwin", "arm64"}, {"linux", "arm64"}}

// RTVariant is one compiled form of the runtime: the parts of std a
// program compiles in (IOFlags, timeFlags) and TLS (tlsArgs).
type RTVariant struct {
	Time, FS, Stdin, Proc, TLS bool
}

// RTVariants lists every variant, the plain runtime first.
func RTVariants() []RTVariant {
	var out []RTVariant
	for i := 0; i < 32; i++ {
		out = append(out, RTVariant{Time: i&1 != 0, FS: i&2 != 0, Stdin: i&4 != 0, Proc: i&8 != 0, TLS: i&16 != 0})
	}
	return out
}

// Defines are the variant's -D flags other than TLS's, in the order a
// native build passes them (timeFlags, then IOFlags).
func (v RTVariant) Defines() []string {
	var out []string
	if v.Time {
		out = append(out, "-DXO_TIME")
	}
	if v.FS {
		out = append(out, "-DXO_FS")
	}
	if v.Stdin {
		out = append(out, "-DXO_STDIN")
	}
	if v.Proc {
		out = append(out, "-DXO_PROC")
	}
	return out
}

// File is the name of the variant's object in a pack.
func (v RTVariant) File() string {
	n := "rt"
	for _, p := range []struct {
		on   bool
		name string
	}{{v.Time, "time"}, {v.FS, "fs"}, {v.Stdin, "stdin"}, {v.Proc, "proc"}, {v.TLS, "tls"}} {
		if p.on {
			n += "-" + p.name
		}
	}
	return n + ".o"
}

// rtPackFlags are the compile flags of every object of a pack for t
// (besides the defines, the TLS include directories, -c, and -o). They
// pin what a compiler would otherwise choose by its host: the oldest
// macOS on arm64; on Linux position independent code (the default
// executables are PIE) and the outline atomics Debian's clang uses (LSE
// when the CPU has it, decision 0119).
func rtPackFlags(t Target) []string {
	f := []string{"-O2", "-g0", "-fexceptions", "-funwind-tables"}
	switch t.OS {
	case "darwin":
		f = append(f, "-mmacosx-version-min=11.0")
	case "linux":
		f = append(f, "-fPIC", "-moutline-atomics")
	}
	return f
}

// rtPackTLSFlags are the extra compile flags of the TLS library's files
// (buildTLSLibrary's).
var rtPackTLSFlags = []string{"-w"}

// RTPackKey is the key of a pack for t made by this xo: a hash of the
// runtime sources it embeds, the flags of a pack, its variants, and the
// TLS library's sources and configuration. It leaves out the compiler, so
// a pack made by a release's compiler matches every xo built from the
// same sources.
func RTPackKey(t Target) (string, error) {
	h := sha256.New()
	fmt.Fprintf(h, "xo rtpack %d\ntarget %s\nflags %q\ntls flags %q\n", rtPackFormat, t, rtPackFlags(t), rtPackTLSFlags)
	for _, v := range RTVariants() {
		fmt.Fprintf(h, "variant %s %q %v\n", v.File(), v.Defines(), v.TLS)
	}
	var names []string
	for _, pat := range []string{"llrt/*.c", "llrt/*.h"} {
		m, err := fs.Glob(std.LLRuntime, pat)
		if err != nil {
			return "", err
		}
		names = append(names, m...)
	}
	sort.Strings(names)
	for _, n := range names {
		data, err := fs.ReadFile(std.LLRuntime, n)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "file %s %d\n", n, len(data))
		h.Write(data)
	}
	tlsMu.Lock()
	src, err := tlsSourceHash()
	tlsMu.Unlock()
	if err != nil {
		return "", err
	}
	fmt.Fprintf(h, "tls %s %s %q %q %v\n", mbedtls.Version, src, mbedtls.Defines, mbedtls.IncludeDirs, mbedtls.FileDefines)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// rtPackManifest is rtpack.json.
type rtPackManifest struct {
	Format   int    `json:"format"`
	Target   string `json:"target"`
	Key      string `json:"key"`
	Compiler string `json:"compiler"`
	// Flags are the compile flags of every object (rtPackFlags), for
	// reading: the key covers them.
	Flags []string `json:"flags"`
	// Files maps each object and the TLS library to its SHA-256.
	Files map[string]string `json:"files"`
}

// RTPackRoot is the pack root this xo reads ("" when packs are off or
// there is none): $XO_RTPACK, else lib/xo/rtpack beside the directory of
// the running binary.
func RTPackRoot() string {
	switch d := os.Getenv("XO_RTPACK"); d {
	case "off":
		return ""
	case "":
	default:
		return d
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return filepath.Join(filepath.Dir(filepath.Dir(exe)), "lib", "xo", "rtpack")
}

// rtPack is a pack directory as loaded (loadRTPack).
type rtPack struct {
	dir string
	m   rtPackManifest
	// why is the reason the pack is not used, "" when it is
	why string
	// ok records the files whose checksum matched
	ok map[string]bool
}

var (
	rtPackMu    sync.Mutex
	rtPackCache = map[string]*rtPack{} // pack directory -> pack
)

// loadRTPack loads the pack for t under the pack root; nil when there is
// no root. A pack that cannot be used has its reason in why.
func loadRTPack(t Target) *rtPack {
	root := RTPackRoot()
	if root == "" {
		return nil
	}
	dir := filepath.Join(root, t.OS+"-"+t.Arch)
	rtPackMu.Lock()
	defer rtPackMu.Unlock()
	if p, ok := rtPackCache[dir]; ok {
		return p
	}
	p := &rtPack{dir: dir, ok: map[string]bool{}}
	rtPackCache[dir] = p
	data, err := os.ReadFile(filepath.Join(dir, RTPackManifestName))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			p.why = "no prebuilt runtime for " + t.String() + " in " + root
		} else {
			p.why = err.Error()
		}
		return p
	}
	if err := json.Unmarshal(data, &p.m); err != nil {
		p.why = fmt.Sprintf("%s: %v", filepath.Join(dir, RTPackManifestName), err)
		return p
	}
	key, err := RTPackKey(t)
	switch {
	case err != nil:
		p.why = err.Error()
	case p.m.Format != rtPackFormat:
		p.why = fmt.Sprintf("the prebuilt runtime in %s has format %d, this xo reads %d", dir, p.m.Format, rtPackFormat)
	case p.m.Target != t.String():
		p.why = fmt.Sprintf("the prebuilt runtime in %s is for %s, not %s", dir, p.m.Target, t)
	case p.m.Key != key:
		p.why = fmt.Sprintf("the prebuilt runtime in %s was built from other runtime sources or flags (key %.12s, this xo's %.12s)", dir, p.m.Key, key)
	}
	return p
}

// file returns the path of a file of the pack when its SHA-256 is the
// manifest's.
func (p *rtPack) file(name string) (string, bool) {
	rtPackMu.Lock()
	defer rtPackMu.Unlock()
	if p.why != "" {
		return "", false
	}
	path := filepath.Join(p.dir, name)
	if p.ok[name] {
		return path, true
	}
	want, ok := p.m.Files[name]
	if !ok {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != want {
		p.why = fmt.Sprintf("%s does not match its checksum in %s", path, RTPackManifestName)
		return "", false
	}
	p.ok[name] = true
	return path, true
}

// rtPackVariant returns the variant a native link with flags (the C
// compiler flags given to ccBuild) and TLS compiles, or false when the
// flags are not exactly those of a pack variant: -O2 and the variant's
// defines, nothing else (the LLVM backend's -flto, sanitizers, the
// cflags of `use c`, ...).
func rtPackVariant(flags []string, tls bool) (RTVariant, bool) {
	v := RTVariant{TLS: tls}
	opt := 0
	for _, f := range flags {
		switch f {
		case "-O2":
			opt++
		case "-DXO_TIME":
			v.Time = true
		case "-DXO_FS":
			v.FS = true
		case "-DXO_STDIN":
			v.Stdin = true
		case "-DXO_PROC":
			v.Proc = true
		default:
			return v, false
		}
	}
	return v, opt == 1
}

// shippedRuntime returns the prebuilt runtime object of a link of the
// host with flags (and, when tls is set, the pack's TLS library), or
// false when no usable pack has it.
func shippedRuntime(flags []string, tls bool) (obj, tlsLib string, ok bool) {
	v, ok := rtPackVariant(flags, tls)
	if !ok {
		return "", "", false
	}
	p := loadRTPack(Host())
	if p == nil {
		return "", "", false
	}
	obj, ok = p.file(v.File())
	if !ok {
		return "", "", false
	}
	if tls {
		if tlsLib, ok = p.file(RTPackTLSLib); !ok {
			return "", "", false
		}
	}
	return obj, tlsLib, true
}

// rtPackID identifies the pack a link of the host may use, for the
// binary cache key (a binary linked with the pack's object is not one
// linked with a local compile, though they behave the same): its key,
// or "local".
func rtPackID() string {
	p := loadRTPack(Host())
	if p == nil {
		return "local"
	}
	rtPackMu.Lock()
	defer rtPackMu.Unlock()
	if p.why != "" {
		return "local"
	}
	return p.dir + " " + p.m.Key
}

// RTPackStatus describes the pack this xo uses for the host: its
// directory and compiler, or why none is used.
func RTPackStatus() string {
	if os.Getenv("XO_RTPACK") == "off" {
		return "prebuilt runtime: off (XO_RTPACK=off); the runtime is compiled locally"
	}
	p := loadRTPack(Host())
	if p == nil {
		return "prebuilt runtime: none; the runtime is compiled locally"
	}
	rtPackMu.Lock()
	defer rtPackMu.Unlock()
	if p.why != "" {
		return "prebuilt runtime: not used (" + p.why + "); the runtime is compiled locally"
	}
	return "prebuilt runtime: " + p.dir + " (" + p.m.Compiler + ", key " + p.m.Key[:12] + ")"
}

// rtPackHint is the hint of a failed local compile of the runtime.
func rtPackHint() string {
	why := "packs are off (XO_RTPACK=off)"
	if os.Getenv("XO_RTPACK") != "off" {
		if p := loadRTPack(Host()); p == nil {
			why = "no pack root was found"
		} else {
			rtPackMu.Lock()
			why = p.why
			rtPackMu.Unlock()
		}
	}
	if why == "" {
		why = "the link's flags are not those of a prebuilt variant"
	}
	install := "install clang or gcc (Debian or Ubuntu: apt-get install clang)"
	if runtime.GOOS == "darwin" {
		install = "install the Command Line Tools (xcode-select --install)"
	}
	return "hint: the C runtime (std/llrt) was compiled locally because no prebuilt runtime matched: " + why +
		"; " + install + ", or set XO_RTPACK to the lib/xo/rtpack directory of a release of xo built from the same sources " +
		"(knowledgebase/decisions/0157-prebuilt-runtime-objects.md)"
}

// RTPackOptions configure BuildRTPack.
type RTPackOptions struct {
	// Target is the pack's target (one of RTPackTargets).
	Target Target
	// CC is the C compiler command (words); default the host's C compiler
	// (CC) when Target is the host.
	CC []string
	// AR is the archiver command (words); default findAr.
	AR []string
	// Out is the pack root; the pack is written to Out/<os>-<arch>.
	Out string
	// Jobs is the number of compiles at once (default GOMAXPROCS).
	Jobs int
	// Work is the directory the sources are written to (default a new
	// temporary directory, removed after). The objects do not depend on
	// it (-ffile-prefix-map).
	Work string
	// variants, when set, limits the pack to these variants (tests)
	variants []RTVariant
}

// BuildRTPack compiles a pack: every variant of the runtime and the TLS
// library, with the flags of rtPackFlags, and its manifest. The objects
// and the archive are byte for byte the same for the same compiler,
// archiver, and sources wherever they are built: sources are written to
// a work directory mapped to /xo (-ffile-prefix-map), no debug
// information, archive members in name order with zero times, owners,
// and fixed modes (normalizeAr). It returns the pack directory.
func BuildRTPack(o RTPackOptions) (string, error) {
	ok := false
	for _, t := range RTPackTargets {
		ok = ok || t == o.Target
	}
	if !ok {
		return "", fmt.Errorf("prebuilt runtimes are built for %v, not %s", RTPackTargets, o.Target)
	}
	if len(o.CC) == 0 {
		if o.Target != Host() {
			return "", fmt.Errorf("building the runtime for %s on %s needs a cross compiler (--cc, for example \"zig cc -target aarch64-linux-gnu.2.31\")", o.Target, Host())
		}
		cc, err := CC()
		if err != nil {
			return "", err
		}
		o.CC = []string{cc}
	}
	if len(o.AR) == 0 {
		ar, err := findAr()
		if err != nil {
			return "", err
		}
		o.AR = []string{ar}
	}
	if o.Jobs <= 0 {
		o.Jobs = runtime.GOMAXPROCS(0)
	}
	key, err := RTPackKey(o.Target)
	if err != nil {
		return "", err
	}
	work := o.Work
	if work == "" {
		work, err = os.MkdirTemp("", "xo-rtpack-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(work)
	}
	if work, err = filepath.Abs(work); err != nil {
		return "", err
	}
	// the sources: the runtime and the TLS library's tree
	llrt := filepath.Join(work, "llrt")
	if err := os.MkdirAll(llrt, 0o755); err != nil {
		return "", err
	}
	rt, err := writeRuntime(llrt)
	if err != nil {
		return "", err
	}
	tlsDir := filepath.Join(work, "tls")
	tlsSrcs, err := writeTLSTree(tlsDir)
	if err != nil {
		return "", err
	}
	stage := filepath.Join(work, "pack")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return "", err
	}
	base := append(append([]string(nil), rtPackFlags(o.Target)...), "-ffile-prefix-map="+work+"=/xo")
	// the variants, then the library
	vs := RTVariants()
	if o.variants != nil {
		vs = o.variants
	}
	errs := make([]error, len(vs))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < o.Jobs; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				v := vs[i]
				args := append(append(append([]string(nil), o.CC[1:]...), base...), v.Defines()...)
				if v.TLS {
					args = append(args, "-DXO_TLS")
					args = append(args, mbedtls.Defines...)
					for _, inc := range mbedtls.IncludeDirs {
						args = append(args, "-I", filepath.Join(tlsDir, filepath.FromSlash(inc)))
					}
				}
				args = append(args, "-c", "-o", filepath.Join(stage, v.File()), rt)
				out, err := exec.Command(o.CC[0], args...).CombinedOutput()
				if err != nil {
					errs[i] = fmt.Errorf("compiling the runtime (%s): %v\n%s", v.File(), err, out)
				}
			}
		}()
	}
	for i := range vs {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			return "", e
		}
	}
	lib := filepath.Join(stage, RTPackTLSLib)
	libArgs := append(append([]string{"-c"}, base...), rtPackTLSFlags...)
	if err := compileTLSArchive(tlsDir, o.CC, libArgs, tlsSrcs, o.AR, lib, o.Jobs); err != nil {
		return "", err
	}
	if err := normalizeAr(lib); err != nil {
		return "", err
	}
	// the manifest
	m := rtPackManifest{Format: rtPackFormat, Target: o.Target.String(), Key: key, Flags: rtPackFlags(o.Target), Files: map[string]string{}}
	// zig cc prints its version and then fails on a glibc version in the
	// target, so the first line is read whatever the exit status
	ver, _ := exec.Command(o.CC[0], append(append([]string(nil), o.CC[1:]...), "--version")...).Output()
	m.Compiler = strings.TrimSpace(strings.SplitN(string(ver), "\n", 2)[0])
	ents, err := os.ReadDir(stage)
	if err != nil {
		return "", err
	}
	for _, e := range ents {
		data, err := os.ReadFile(filepath.Join(stage, e.Name()))
		if err != nil {
			return "", err
		}
		sum := sha256.Sum256(data)
		m.Files[e.Name()] = hex.EncodeToString(sum[:])
	}
	data, err := json.MarshalIndent(m, "", "\t")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(stage, RTPackManifestName), append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	dst := filepath.Join(o.Out, o.Target.OS+"-"+o.Target.Arch)
	if err := os.MkdirAll(o.Out, 0o755); err != nil {
		return "", err
	}
	if err := os.RemoveAll(dst); err != nil {
		return "", err
	}
	if err := os.Rename(stage, dst); err != nil {
		// another file system: copy
		if err := copyDir(stage, dst); err != nil {
			return "", err
		}
	}
	return dst, nil
}

// copyDir copies the regular files of src into a new directory dst.
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	ents, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range ents {
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// normalizeAr rewrites the member headers of the ar archive at path with
// zero times, owners, and groups and mode 644, so an archive does not
// depend on who made it when (GNU ar's and llvm-ar's deterministic mode,
// which Apple's ar lacks). The headers have fixed widths, so the symbol
// table's offsets stay valid.
func normalizeAr(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	const magic = "!<arch>\n"
	if !bytes.HasPrefix(data, []byte(magic)) {
		return fmt.Errorf("%s: not an ar archive", path)
	}
	field := func(b []byte, v string) {
		copy(b, v+strings.Repeat(" ", len(b)-len(v)))
	}
	for off := len(magic); off < len(data); {
		if off+60 > len(data) || string(data[off+58:off+60]) != "`\n" {
			return fmt.Errorf("%s: bad member header at %d", path, off)
		}
		h := data[off : off+60]
		size, err := strconv.Atoi(strings.TrimSpace(string(h[48:58])))
		if err != nil {
			return fmt.Errorf("%s: bad member size at %d", path, off)
		}
		field(h[16:28], "0")
		field(h[28:34], "0")
		field(h[34:40], "0")
		field(h[40:48], "644")
		off += 60 + size
		off += off & 1
	}
	return os.WriteFile(path, data, 0o644)
}
