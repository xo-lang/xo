// Package stdcoverage reports the std API that no corpus or golden
// reaches: every top level function and every method std/*.xo and
// std/native/prelude.xo declare, marked against the call sites under
// internal/stdtest/testdata/corpus (the generated corpora),
// internal/llvm/testdata/golden and internal/arm64/testdata (the program
// goldens), with the calls std's own bodies make followed to the prelude
// helpers the compiler lowers a method to (`Str.contains` runs
// str_contains, per decision 0074).
//
// scripts/stdcoverage prints the list, and internal/repotest's
// TestStdAPICovered fails when a declaration is uncovered and not named in
// allow.txt, so a new std function or method cannot join the API without a
// corpus, a golden, or an allowlist line with the reason it cannot have
// one. The allowlist is matched by symbol, with a trailing * for a family.
package stdcoverage

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed allow.txt
var allowText string

// Entry is one declaration: its module (the std file's base name, or
// "prelude" for std/native/prelude.xo), the symbol it is named by
// (`Type.method` for a method), and whether it is a function, a method or
// an interface method.
type Entry struct {
	Module string
	Symbol string
	Kind   string // "fn", "method" or "ifmethod"
}

// decl is an Entry with the body needed to follow its calls, and the file
// and 1 based line its name starts on (the key of the resolved edges).
type decl struct {
	Entry
	bare string
	recv string
	body string
	path string
	line int
}

var (
	topRe      = regexp.MustCompile(`^(?:pub\s+)?(?:extern\s+)?(?:fn|struct|type|enum|impl|const|use|test|trait|var|let|interface|module)\b`)
	fnRe       = regexp.MustCompile(`^(?:pub\s+)?fn\s+([a-z_][A-Za-z0-9_]*)\s*[(<]`)
	methodRe   = regexp.MustCompile(`^fn\s*\(\s*(?:var\s+)?[A-Za-z_][A-Za-z0-9_]*\s*:\s*([A-Za-z0-9_\[\]?, ]+?)\s*\)\s+([a-z_][A-Za-z0-9_]*)`)
	ifaceRe    = regexp.MustCompile(`^(?:pub\s+)?interface\s+([A-Za-z_][A-Za-z0-9_]*)`)
	ifaceMeth  = regexp.MustCompile(`^\s+fn\s+([a-z_][A-Za-z0-9_]*)\s*\(`)
	genericRe  = regexp.MustCompile(`\[.*`)
	awaitRe    = regexp.MustCompile(`\.await\b`)
	identChars = func(c byte) bool {
		return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
	}
)

// declared reads the declarations of one file.
func declared(mod, path string) ([]decl, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	starts := []int{}
	for i, l := range lines {
		if l != "" && l[0] != ' ' && l[0] != '\t' && topRe.MatchString(l) {
			starts = append(starts, i)
		}
	}
	starts = append(starts, len(lines))
	var out []decl
	for _, bounds := range pairs(starts) {
		head := lines[bounds[0]]
		body := strings.Join(lines[bounds[0]:bounds[1]], "\n")
		if m := ifaceRe.FindStringSubmatch(head); m != nil {
			// The interface's methods are the indented `fn name(` lines of
			// its block, each a signature with no body.
			for i, l := range lines[bounds[0]+1 : bounds[1]] {
				if mm := ifaceMeth.FindStringSubmatch(l); mm != nil {
					out = append(out, decl{
						Entry: Entry{Module: mod, Symbol: m[1] + "." + mm[1], Kind: "ifmethod"},
						bare:  mm[1], recv: m[1], body: l, path: path, line: bounds[0] + i + 2})
				}
			}
			continue
		}
		if m := methodRe.FindStringSubmatch(head); m != nil {
			recv := genericRe.ReplaceAllString(strings.TrimSpace(m[1]), "")
			out = append(out, decl{Entry: Entry{Module: mod, Symbol: recv + "." + m[2], Kind: "method"},
				bare: m[2], recv: recv, body: body, path: path, line: bounds[0] + 1})
			continue
		}
		if m := fnRe.FindStringSubmatch(head); m != nil {
			out = append(out, decl{Entry: Entry{Module: mod, Symbol: m[1], Kind: "fn"},
				bare: m[1], body: body, path: path, line: bounds[0] + 1})
		}
	}
	return out, nil
}

// pairs turns the boundary list into [start, end) pairs.
func pairs(bounds []int) [][2]int {
	out := make([][2]int, 0, len(bounds)-1)
	for i := 0; i+1 < len(bounds); i++ {
		out = append(out, [2]int{bounds[i], bounds[i+1]})
	}
	return out
}

// callsIn returns the bare and qualified names a body calls: an identifier
// followed, after blanks and any type arguments, by `(`.
func callsIn(body string) map[string]bool {
	out := map[string]bool{}
	n := len(body)
	for i := 0; i < n; i++ {
		if !identChars(body[i]) || body[i] >= '0' && body[i] <= '9' {
			continue
		}
		start := i
		for i < n && identChars(body[i]) {
			i++
		}
		name := body[start:i]
		k := i
		for k < n && (body[k] == ' ' || body[k] == '\t') {
			k++
		}
		for k < n && body[k] == '[' {
			depth := 0
			ok := false
			for k < n {
				switch body[k] {
				case '[':
					depth++
				case ']':
					depth--
					if depth == 0 {
						k++
						ok = true
					}
				case '\n':
					if depth > 0 {
						ok = false
						depth = -1
					}
				}
				if ok || depth < 0 {
					break
				}
				k++
			}
			if !ok {
				break
			}
			for k < n && (body[k] == ' ' || body[k] == '\t') {
				k++
			}
		}
		if k >= n || body[k] != '(' {
			i--
			continue
		}
		out[name] = true
		p := start - 1
		for p >= 0 && (body[p] == ' ' || body[p] == '\t') {
			p--
		}
		if p >= 0 && body[p] == '.' {
			q := p - 1
			for q >= 0 && (body[q] == ' ' || body[q] == '\t') {
				q--
			}
			r := q
			for r >= 0 && identChars(body[r]) {
				r--
			}
			if r < q {
				out[body[r+1:q+1]+"."+name] = true
			}
		}
		i--
	}
	return out
}

// corpusDirs are the roots of the corpora, the program goldens and the
// tool's own test programs.
func corpusDirs() []string {
	// cmd/xo's test programs are joined in two steps: TestCacheTable reads
	// the literal `"xo", "NAME"` as a cache directory under the user's cache.
	xoDir := filepath.Join("cmd", "xo")
	return []string{
		filepath.Join("internal", "stdtest", "testdata", "corpus"),
		filepath.Join("internal", "llvm", "testdata", "golden"),
		filepath.Join("internal", "arm64", "testdata"),
		filepath.Join("internal", "gen", "testdata"),
		filepath.Join(xoDir, "testdata"),
	}
}

// corpusText concatenates the .xo files of the corpora, the program
// goldens and the tool's own test programs.
func corpusText(root string) (string, error) {
	return corpusTextIn(root, corpusDirs())
}

// CorpusTextIn is the text UncoveredIn scans: every .xo file under dirs
// (paths relative to root).
func CorpusTextIn(root string, dirs []string) (string, error) {
	return corpusTextIn(root, dirs)
}

// corpusTextIn concatenates the .xo files under the given directories.
func corpusTextIn(root string, dirs []string) (string, error) {
	var b strings.Builder
	for _, dir := range dirs {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(p, ".xo") {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			b.Write(data)
			b.WriteByte('\n')
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

// loadDecls reads every declaration Uncovered reports on: the top level
// functions and methods of std/*.xo and std/native/prelude.xo.
func loadDecls(root string) ([]decl, error) {
	paths, err := filepath.Glob(filepath.Join(root, "std", "*.xo"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var decls []decl
	for _, p := range paths {
		mod := strings.TrimSuffix(filepath.Base(p), ".xo")
		ds, err := declared(mod, p)
		if err != nil {
			return nil, err
		}
		for i := range ds {
			ds[i].path = relPath(root, p)
		}
		decls = append(decls, ds...)
	}
	ds, err := declared("prelude", filepath.Join(root, "std", "native", "prelude.xo"))
	if err != nil {
		return nil, err
	}
	for i := range ds {
		ds[i].path = relPath(root, filepath.Join(root, "std", "native", "prelude.xo"))
	}
	return append(decls, ds...), nil
}

// relPath is p as the checker names it in `xo dev edges`: relative to the
// repository root, slash separated.
func relPath(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(r)
}

// Uncovered returns every declaration no corpus or golden reaches, sorted
// by module and symbol, and the count of declarations seen.
func Uncovered(root string) (entries []Entry, total int, err error) {
	text, err := corpusText(root)
	if err != nil {
		return nil, 0, err
	}
	return UncoveredIn(root, text)
}

// UncoveredIn is Uncovered over the given corpus text, for a report that
// must compare like with like: the resolved edges can only cover the
// programs they were run on.
func UncoveredIn(root, text string) (entries []Entry, total int, err error) {
	decls, err := loadDecls(root)
	if err != nil {
		return nil, 0, err
	}
	calls := callsIn(text)

	// The compiler lowers a prelude method to lower(Type)_name (decision
	// 0074), so a covered Type.name covers that prelude function too.
	fnBySymbol := map[string][]int{}
	for i, d := range decls {
		if d.Module == "prelude" && d.Kind == "fn" {
			fnBySymbol[d.Symbol] = append(fnBySymbol[d.Symbol], i)
		}
	}
	byName := map[string][]int{}
	for i, d := range decls {
		byName[d.bare] = append(byName[d.bare], i)
		byName[d.Symbol] = append(byName[d.Symbol], i)
	}
	covered := make([]bool, len(decls))
	queue := []int{}
	mark := func(i int) {
		if !covered[i] {
			covered[i] = true
			queue = append(queue, i)
		}
	}
	for i, d := range decls {
		switch {
		case calls[d.Symbol], calls[d.bare]:
			mark(i)
		case d.Kind == "ifmethod" && calls[strings.ToLower(d.recv)+"."+d.bare]:
			mark(i)
		case d.bare == "await" && awaitRe.MatchString(text):
			mark(i)
		}
	}
	for len(queue) > 0 {
		i := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		d := decls[i]
		if d.Kind == "method" || d.Kind == "ifmethod" {
			for _, j := range fnBySymbol[strings.ToLower(d.recv)+"_"+d.bare] {
				mark(j)
			}
		}
		for c := range callsIn(d.body) {
			for _, j := range byName[c] {
				mark(j)
			}
			if dot := strings.LastIndexByte(c, '.'); dot >= 0 {
				for _, j := range byName[c[dot+1:]] {
					mark(j)
				}
			}
		}
	}
	for i, d := range decls {
		if !covered[i] {
			entries = append(entries, d.Entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Module != entries[j].Module {
			return entries[i].Module < entries[j].Module
		}
		return entries[i].Symbol < entries[j].Symbol
	})
	return entries, len(decls), nil
}

// Edge is one resolved reference, as `xo dev edges` writes it: the
// declaration whose body holds the site (SrcPath and SrcLine; "" and 0
// outside a function), the declaration the site names (TgtPath, TgtLine,
// TgtName), and the site itself (SitePath).
type Edge struct {
	SrcPath  string
	SrcLine  int
	TgtPath  string
	TgtLine  int
	TgtName  string
	SitePath string
}

// ReadEdges reads the edge files `xo dev edges --out=FILE` wrote.
func ReadEdges(paths []string) ([]Edge, error) {
	var out []Edge
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			if line == "" {
				continue
			}
			// SRC_PATH SRC_LINE SRC_NAME TGT_PATH TGT_LINE TGT_NAME
			// SITE_PATH SITE_LINE SITE_COL SITE_NAME
			f := strings.Split(line, "\t")
			if len(f) != 10 {
				return nil, fmt.Errorf("%s: %d fields, want 10: %q", p, len(f), line)
			}
			srcLine, err := strconv.Atoi(f[1])
			if err != nil {
				return nil, fmt.Errorf("%s: src line: %v", p, err)
			}
			tgtLine, err := strconv.Atoi(f[4])
			if err != nil {
				return nil, fmt.Errorf("%s: target line: %v", p, err)
			}
			out = append(out, Edge{SrcPath: f[0], SrcLine: srcLine, TgtPath: f[3], TgtLine: tgtLine, TgtName: f[5], SitePath: f[6]})
		}
	}
	return out, nil
}

// posKey is the identity of a declaration in the resolved edges: its file
// as the checker names it and its line.
func posKey(path string, line int) string { return path + ":" + strconv.Itoa(line) }

// isStdPath reports whether a checker path names a std file (the corpora,
// the goldens, and the tool's own test programs are otherwise).
func isStdPath(p string) bool { return strings.HasPrefix(p, "std/") }

// UncoveredResolved returns every declaration the resolved edges do not
// reach, the count of declarations seen, and those declarations' names by
// the path:line key the edges use (for a report that says where a
// declaration the text scan covered is really unreached).
//
// It is Uncovered with the checker's resolution in place of the name
// match: a root is an edge whose site is not in a std file (a corpus, a
// golden or a tool test program), and a declaration is covered when an
// edge from a root or from a covered declaration names it. The name based
// rules Uncovered keeps for what no resolution can see stay in force: a
// method covers the prelude function the compiler lowers it to
// (decision 0074), and `.await` covers await.
//
// A method that implements an interface method is reached through the
// interface, and the checker's edge names the interface method, so the
// implementation stays uncovered until an implementation edge is added
// (`xo dev edges --impls`): Impls says which, for the report.
func UncoveredResolved(root string, edges []Edge) (entries []Entry, total int, impls map[string]bool, err error) {
	decls, err := loadDecls(root)
	if err != nil {
		return nil, 0, nil, err
	}
	byKey := map[string]int{}
	for i, d := range decls {
		k := posKey(d.path, d.line)
		if _, ok := byKey[k]; !ok {
			byKey[k] = i
		}
	}
	bySrc := map[string][]Edge{}
	for _, e := range edges {
		if e.SrcPath == "" {
			continue
		}
		bySrc[posKey(e.SrcPath, e.SrcLine)] = append(bySrc[posKey(e.SrcPath, e.SrcLine)], e)
	}
	covered := make([]bool, len(decls))
	queue := []int{}
	mark := func(i int) {
		if !covered[i] {
			covered[i] = true
			queue = append(queue, i)
		}
	}
	for _, e := range edges {
		if isStdPath(e.SitePath) {
			continue
		}
		if i, ok := byKey[posKey(e.TgtPath, e.TgtLine)]; ok {
			mark(i)
		}
	}
	// decision 0074: a covered method covers the prelude function the
	// compiler lowers it to
	fnBySymbol := map[string][]int{}
	ifaceByName := map[string][]int{}
	for i, d := range decls {
		if d.Module == "prelude" && d.Kind == "fn" {
			fnBySymbol[d.Symbol] = append(fnBySymbol[d.Symbol], i)
		}
		if d.Kind == "ifmethod" {
			ifaceByName[d.bare] = append(ifaceByName[d.bare], i)
		}
	}
	text, err := corpusText(root)
	if err != nil {
		return nil, 0, nil, err
	}
	for i, d := range decls {
		if d.bare == "await" && awaitRe.MatchString(text) {
			mark(i)
		}
	}
	for len(queue) > 0 {
		i := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		d := decls[i]
		if d.Kind == "method" || d.Kind == "ifmethod" {
			for _, j := range fnBySymbol[strings.ToLower(d.recv)+"_"+d.bare] {
				mark(j)
			}
		}
		for _, e := range bySrc[posKey(d.path, d.line)] {
			if j, ok := byKey[posKey(e.TgtPath, e.TgtLine)]; ok {
				mark(j)
			}
		}
	}
	impls = map[string]bool{}
	for i, d := range decls {
		if covered[i] {
			continue
		}
		entries = append(entries, d.Entry)
		if d.Kind != "method" {
			continue
		}
		// a method whose name a covered interface method has: possibly its
		// implementation (the report counts these apart)
		for _, j := range ifaceByName[d.bare] {
			if covered[j] {
				impls[d.Module+"."+d.Symbol+" ("+d.Kind+")"] = true
				break
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Module != entries[j].Module {
			return entries[i].Module < entries[j].Module
		}
		return entries[i].Symbol < entries[j].Symbol
	})
	return entries, len(decls), impls, nil
}

// Rule is one allowlist line: a symbol (with a trailing * for a family) and
// why it cannot have a corpus.
type Rule struct {
	Pattern string
	Reason  string
}

// Match reports whether an entry is allowed.
func (r Rule) Match(e Entry) bool {
	sym := e.Module + "." + e.Symbol
	if strings.HasSuffix(r.Pattern, "*") {
		return strings.HasPrefix(sym, strings.TrimSuffix(r.Pattern, "*"))
	}
	return sym == r.Pattern
}

// ParseAllow reads the allowlist: a `#` comment, a blank line, or
// `module.Symbol<whitespace>reason`.
func ParseAllow(text string) ([]Rule, error) {
	var out []Rule
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("allow.txt line %d has no reason: %q", i+1, line)
		}
		out = append(out, Rule{Pattern: fields[0], Reason: strings.Join(fields[1:], " ")})
	}
	return out, nil
}

// Allow returns the embedded allowlist.
func Allow() ([]Rule, error) { return ParseAllow(allowText) }
