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

// decl is an Entry with the body needed to follow its calls.
type decl struct {
	Entry
	bare string
	recv string
	body string
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
			for _, l := range lines[bounds[0]+1 : bounds[1]] {
				if mm := ifaceMeth.FindStringSubmatch(l); mm != nil {
					out = append(out, decl{
						Entry: Entry{Module: mod, Symbol: m[1] + "." + mm[1], Kind: "ifmethod"},
						bare:  mm[1], recv: m[1], body: l})
				}
			}
			continue
		}
		if m := methodRe.FindStringSubmatch(head); m != nil {
			recv := genericRe.ReplaceAllString(strings.TrimSpace(m[1]), "")
			out = append(out, decl{Entry: Entry{Module: mod, Symbol: recv + "." + m[2], Kind: "method"},
				bare: m[2], recv: recv, body: body})
			continue
		}
		if m := fnRe.FindStringSubmatch(head); m != nil {
			out = append(out, decl{Entry: Entry{Module: mod, Symbol: m[1], Kind: "fn"},
				bare: m[1], body: body})
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

// corpusText concatenates the .xo files of the corpora, the program
// goldens and the tool's own test programs.
func corpusText(root string) (string, error) {
	var b strings.Builder
	// cmd/xo's test programs are joined in two steps: TestCacheTable reads
	// the literal `"xo", "NAME"` as a cache directory under the user's cache.
	xoDir := filepath.Join("cmd", "xo")
	for _, dir := range []string{
		filepath.Join("internal", "stdtest", "testdata", "corpus"),
		filepath.Join("internal", "llvm", "testdata", "golden"),
		filepath.Join("internal", "arm64", "testdata"),
		filepath.Join("internal", "gen", "testdata"),
		filepath.Join(xoDir, "testdata"),
	} {
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

// Uncovered returns every declaration no corpus or golden reaches, sorted
// by module and symbol, and the count of declarations seen.
func Uncovered(root string) (entries []Entry, total int, err error) {
	paths, err := filepath.Glob(filepath.Join(root, "std", "*.xo"))
	if err != nil {
		return nil, 0, err
	}
	sort.Strings(paths)
	var decls []decl
	for _, p := range paths {
		mod := strings.TrimSuffix(filepath.Base(p), ".xo")
		ds, err := declared(mod, p)
		if err != nil {
			return nil, 0, err
		}
		decls = append(decls, ds...)
	}
	ds, err := declared("prelude", filepath.Join(root, "std", "native", "prelude.xo"))
	if err != nil {
		return nil, 0, err
	}
	decls = append(decls, ds...)

	text, err := corpusText(root)
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
