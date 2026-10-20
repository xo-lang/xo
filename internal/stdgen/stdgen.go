// Package stdgen generates std/rt/std_gen.go, the Go declarations of the
// std types for the Go backend's runtime, from the std declaration files
// (std/*.xo): `xo dev dump go --std-types` (compiler/gen stdtypes.xo)
// over a program that uses every std module, then gofmt. The file's
// second line records Hash, a hash of the type declarations it was
// generated from, which TestStdGenCurrent (internal/repotest, -short)
// checks without running xo; TestStdGenOutput (internal/gen) checks the
// whole file. It also generates std/rt/std_sigs_gen.go (SigsPath), a
// `var _ TYPE = NAME` line for each std function the runtime writes by
// hand, so std/rt stops compiling when one's signature is not the one
// the Go backend calls.
package stdgen

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Path is std_gen.go's path in the repository.
const Path = "std/rt/std_gen.go"

// SigsPath is std_sigs_gen.go's path in the repository.
const SigsPath = "std/rt/std_sigs_gen.go"

// hashPrefix starts the line of std_gen.go that records Hash.
const hashPrefix = "// Declarations: "

// modules are the std modules the generator's program uses: every
// std/*.xo but tests, the prelude (always loaded), and std/go and
// std/internal (not modules a program uses for types).
func modules(root string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(root, "std", "*.xo"))
	if err != nil {
		return nil, err
	}
	var mods []string
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".xo")
		if strings.HasSuffix(name, "_test") || name == "prelude" || name == "go" || name == "internal" {
			continue
		}
		mods = append(mods, name)
	}
	sort.Strings(mods)
	return mods, nil
}

var declStart = regexp.MustCompile(`^(pub )?(struct|enum|interface|type) `)

// Hash is a hash of the type declarations of std/*.xo (the lines of
// each top level struct, enum, interface, and type declaration through
// its closing brace at column 0, comment lines left out), in file name
// order: what std_gen.go is generated from. A change to a function or a
// comment leaves it unchanged.
func Hash(root string) (string, error) {
	files, err := filepath.Glob(filepath.Join(root, "std", "*.xo"))
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	h := sha256.New()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.xo") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "== %s\n", filepath.Base(f))
		in := false
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			l := sc.Text()
			first := false
			if !in && declStart.MatchString(l) {
				in, first = true, true
			}
			if !in {
				continue
			}
			if t := strings.TrimSpace(l); strings.HasPrefix(t, "//") || t == "" {
				continue
			}
			fmt.Fprintln(h, l)
			// the closing brace at column 0, or a one line declaration:
			// `type T struct(Str)`, `type F = ...`, `struct S {}`
			if strings.HasPrefix(l, "}") || first && !strings.HasSuffix(l, "{") {
				in = false
			}
		}
		if err := sc.Err(); err != nil {
			return "", err
		}
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))[:32], nil
}

// Recorded is the hash std_gen.go at root records, "" when it has none.
func Recorded(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, Path))
	if err != nil {
		return "", err
	}
	for _, l := range strings.SplitN(string(data), "\n", 5) {
		if strings.HasPrefix(l, hashPrefix) {
			return strings.TrimPrefix(l, hashPrefix), nil
		}
	}
	return "", nil
}

// Generate returns the generated files for the std at root by their
// paths (Path, SigsPath), made with the xo command xo (its fields: the
// binary and its flags).
func Generate(root string, xo []string) (map[string][]byte, error) {
	mods, err := modules(root)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "xo-stdgen-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	var src strings.Builder
	for _, m := range mods {
		fmt.Fprintf(&src, "use std/%s\n", m)
	}
	src.WriteString("\nfn main() {}\n")
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src.String()), 0o644); err != nil {
		return nil, err
	}
	std := filepath.Join(root, "std")
	dump := func(flag string) ([]byte, error) {
		args := append(append([]string{}, xo[1:]...), "dev", "dump", "go", "--std="+std, flag, dir)
		cmd := exec.Command(xo[0], args...)
		cmd.Env = append(os.Environ(), "XO_STD="+std)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("xo dev dump go %s: %v\n%s", flag, err, stderr.Bytes())
		}
		return out, nil
	}
	out, err := dump("--std-types")
	if err != nil {
		return nil, err
	}
	sigs, err := dump("--std-sigs")
	if err != nil {
		return nil, err
	}
	sigsSrc, err := format.Source(sigs)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", SigsPath, err)
	}
	hash, err := Hash(root)
	if err != nil {
		return nil, err
	}
	// the hash after the generator's two comment lines
	lines := strings.SplitN(string(out), "\n", 3)
	if len(lines) < 3 {
		return nil, fmt.Errorf("xo dev dump go --std-types: short output %q", out)
	}
	text := lines[0] + "\n" + lines[1] + "\n" + hashPrefix + hash + "\n" + lines[2]
	formatted, err := format.Source([]byte(text))
	if err != nil {
		return nil, fmt.Errorf("%s: %v", Path, err)
	}
	return map[string][]byte{Path: formatted, SigsPath: sigsSrc}, nil
}
