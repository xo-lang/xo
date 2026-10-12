// Package gotool finds the go command the Go backend runs, reads its
// version once per process, and holds the Go versions Xo supports
// (decision 0176): the oldest Go that builds generated programs and
// std/rt (MinGo), and the newest Go the test suite has passed on
// (TestedGo), which is the highest `go` line Xo writes into a go.mod.
package gotool

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// MinGo is the oldest Go release that builds generated programs and
// std/rt: rt uses crypto/pbkdf2, hash/maphash.Comparable, and
// http.Protocols (all Go 1.24). `go vet -stdversion` with a go.mod at
// `go 1.23` reports them; at `go 1.24` rt vets clean for darwin, linux,
// windows, and wasip1 with every rt build tag (decision 0176).
const MinGo = "1.24"

// TestedGo is the newest Go release the suite has passed on, and the
// highest `go` line Xo writes: a newer go command builds with this
// language version until the new release procedure
// (knowledgebase/infrastructure/toolchain.md, "Go versions") raises it.
const TestedGo = "1.26"

// Path finds the go command: $XO_GO, then PATH.
func Path() string {
	if g := os.Getenv("XO_GO"); g != "" {
		return g
	}
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	return "go"
}

// CacheDir is the Go build cache for every go command Xo runs:
// $XO_GOCACHE, else <user cache>/xo/gocache. internal/build trims it to
// a size cap (gotchas/go-build-cache-growth.md).
func CacheDir() string {
	if d := os.Getenv("XO_GOCACHE"); d != "" {
		return d
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "xo", "gocache")
}

// CacheEnv is the GOCACHE entry that routes a go command to CacheDir.
func CacheEnv() string { return "GOCACHE=" + CacheDir() }

var (
	verMu   sync.Mutex
	verSeen = map[string]verResult{}
)

type verResult struct {
	v   string
	err error
}

// Version is the version of the go command at Path, as `go env
// GOVERSION` prints it ("go1.26.1", "go1.27rc1", "devel go1.27-..."). It
// runs the command once per process for each go command path. It runs
// in the temporary directory so a go.mod in the working directory cannot
// make the go command switch toolchains (GOTOOLCHAIN in the environment
// still applies).
func Version() (string, error) {
	path := Path()
	verMu.Lock()
	r, ok := verSeen[path]
	verMu.Unlock()
	if ok {
		return r.v, r.err
	}
	cmd := exec.Command(path, "env", "GOVERSION")
	cmd.Dir = os.TempDir()
	out, err := cmd.Output()
	r = verResult{v: strings.TrimSpace(string(out))}
	if err != nil {
		r.err = fmt.Errorf("%s env GOVERSION: %v", path, err)
	} else if r.v == "" {
		r.err = fmt.Errorf("%s env GOVERSION printed nothing", path)
	}
	verMu.Lock()
	verSeen[path] = r
	verMu.Unlock()
	return r.v, r.err
}

// Parse reads the language version (major and minor) of a Go version
// string: "go1.26.1", "go1.27rc1", "1.24", "devel go1.27-abc Tue ...".
func Parse(s string) (major, minor int, ok bool) {
	s = strings.TrimSpace(s)
	if rest, found := strings.CutPrefix(s, "devel "); found {
		s = rest
	}
	s = strings.TrimPrefix(s, "go")
	maj, rest, found := strings.Cut(s, ".")
	if !found {
		return 0, 0, false
	}
	n := 0
	for n < len(rest) && rest[n] >= '0' && rest[n] <= '9' {
		n++
	}
	a, err1 := strconv.Atoi(maj)
	b, err2 := strconv.Atoi(rest[:n])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return a, b, true
}

// Less reports whether language version a is older than b; versions
// that do not parse compare as not less.
func Less(a, b string) bool {
	am, an, ok1 := Parse(a)
	bm, bn, ok2 := Parse(b)
	if !ok1 || !ok2 {
		return false
	}
	return am < bm || am == bm && an < bn
}

// Line is the `go` line of a go.mod Xo writes, for a go command of
// version v: its language version, but no newer than TestedGo, so a go
// command is never asked for a newer Go than itself (with the default
// GOTOOLCHAIN=auto that would download one) and a new release does not
// change language semantics or GODEBUG defaults before the suite has
// passed on it. An unknown version gives TestedGo.
func Line(v string) string {
	maj, min, ok := Parse(v)
	if !ok {
		return TestedGo
	}
	l := fmt.Sprintf("%d.%d", maj, min)
	if Less(TestedGo, l) {
		return TestedGo
	}
	return l
}

// GoLine is Line of the installed go command. XO_GO_LINE overrides it
// (scripts/go-version-check.sh builds the goldens at MinGo with it).
func GoLine() string {
	if l := os.Getenv("XO_GO_LINE"); l != "" {
		return l
	}
	v, err := Version()
	if err != nil {
		return TestedGo
	}
	return Line(v)
}

// TooOldError is a go command older than MinGo (diagnostic XO1502).
type TooOldError struct {
	Path    string
	Version string
}

func (e *TooOldError) Error() string {
	return fmt.Sprintf("the Go backend needs Go %s or newer, but %s is %s; install a newer Go (https://go.dev/dl), set XO_GO to its go command, or build with --backend=arm64 or llvm", MinGo, e.Path, e.Version)
}

// Check returns a *TooOldError when the go command is older than MinGo.
// A version that cannot be read is not an error here: the build then
// reports the go command's own failure.
func Check() error {
	v, err := Version()
	if err != nil {
		return nil
	}
	if _, _, ok := Parse(v); ok && Less(v, MinGo) {
		return &TooOldError{Path: Path(), Version: v}
	}
	return nil
}
