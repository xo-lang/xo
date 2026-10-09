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
	"encoding/json"
	"errors"
	"fmt"
	"go/importer"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Version is the format version of generated bindings. Cached bindings
// written with another version are regenerated. 2 (2026-10-08, decision
// 0066): callbacks run without the request's arena. 3 (2026-10-08,
// decision 0067): adapters bracket the Go call with rt.GoEnter/GoExit and
// callbacks go through rt.GoCallback, which runs a call on another
// goroutine as a task of its own. 4 (2026-10-08, decision 0073): callback
// arguments are converted before cb.Begin, which takes them (for the
// deterministic scheduler and recordings), and *time.Timer and *time.Ticker
// are handles (time.AfterFunc binds).
const Version = 4

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

func declPath(dir, goPath string) string {
	return filepath.Join(dir, filepath.FromSlash(goPath)+".xo")
}

func adapterPath(dir, goPath string) string {
	return filepath.Join(dir, filepath.FromSlash(goPath)+".go")
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

// PinFor returns the pinned version of the module that provides goPath
// (longest module path prefix), or "".
func PinFor(pins map[string]string, goPath string) string {
	best, v := "", ""
	for m, ver := range pins {
		if (goPath == m || strings.HasPrefix(goPath, m+"/")) && len(m) > len(best) {
			best, v = m, ver
		}
	}
	return v
}

// Load returns the cached binding of goPath, or nil when there is none or
// it is stale (another Version, or another pinned module version).
func Load(goPath string, opts Options) (*Binding, error) {
	dp := declPath(opts.Dir, goPath)
	decl, err := os.ReadFile(dp)
	if err != nil {
		return nil, nil
	}
	adapter, err := os.ReadFile(adapterPath(opts.Dir, goPath))
	if err != nil {
		return nil, nil
	}
	b := &Binding{GoPath: goPath, Prefix: Prefix(goPath), DeclPath: dp, Decl: decl, Adapter: adapter, Skipped: map[string]string{}}
	version := -1
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
		case strings.HasPrefix(l, "skip: "):
			name, reason, _ := strings.Cut(strings.TrimPrefix(l, "skip: "), ": ")
			b.Skipped[name] = reason
		}
	}
	if version != Version || b.Name == "" {
		return nil, nil
	}
	if opts.Version != "" && !b.Standard && opts.Version != b.ModVersion {
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
func goTool() string {
	if g := os.Getenv("XO_GO"); g != "" {
		return g
	}
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	return "go"
}

// goCacheDir matches build.GoCacheDir (gobind cannot import build): Xo's own
// capped Go cache, so bindings do not grow the user's default cache.
func goCacheDir() string {
	if d := os.Getenv("XO_GOCACHE"); d != "" {
		return d
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "xo", "gocache")
}

func runGo(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command(goTool(), args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=-mod=mod", "GOCACHE="+goCacheDir())
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
	return os.WriteFile(gm, []byte("// Binding workspace of xo bind (internal/gobind). Generated; do not edit.\nmodule xogobind\n\ngo 1.26\n"), 0o644)
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
		target := goPath
		if opts.Version != "" {
			target += "@" + opts.Version
		} else if _, err := runGo(wd, "list", "-json=ImportPath", goPath); err == nil {
			target = ""
		}
		if target != "" {
			if _, err := runGo(wd, "get", target); err != nil {
				return nil, &Error{GoPath: goPath, Msg: firstLine(err.Error())}
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
	pkg, err := imp.Import(goPath)
	if err != nil {
		return nil, &Error{GoPath: goPath, Msg: err.Error()}
	}
	b := &Binding{GoPath: goPath, Name: self.Name, Prefix: Prefix(goPath), Standard: self.Standard, Skipped: map[string]string{}}
	if self.Module != nil && !self.Standard {
		b.Module, b.ModVersion = self.Module.Path, self.Module.Version
	}
	g := newGen(b, pkg)
	g.run()
	b.Decl = g.declFile()
	b.Adapter, err = g.adapterFile()
	if err != nil {
		return nil, fmt.Errorf("gobind: adapter for %s: %v", goPath, err)
	}
	b.DeclPath = declPath(opts.Dir, goPath)
	if err := os.MkdirAll(filepath.Dir(b.DeclPath), 0o755); err != nil {
		return nil, err
	}
	// the adapter first and the declaration last, each renamed into place,
	// so a concurrent Load never sees a partial binding
	if err := writeAtomic(adapterPath(opts.Dir, goPath), b.Adapter); err != nil {
		return nil, err
	}
	if err := writeAtomic(b.DeclPath, b.Decl); err != nil {
		return nil, err
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
