// Package stdgen generates std/rt/std_gen_M.go, the Go declarations of
// the std types of module M for the Go backend's runtime, from the std
// declaration files (std/*.xo): `xo dev dump go --std-types`
// (compiler/gen stdtypes.xo) over a program that uses every std module,
// then gofmt, split by module (each type's Go name starts with its
// module's, Archive_Entry; the prelude's types go to std_gen_prelude.go),
// so pushes that change different std modules do not touch the same
// generated file (2026-10-21). Each file's third line records the hash
// of its module's type declarations (Hashes), which TestStdGenCurrent
// (internal/repotest, -short) checks without running xo; TestStdGenOutput
// (internal/gen) and `stdgen -check` check every file and that no other
// generated file is left. It also generates std/rt/std_sigs_M.go, a
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
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Dir is the generated files' directory in the repository.
const Dir = "std/rt"

// TypesPath is the path of module m's generated type declarations.
func TypesPath(m string) string { return Dir + "/std_gen_" + m + ".go" }

// SigsPath is the path of module m's generated signature checks.
func SigsPath(m string) string { return Dir + "/std_sigs_" + m + ".go" }

// Generated are the generated files present at root (their paths), the
// ones Generate writes and any left from an earlier layout.
func Generated(root string) ([]string, error) {
	var out []string
	for _, pat := range []string{"std_gen*.go", "std_sigs*.go"} {
		ms, err := filepath.Glob(filepath.Join(root, Dir, pat))
		if err != nil {
			return nil, err
		}
		for _, m := range ms {
			out = append(out, Dir+"/"+filepath.Base(m))
		}
	}
	sort.Strings(out)
	return out, nil
}

// hashPrefix starts the line of a std_gen_M.go file that records its
// module's hash.
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

// Hashes is a hash per generated module (the prelude's from
// std/prelude.xo, go.xo, and internal.xo) of the type declarations of
// its std/*.xo file (the lines of each top level struct, enum, interface,
// and type declaration through its closing brace at column 0, comment
// lines left out): what its std_gen_M.go is generated from. A change to a
// function or a comment leaves it unchanged. A type of one module that
// names another module's type is checked through the whole files
// (TestStdGenOutput, stdgen -check), not through this hash.
func Hashes(root string) (map[string]string, error) {
	files, err := filepath.Glob(filepath.Join(root, "std", "*.xo"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	hs := map[string]*bytes.Buffer{}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".xo")
		if strings.HasSuffix(name, "_test") {
			continue
		}
		g := name
		if name == "go" || name == "internal" {
			g = "prelude"
		}
		if hs[g] == nil {
			hs[g] = &bytes.Buffer{}
		}
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		h := hs[g]
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
			return nil, err
		}
	}
	out := map[string]string{}
	for g, h := range hs {
		sum := sha256.Sum256(h.Bytes())
		out[g] = "sha256:" + hex.EncodeToString(sum[:])[:32]
	}
	return out, nil
}

// Recorded is the hash each std_gen_M.go at root records, by module.
func Recorded(root string) (map[string]string, error) {
	ms, err := filepath.Glob(filepath.Join(root, Dir, "std_gen_*.go"))
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, f := range ms {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		m := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "std_gen_"), ".go")
		out[m] = ""
		for _, l := range strings.SplitN(string(data), "\n", 5) {
			if strings.HasPrefix(l, hashPrefix) {
				out[m] = strings.TrimPrefix(l, hashPrefix)
			}
		}
	}
	return out, nil
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
	hashes, err := Hashes(root)
	if err != nil {
		return nil, err
	}
	group := func(name string) string {
		name = strings.TrimPrefix(strings.TrimPrefix(name, "(*"), "*")
		p, _, ok := strings.Cut(name, "_")
		if !ok {
			return "prelude"
		}
		for _, m := range mods {
			if strings.ToLower(p) == m {
				return m
			}
		}
		return "prelude"
	}
	files := map[string][]byte{}
	// the types: each top level declaration (with its doc comment) to its
	// module's file
	lines := strings.SplitN(string(out), "\n", 3)
	if len(lines) < 3 {
		return nil, fmt.Errorf("xo dev dump go --std-types: short output %q", out)
	}
	whole, err := format.Source(out)
	if err != nil {
		return nil, fmt.Errorf("std types: %v", err)
	}
	chunks, err := split(whole, func(d ast.Decl) string {
		switch d := d.(type) {
		case *ast.GenDecl:
			if len(d.Specs) > 0 {
				if ts, ok := d.Specs[0].(*ast.TypeSpec); ok {
					return group(ts.Name.Name)
				}
			}
		case *ast.FuncDecl:
			if d.Recv != nil && len(d.Recv.List) == 1 {
				t := d.Recv.List[0].Type
				if st, ok := t.(*ast.StarExpr); ok {
					t = st.X
				}
				if id, ok := t.(*ast.Ident); ok {
					return group(id.Name)
				}
			}
			return group(d.Name.Name)
		}
		return "prelude"
	})
	if err != nil {
		return nil, err
	}
	for g, body := range chunks {
		var b strings.Builder
		b.WriteString(lines[0] + "\n" + lines[1] + "\n" + hashPrefix + hashes[g] + "\n\npackage rt\n\n")
		if strings.Contains(body, "strings.") {
			b.WriteString("import \"strings\"\n\n")
		}
		b.WriteString(body)
		src, err := format.Source([]byte(b.String()))
		if err != nil {
			return nil, fmt.Errorf("%s: %v", TypesPath(g), err)
		}
		files[TypesPath(g)] = src
	}
	// the signature checks: each `_ T = NAME` line to NAME's module
	byGroup := map[string][]string{}
	var head []string
	inVar := false
	for _, l := range strings.Split(string(sigs), "\n") {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, "var ("):
			inVar = true
		case inVar && t == ")":
			inVar = false
		case inVar && strings.HasPrefix(t, "_ "):
			i := strings.LastIndex(t, "= ")
			if i < 0 {
				return nil, fmt.Errorf("std sigs: %q", t)
			}
			g := group(strings.TrimSpace(t[i+2:]))
			byGroup[g] = append(byGroup[g], t)
		case !inVar && !strings.HasPrefix(t, "package ") && (strings.HasPrefix(t, "//") || t == ""):
			if !strings.HasPrefix(t, "// Code generated") && !strings.HasPrefix(t, "// Regenerate") {
				head = append(head, t)
			}
		}
	}
	for g, specs := range byGroup {
		var b strings.Builder
		b.WriteString(lines[0] + "\n" + lines[1] + "\n\npackage rt\n\n")
		for _, h := range head {
			if h != "" {
				b.WriteString(h + "\n")
			}
		}
		b.WriteString("var (\n" + strings.Join(specs, "\n") + "\n)\n")
		src, err := format.Source([]byte(b.String()))
		if err != nil {
			return nil, fmt.Errorf("%s: %v", SigsPath(g), err)
		}
		files[SigsPath(g)] = src
	}
	return files, nil
}

// split is the top level declarations of the Go file src (with their doc
// comments) grouped by key, in order, each group's text joined by blank
// lines.
func split(src []byte, key func(ast.Decl) string) (map[string]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "std_gen.go", src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, d := range f.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.IMPORT {
			continue
		}
		start := d.Pos()
		switch d := d.(type) {
		case *ast.GenDecl:
			if d.Doc != nil {
				start = d.Doc.Pos()
			}
		case *ast.FuncDecl:
			if d.Doc != nil {
				start = d.Doc.Pos()
			}
		}
		text := string(src[fset.Position(start).Offset:fset.Position(d.End()).Offset])
		k := key(d)
		if out[k] != "" {
			out[k] += "\n\n"
		}
		out[k] += text
	}
	return out, nil
}
