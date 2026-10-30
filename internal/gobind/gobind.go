// Package gobind binds Go packages for `use go "<import path>"` (decision
// 0041, prototype 2026-10-08).
//
// Bind reads a Go package with the standard library only: `go list -export
// -deps -json` finds the compiled export data of the package and its
// dependencies, and go/importer turns it into go/types objects. From them
// it writes two files into a cache directory (CacheDir):
//
//   - <import path>.xo: an Xo declaration file in the bodiless form of
//     std/*.xo (encoding rules at the top of std/prelude.xo), parsed in
//     signature mode and loaded by internal/check as a std like module.
//   - <import path>.go: Go source of package rt (the runtime) holding one
//     adapter per declaration. internal/gen calls the adapters with the std
//     calling convention (`rt.<Prefix><name>(tk, ...)`, `h.M_<name>(tk,
//     ...)`), and internal/build copies the file into the build directory.
//
// The type mapping follows decision 0041: `(T, error)` is `T ! go.Error`;
// a pointer result that may be nil is `T?`; Go types with methods, structs,
// and interfaces are opaque handles; slices and maps are copied at the
// boundary; a leading context.Context parameter is supplied from the
// current task; a Go panic becomes a fault at the boundary. Every function
// carries the `go` effect. Signatures that do not map are skipped and
// listed in the header of the declaration file with the reason.
package gobind

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/importer"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/gotool"
)

// Version is the format version of generated bindings. Cached bindings
// written with another version are regenerated. 2 (2026-10-08, decision
// 0066): callbacks run without the request's arena. 3 (2026-10-08,
// decision 0067): adapters bracket the Go call with rt.GoEnter/GoExit and
// callbacks go through rt.GoCallback, which runs a call on another
// goroutine as a task of its own. 4 (2026-10-08, decision 0073): callback
// arguments are converted before cb.Begin, which takes them (for the
// deterministic scheduler and recordings), and *time.Timer and *time.Ticker
// are handles (time.AfterFunc binds). 5 (2026-10-09, decision 0093): Go
// calls are recorded by `--record` and answered or re executed by `xo
// replay` (rt.GoFxValue, rt.GoFxRerun); declarations carry a `Replay:`
// doc line. 6 (2026-10-27, Intel b15): a comparable struct handle the
// package hands out as a value (netip.Addr) compares and hashes by value.
// 7 (2026-10-27, decision 0392): a type of another package is that
// package's binding's, named through a `use go` line of the declaration
// file, not a copy. 8 (2026-10-29, the Windows tester's bugs 18 and 19):
// a []byte parameter Go writes into and a parameter of an unexported type
// are skipped.
// 9 (2026-10-29, decision 0412): Go's any is go.Any; a handle has its Go
// value both ways (XoGoValue, XoFromGo) for go.any and go.Any.as_handle.
// 10 (2026-10-29, decision 0415): generic functions bind as Xo generics.
// 23 (2026-10-30, decision 0422): a variadic of an unnamed function
// type (Go's functional options) is a List of callbacks.
// 24 (2026-10-30, coverage item 4): a callback or Go function value with
// several results returns them as a tuple.
// 25 (2026-10-30, decision 0421): a Go channel given to Xo is go.Recv[T].
// 26 (2026-10-30, decision 0416 addendum): a handle has as_X for the
// other packages' interfaces its package's signatures name.
// 27 (2026-10-30): a Go constraint interface (a type set) is not bound
// as a type (zap's ObjectMarshalerPtr[T] broke zap's whole binding).
const Version = 27

// Binding is one bound Go package.
type Binding struct {
	GoPath   string // Go import path
	Name     string // Go package name: the default local name in Xo
	Prefix   string // Go name prefix of the adapters in package rt: "Go_net_url_"
	Standard bool   // part of the Go standard library
	// Module and ModVersion name the Go module of a third party package.
	Module     string
	ModVersion string
	DeclPath   string // the declaration file
	Decl       []byte // the declaration file's content
	Adapter    []byte // Go source of the adapters (package rt)
	// Require holds the `path version` requirements of the binding
	// workspace (third party packages only), for the build's go.mod.
	Require []string
	// Sum is the binding workspace's go.sum.
	Sum []byte
	// Skipped maps the Xo name of each skipped declaration ("name" or
	// "Type.name") to the reason.
	Skipped map[string]string
	Stats   Stats
}

// Stats counts what a binding mapped. Functions and methods count the
// exported functions of the package and the method sets of its exported
// types; Foreign counts methods of handle types from other packages that
// the package's signatures reference.
type Stats struct {
	Funcs, FuncsMapped     int
	Methods, MethodsMapped int
	Fields, FieldsMapped   int
	Consts, ConstsMapped   int
	Vars, VarsMapped       int
	Foreign, ForeignMapped int
	// Impls counts the interfaces an Xo program can implement (decision 0416).
	Impls int
	// Reasons counts skipped functions and methods of the package (not
	// fields, constants, variables, or foreign methods) by reason category.
	Reasons map[string]int
}

// Options configure Bind.
type Options struct {
	// Dir is the cache directory (CacheDir).
	Dir string
	// Version pins the Go module version of a third party package
	// (`require go <module> <version>` in xo.mod); empty uses the version
	// already in the binding workspace, or the latest.
	Version string
	// Module is the pinned module Version belongs to (PinFor). It may
	// enclose the package's own module (aws-sdk-go-v2 for service/s3,
	// a module of its own): then Version pins only Module, and the
	// package resolves at a version Go picks beside it.
	Module string
	// Rerun names the functions an xo.mod `replay rerun go <path>
	// [names]` line marks pure (`name` or `Type.method`; "*" for all):
	// they run again in a replay and are not recorded (decision 0093).
	Rerun []string
}

// ReadRerun reads `replay rerun go <Go import path> [names...]` lines of an
// xo.mod: Go functions that a replay runs again instead of answering
// them from the trace (decision 0093). A line without names marks every
// function of the package ("*").
func ReadRerun(xoMod string) map[string][]string {
	data, err := os.ReadFile(xoMod)
	if err != nil {
		return nil
	}
	out := map[string][]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		text, _, _ := strings.Cut(sc.Text(), "//")
		f := strings.Fields(text)
		if len(f) < 4 || f[0] != "replay" || f[1] != "rerun" || f[2] != "go" {
			continue
		}
		names := f[4:]
		if len(names) == 0 {
			names = []string{"*"}
		}
		out[f[3]] = append(out[f[3]], names...)
	}
	return out
}

// rerunKey is the canonical text of a rerun list, as the declaration
// file's `replay-rerun:` header line records it.
func rerunKey(names []string) string {
	s := append([]string(nil), names...)
	sort.Strings(s)
	return strings.Join(s, " ")
}

// CacheDir is the binding cache of the Xo project rooted at root (the
// directory of its xo.mod, or the module directory): $XO_GOBIND_DIR when
// set, else <root>/.xo/gobind. Go and the Xo checker skip dot directories.
func CacheDir(root string) string {
	if d := os.Getenv("XO_GOBIND_DIR"); d != "" {
		return d
	}
	return filepath.Join(root, ".xo", "gobind")
}

// Prefix is the Go name prefix of a binding's declarations in package rt.
func Prefix(goPath string) string { return "Go_" + ident(goPath) + "_" }

// ident turns an import path into a Go identifier fragment.
func ident(p string) string {
	var b strings.Builder
	for _, r := range p {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// cacheBase is the cache path of a binding without its extension. A
// binding made with an xo.mod `replay rerun go` list differs from the plain
// one, so it gets a name of its own: projects that share a cache
// ($XO_GOBIND_DIR) do not overwrite each other's.
func cacheBase(dir, goPath string, rerun []string) string {
	base := filepath.Join(dir, filepath.FromSlash(goPath))
	if len(rerun) > 0 {
		sum := sha256.Sum256([]byte(rerunKey(rerun)))
		base += ".rerun-" + hex.EncodeToString(sum[:4])
	}
	return base
}

func declPath(dir, goPath string, rerun []string) string {
	return cacheBase(dir, goPath, rerun) + ".xo"
}

func adapterPath(dir, goPath string, rerun []string) string {
	return cacheBase(dir, goPath, rerun) + ".go"
}

// workDir is the Go module the binder resolves packages in.
func workDir(dir string) string { return filepath.Join(dir, "_mod") }

// ReadPins reads `require go <module> <version>` lines of an xo.mod.
func ReadPins(xoMod string) map[string]string {
	data, err := os.ReadFile(xoMod)
	if err != nil {
		return nil
	}
	pins := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 4 && f[0] == "require" && f[1] == "go" {
			pins[f[2]] = f[3]
		}
	}
	return pins
}

// PinFor returns the pinned module that provides goPath (longest module
// path prefix) and its version, or "", "".
func PinFor(pins map[string]string, goPath string) (string, string) {
	best, v := "", ""
	for m, ver := range pins {
		if (goPath == m || strings.HasPrefix(goPath, m+"/")) && len(m) > len(best) {
			best, v = m, ver
		}
	}
	return best, v
}

// Load returns the cached binding of goPath, or nil when there is none or
// it is stale (another Version, or another pinned module version).
func Load(goPath string, opts Options) (*Binding, error) {
	dp := declPath(opts.Dir, goPath, opts.Rerun)
	decl, err := os.ReadFile(dp)
	if err != nil {
		return nil, nil
	}
	adapter, err := os.ReadFile(adapterPath(opts.Dir, goPath, opts.Rerun))
	if err != nil {
		return nil, nil
	}
	b := &Binding{GoPath: goPath, Prefix: Prefix(goPath), DeclPath: dp, Decl: decl, Adapter: adapter, Skipped: map[string]string{}}
	version := -1
	rerun := ""
	sc := bufio.NewScanner(bytes.NewReader(decl))
	for sc.Scan() {
		l := sc.Text()
		if !strings.HasPrefix(l, "//") {
			break
		}
		l = strings.TrimSpace(strings.TrimPrefix(l, "//"))
		switch {
		case strings.HasPrefix(l, "xo-bind: v"):
			fmt.Sscanf(l, "xo-bind: v%d", &version)
		case strings.HasPrefix(l, "go-package: "):
			b.Name = strings.TrimPrefix(l, "go-package: ")
		case l == "go-standard: true":
			b.Standard = true
		case strings.HasPrefix(l, "go-module: "):
			f := strings.Fields(strings.TrimPrefix(l, "go-module: "))
			if len(f) == 2 {
				b.Module, b.ModVersion = f[0], f[1]
			}
		case strings.HasPrefix(l, "replay-rerun: "):
			rerun = strings.TrimPrefix(l, "replay-rerun: ")
		case strings.HasPrefix(l, "skip: "):
			name, reason, _ := strings.Cut(strings.TrimPrefix(l, "skip: "), ": ")
			// the line ends with the Go name, " (strconv.FormatComplex)",
			// which Bind's Skipped leaves out: XO1102 reads the same
			// whether the binding was made or loaded
			if i := strings.LastIndex(reason, " ("); i >= 0 && strings.HasSuffix(reason, ")") {
				reason = reason[:i]
			}
			b.Skipped[name] = reason
		}
	}
	if version != Version || b.Name == "" || rerun != rerunKey(opts.Rerun) {
		return nil, nil
	}
	if opts.Version != "" && !b.Standard && (opts.Module == "" || opts.Module == b.Module) && opts.Version != b.ModVersion {
		return nil, nil
	}
	if !b.Standard {
		b.Require, b.Sum = workspaceDeps(opts.Dir)
	}
	return b, nil
}

// workspaceDeps returns the requirements and go.sum of the binding
// workspace.
func workspaceDeps(dir string) ([]string, []byte) {
	wd := workDir(dir)
	data, err := os.ReadFile(filepath.Join(wd, "go.mod"))
	if err != nil {
		return nil, nil
	}
	var req []string
	in := false
	for _, l := range strings.Split(string(data), "\n") {
		l = strings.TrimSpace(l)
		if i := strings.Index(l, "//"); i >= 0 {
			l = strings.TrimSpace(l[:i])
		}
		switch {
		case l == "require (":
			in = true
		case in && l == ")":
			in = false
		case in && l != "":
			req = append(req, l)
		case strings.HasPrefix(l, "require "):
			req = append(req, strings.TrimSpace(strings.TrimPrefix(l, "require ")))
		}
	}
	sum, _ := os.ReadFile(filepath.Join(wd, "go.sum"))
	return req, sum
}

// listed is the part of `go list -json` output the binder reads.
type listed struct {
	ImportPath string
	Name       string
	Export     string
	Standard   bool
	Module     *struct {
		Path    string
		Version string
	}
	Error *struct {
		Err string
	}
}

// Error is a failure to bind a package (diagnostic XO1101).
type Error struct {
	GoPath string
	Msg    string
}

func (e *Error) Error() string { return fmt.Sprintf("cannot bind Go package %q: %s", e.GoPath, e.Msg) }

// goTool finds the go command: $XO_GO, then PATH.
func goTool() string { return gotool.Path() }

func runGo(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command(goTool(), args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=-mod=mod", gotool.CacheEnv())
	defer gotool.HoldCache()()
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.Bytes(), errors.New(msg)
	}
	return out.Bytes(), nil
}

// ensureWorkspace creates the binding workspace module.
func ensureWorkspace(dir string) error {
	wd := workDir(dir)
	if err := os.MkdirAll(wd, 0o755); err != nil {
		return err
	}
	gm := filepath.Join(wd, "go.mod")
	if _, err := os.Stat(gm); err == nil {
		return nil
	}
	return os.WriteFile(gm, []byte(workspaceMod(gotool.GoLine())), 0o644)
}

// workspaceMod is the binding workspace's go.mod for a `go` line: the
// installed go command's language version, at most gotool.TestedGo
// (decision 0176). A fixed line newer than the installed Go would make
// the go command download that Go (GOTOOLCHAIN=auto) or fail
// (GOTOOLCHAIN=local); `go get` raises the line itself when a bound
// module needs a newer one.
func workspaceMod(line string) string {
	return "// Binding workspace of xo bind (internal/gobind). Generated; do not edit.\nmodule xogobind\n\ngo " + line + "\n"
}

func decodeList(data []byte) ([]listed, error) {
	var out []listed
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var l listed
		if err := dec.Decode(&l); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

// thirdParty reports whether an import path names a module outside the
// standard library (its first element has a dot).
func thirdParty(p string) bool {
	first, _, _ := strings.Cut(p, "/")
	return strings.Contains(first, ".")
}

// Bind binds goPath and writes the declaration and adapter files into
// opts.Dir. Third party packages are added to the binding workspace with
// `go get` (which needs the module cache or the network).
func Bind(goPath string, opts Options) (*Binding, error) {
	if goPath == "" || strings.HasPrefix(goPath, ".") || strings.HasPrefix(goPath, "/") {
		return nil, &Error{GoPath: goPath, Msg: "not a Go import path"}
	}
	if err := ensureWorkspace(opts.Dir); err != nil {
		return nil, err
	}
	wd := workDir(opts.Dir)
	if thirdParty(goPath) {
		var gets []string
		switch {
		case opts.Version != "" && opts.Module != "" && opts.Module != goPath:
			// the pin's module first: goPath may be in a nested module
			// of its own, which `go get goPath@Version` would ask for at
			// the enclosing module's version
			gets = append(gets, opts.Module+"@"+opts.Version)
		case opts.Version != "":
			gets = append(gets, goPath+"@"+opts.Version)
		}
		for _, target := range gets {
			if _, err := runGo(wd, "get", target); err != nil {
				return nil, &Error{GoPath: goPath, Msg: firstLine(err.Error())}
			}
		}
		if len(gets) == 0 || opts.Module != "" && opts.Module != goPath {
			if _, err := runGo(wd, "list", "-json=ImportPath", goPath); err != nil {
				if _, err := runGo(wd, "get", goPath); err != nil {
					return nil, &Error{GoPath: goPath, Msg: firstLine(err.Error())}
				}
			}
		}
	}
	data, err := runGo(wd, "list", "-e", "-export", "-deps", "-json=ImportPath,Name,Export,Standard,Module,Error", goPath)
	if err != nil {
		return nil, &Error{GoPath: goPath, Msg: firstLine(err.Error())}
	}
	pkgs, err := decodeList(data)
	if err != nil {
		return nil, err
	}
	exports := map[string]string{}
	var self *listed
	for i := range pkgs {
		p := &pkgs[i]
		if p.ImportPath == goPath {
			self = p
		}
		if p.Export != "" {
			exports[p.ImportPath] = p.Export
		}
	}
	if self == nil {
		return nil, &Error{GoPath: goPath, Msg: "go list did not report the package"}
	}
	if self.Error != nil {
		return nil, &Error{GoPath: goPath, Msg: firstLine(self.Error.Err)}
	}
	for _, p := range pkgs {
		if p.Error != nil {
			return nil, &Error{GoPath: goPath, Msg: fmt.Sprintf("dependency %s: %s", p.ImportPath, firstLine(p.Error.Err))}
		}
	}
	if self.Name == "main" {
		return nil, &Error{GoPath: goPath, Msg: "a main package cannot be imported"}
	}
	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		f, ok := exports[path]
		if !ok {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(f)
	})
	pkg, err := importPkg(imp, goPath)
	if err != nil {
		return nil, err
	}
	b := &Binding{GoPath: goPath, Name: self.Name, Prefix: Prefix(goPath), Standard: self.Standard, Skipped: map[string]string{}}
	if self.Module != nil && !self.Standard {
		b.Module, b.ModVersion = self.Module.Path, self.Module.Version
	}
	g := newGen(b, pkg)
	g.imp = imp
	g.rerunKey = rerunKey(opts.Rerun)
	for _, n := range opts.Rerun {
		if n == "*" {
			g.rerunAll = true
		}
		g.rerun[n] = true
	}
	g.run()
	b.Decl = g.declFile()
	b.Adapter, err = g.adapterFile()
	if err != nil {
		return nil, fmt.Errorf("gobind: adapter for %s: %v%s", goPath, err, adapterLine(b.Adapter, err))
	}
	b.DeclPath = declPath(opts.Dir, goPath, opts.Rerun)
	if err := os.MkdirAll(filepath.Dir(b.DeclPath), 0o755); err != nil {
		return nil, err
	}
	// the adapter first and the declaration last, each renamed into place,
	// so a concurrent Load never sees a partial binding
	if err := writeAtomic(adapterPath(opts.Dir, goPath, opts.Rerun), b.Adapter); err != nil {
		return nil, err
	}
	if err := writeAtomic(b.DeclPath, b.Decl); err != nil {
		return nil, err
	}
	// the bindings this one refers to (decision 0392), bound now so one
	// check finds them all: Go's imports are acyclic, so this ends
	for _, dep := range sortedKeys(g.deps) {
		if _, err := LoadOrBind(dep, Options{Dir: opts.Dir}); err != nil {
			return nil, &Error{GoPath: goPath, Msg: fmt.Sprintf("binding %s, which its types come from: %v", dep, err)}
		}
	}
	if !b.Standard {
		b.Require, b.Sum = workspaceDeps(opts.Dir)
	}
	return b, nil
}

// LoadOrBind returns the cached binding of goPath, binding it when the
// cache has none or it is stale.
func LoadOrBind(goPath string, opts Options) (*Binding, error) {
	if b, err := Load(goPath, opts); b != nil || err != nil {
		return b, err
	}
	return Bind(goPath, opts)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// sortedKeys returns the keys of m in order.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func writeAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
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
	return os.Rename(f.Name(), path)
}

// importPkg is imp.Import, with the importer's panic on export data it
// cannot read (a newer Go's format: go1.26's importer panics with
// "export data version 4 is greater than maximum supported version 2"
// on Go 1.27's) turned into an Error naming the two Go versions, as is an
// error about the export data's version.
func importPkg(imp types.Importer, goPath string) (pkg *types.Package, err error) {
	defer func() {
		if r := recover(); r != nil {
			pkg, err = nil, &Error{GoPath: goPath, Msg: exportDataMsg(fmt.Sprint(r))}
		}
	}()
	pkg, err = imp.Import(goPath)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "export data") {
			msg = exportDataMsg(msg)
		}
		return nil, &Error{GoPath: goPath, Msg: msg}
	}
	return pkg, nil
}

// exportDataMsg says that this helper's Go cannot read the export data
// of the installed go command, and how to get a helper that can.
func exportDataMsg(detail string) string {
	local, err := gotool.Version()
	if err != nil {
		local = "the installed go command"
	}
	return fmt.Sprintf("xo-gobind was built with %s and cannot read the export data of %s (%s); "+
		"build xo-gobind with the installed Go (go build -o xo-gobind ./cmd/xo-gobind in this xo's source at its version) and set XO_GOBIND to it",
		runtime.Version(), local, firstLine(detail))
}

// adapterLine is the adapter's source line a format error names, quoted
// for the error message ("" when the error names none).
func adapterLine(src []byte, err error) string {
	var line int
	if _, e := fmt.Sscanf(err.Error(), "%d:", &line); e != nil || line < 1 {
		return ""
	}
	lines := strings.Split(string(src), "\n")
	if line > len(lines) {
		return ""
	}
	return "\n\t" + strings.TrimSpace(lines[line-1])
}
