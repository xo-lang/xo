package xotool

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// CodegenRoots are the compiler modules that generate code (decision
// 0275): the backends. CodegenKey hashes them and every compiler module
// they use, transitively.
var CodegenRoots = []string{"arm64", "llvm", "wasm", "macho"}

// codegenKeyVersion changes when what CodegenKey hashes changes.
const codegenKeyVersion = "xo codegen key v1"

var versionRe = regexp.MustCompile(`(?m)^pub const VERSION = "([^"]*)"`)

// CodegenClosure is the import closure of CodegenRoots within root's
// compiler module: the directories relative to the compiler directory,
// sorted. An error when a `use` of a module in the closure names
// anything but std or the compiler module, or a module directory has no
// .xo file: the key then fails closed (no stamp, xo keys on its own
// binary).
func CodegenClosure(root string) ([]string, error) {
	cdir := CompilerDir(root)
	base := filepath.Join(root, cdir)
	prefix := "github.com/xo-lang/xo/" + cdir + "/"
	seen := map[string]bool{}
	queue := append([]string(nil), CodegenRoots...)
	for len(queue) > 0 {
		m := queue[0]
		queue = queue[1:]
		if seen[m] {
			continue
		}
		seen[m] = true
		files, err := moduleFiles(filepath.Join(base, m))
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("codegen closure: module %s has no source", m)
		}
		for _, f := range files {
			uses, err := fileUses(f)
			if err != nil {
				return nil, err
			}
			for _, u := range uses {
				switch {
				case u == "std" || strings.HasPrefix(u, "std/"):
				case strings.HasPrefix(u, prefix):
					queue = append(queue, strings.TrimPrefix(u, prefix))
				default:
					return nil, fmt.Errorf("codegen closure: %s uses %q", f, u)
				}
			}
		}
	}
	var out []string
	for m := range seen {
		out = append(out, m)
	}
	sort.Strings(out)
	return out, nil
}

// CodegenKey is XO_CODEGEN_KEY of root (decision 0275): the SHA-256 of
// the files of CodegenClosure (by path and contents, test files left
// out) and core.VERSION. The backend caches key on it instead of xo's
// binary, so they survive an xo rebuilt from changes that leave code
// generation alone. Any error fails closed: the caller sets no stamp.
func CodegenKey(root string) (string, error) {
	mods, err := CodegenClosure(root)
	if err != nil {
		return "", err
	}
	cdir := CompilerDir(root)
	core, err := os.ReadFile(filepath.Join(root, cdir, "cli", "core", "cli.xo"))
	if err != nil {
		return "", err
	}
	v := versionRe.FindSubmatch(core)
	if v == nil {
		return "", fmt.Errorf("codegen key: no VERSION in %s/cli/core/cli.xo", cdir)
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\nversion %s\n", codegenKeyVersion, v[1])
	for _, m := range mods {
		files, err := moduleFiles(filepath.Join(root, cdir, m))
		if err != nil {
			return "", err
		}
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				return "", err
			}
			rel := filepath.ToSlash(m + "/" + filepath.Base(f))
			fmt.Fprintf(h, "%d:%s\n%d\n", len(rel), rel, len(b))
			h.Write(b)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// moduleFiles are the .xo files of a module directory but its tests,
// sorted.
func moduleFiles(dir string) ([]string, error) {
	es, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range es {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".xo") || strings.HasSuffix(n, "_test.xo") {
			continue
		}
		out = append(out, filepath.Join(dir, n))
	}
	sort.Strings(out)
	return out, nil
}

// fileUses are the module paths of a file's `use` lines (a `use go`
// line is returned as "go ...", which no closure accepts).
func fileUses(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(l, "use ") {
			continue
		}
		f := strings.Fields(l)
		if f[1] == "go" {
			out = append(out, "go "+strings.Join(f[2:], " "))
			continue
		}
		out = append(out, f[1])
	}
	return out, sc.Err()
}
