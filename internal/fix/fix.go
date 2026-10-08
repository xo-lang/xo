// Package fix implements `xo fix`: it applies the machine fixes attached to
// check diagnostics until none apply, and runs the migration rewriters that
// move code written for an older draft or edition to the current one
// (core 13).
package fix

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/format"
)

// MaxRounds bounds the fix loop. Each round checks the sources and applies
// every fix that does not overlap another; a fix that keeps producing new
// diagnostics with fixes stops here instead of looping forever.
const MaxRounds = 25

// Change is one applied rewrite or fix.
type Change struct {
	File  string `json:"file"`
	Line  int    `json:"line"`
	Rule  string `json:"rule"`  // rewriter name, or the diagnostic code of a check fix
	Title string `json:"title"` // what was done
}

// Result is the outcome of Run.
type Result struct {
	Changes []Change `json:"changes"`
	// Files are the files whose contents changed, sorted.
	Files []string `json:"files"`
	// Rounds is the number of check rounds of the fix loop.
	Rounds int `json:"rounds"`
	// Remaining are the error diagnostics left after the last round.
	Remaining []diag.Diagnostic `json:"remaining"`

	sources map[string][]byte // final contents of changed files, by path
	orig    map[string][]byte
}

// Options configures Run.
type Options struct {
	// Rewriters run once over every source file before the fix loop.
	Rewriters []Rewriter
	// DryRun leaves the files on disk unchanged.
	DryRun bool
}

// Run migrates and fixes the modules under dirs. Without DryRun the changed
// files are written back.
func Run(dirs []string, opt Options) (*Result, error) {
	files, err := sourceFiles(dirs)
	if err != nil {
		return nil, err
	}
	res := &Result{sources: map[string][]byte{}, orig: map[string][]byte{}}
	cur := map[string][]byte{}   // absolute path -> contents
	names := map[string]string{} // absolute path -> path as found
	for _, p := range files {
		src, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		cur[abs] = src
		res.orig[abs] = src
		names[abs] = p
	}

	// Migration rewriters, in registry order.
	for _, rw := range opt.Rewriters {
		if rw.Text == nil {
			continue
		}
		for _, abs := range sortedKeys(cur) {
			out, chs := rw.Text(names[abs], cur[abs])
			if len(chs) == 0 {
				continue
			}
			cur[abs] = out
			res.Changes = append(res.Changes, chs...)
		}
	}

	// The fix loop.
	byCode := map[string]string{}
	for _, rw := range opt.Rewriters {
		for _, code := range rw.Codes {
			byCode[code] = rw.Name
		}
	}
	seen := map[string]bool{stateKey(cur): true}
	for res.Rounds < MaxRounds {
		res.Rounds++
		cr, err := check.CheckOverlay(dirs, cur)
		if err != nil {
			return nil, err
		}
		res.Remaining = res.Remaining[:0]
		for _, d := range cr.Diagnostics {
			if d.Severity == diag.Error {
				res.Remaining = append(res.Remaining, d)
			}
		}
		next, chs := applyFixes(cur, cr.Diagnostics, byCode)
		if len(chs) == 0 {
			break
		}
		key := stateKey(next)
		if seen[key] {
			// The fixes undo each other; stop before cycling.
			break
		}
		seen[key] = true
		cur = next
		res.Changes = append(res.Changes, chs...)
	}

	for abs, src := range cur {
		if !bytes.Equal(src, res.orig[abs]) {
			// Inserted text may break the canonical layout; reformat when
			// the file parses (a file that does not is left as rewritten).
			if out, err := format.Source(names[abs], src); err == nil {
				src = out
			}
			res.Files = append(res.Files, names[abs])
			res.sources[abs] = src
		}
	}
	sort.Strings(res.Files)
	sort.SliceStable(res.Changes, func(i, j int) bool {
		a, b := res.Changes[i], res.Changes[j]
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
	if res.Remaining == nil {
		res.Remaining = []diag.Diagnostic{}
	}
	if res.Changes == nil {
		res.Changes = []Change{}
	}
	if res.Files == nil {
		res.Files = []string{}
	}
	if !opt.DryRun {
		for abs, src := range res.sources {
			info, err := os.Stat(abs)
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(abs, src, info.Mode().Perm()); err != nil {
				return nil, err
			}
		}
	}
	return res, nil
}

// applyFixes applies the first fix of every diagnostic whose edits do not
// overlap an edit already taken this round. A fix is applied whole or not at
// all. Diagnostics outside the files in cur (std, bindings) are ignored.
func applyFixes(cur map[string][]byte, ds []diag.Diagnostic, byCode map[string]string) (map[string][]byte, []Change) {
	type span struct{ start, end int }
	taken := map[string][]span{}
	var edits = map[string][]offEdit{}
	var chs []Change
	for _, d := range ds {
		if len(d.Fixes) == 0 || len(d.Fixes[0].Edits) == 0 {
			continue
		}
		f := d.Fixes[0]
		var mine []offEdit
		ok := true
		for _, e := range f.Edits {
			abs, err := filepath.Abs(e.Span.File)
			if err != nil {
				ok = false
				break
			}
			src, in := cur[abs]
			if !in {
				ok = false
				break
			}
			s, e2, valid := offsets(src, e.Span)
			if !valid {
				ok = false
				break
			}
			for _, t := range taken[abs] {
				if overlaps(s, e2, t.start, t.end) {
					ok = false
				}
			}
			for _, m := range mine {
				if m.file == abs && overlaps(s, e2, m.start, m.end) {
					ok = false
				}
			}
			if !ok {
				break
			}
			mine = append(mine, offEdit{file: abs, start: s, end: e2, text: e.Replace})
		}
		if !ok {
			continue
		}
		for _, m := range mine {
			taken[m.file] = append(taken[m.file], span{m.start, m.end})
			edits[m.file] = append(edits[m.file], m)
		}
		rule := d.Code
		if n, ok := byCode[d.Code]; ok {
			rule = n + " (" + d.Code + ")"
		}
		chs = append(chs, Change{File: d.Span.File, Line: d.Span.Line, Rule: rule, Title: f.Title})
	}
	if len(chs) == 0 {
		return cur, nil
	}
	next := make(map[string][]byte, len(cur))
	for k, v := range cur {
		next[k] = v
	}
	for file, es := range edits {
		next[file] = applyOffEdits(cur[file], es)
	}
	return next, chs
}

type offEdit struct {
	file       string
	start, end int
	text       string
}

// overlaps reports whether [a1,a2) and [b1,b2) conflict. Two insertions at
// the same point conflict (their order would be arbitrary); an insertion at
// the boundary of a replacement does not.
func overlaps(a1, a2, b1, b2 int) bool {
	if a1 == a2 && b1 == b2 {
		return a1 == b1
	}
	if a1 == a2 {
		return a1 > b1 && a1 < b2
	}
	if b1 == b2 {
		return b1 > a1 && b1 < a2
	}
	return a1 < b2 && b1 < a2
}

// applyOffEdits applies non-overlapping edits to src.
func applyOffEdits(src []byte, es []offEdit) []byte {
	sort.SliceStable(es, func(i, j int) bool {
		if es[i].start != es[j].start {
			return es[i].start < es[j].start
		}
		return es[i].end < es[j].end
	})
	var b bytes.Buffer
	pos := 0
	for _, e := range es {
		b.Write(src[pos:e.start])
		b.WriteString(e.text)
		pos = e.end
	}
	b.Write(src[pos:])
	return b.Bytes()
}

// offsets converts a diagnostic span (1 based lines, byte columns, end
// exclusive) to byte offsets in src.
func offsets(src []byte, sp diag.Span) (int, int, bool) {
	s, ok := offset(src, sp.Line, sp.Col)
	if !ok {
		return 0, 0, false
	}
	if sp.EndLine == 0 {
		return s, s, true
	}
	e, ok := offset(src, sp.EndLine, sp.EndCol)
	if !ok || e < s {
		return 0, 0, false
	}
	return s, e, true
}

func offset(src []byte, line, col int) (int, bool) {
	if line < 1 || col < 1 {
		return 0, false
	}
	off := 0
	for l := 1; l < line; l++ {
		i := bytes.IndexByte(src[off:], '\n')
		if i < 0 {
			return 0, false
		}
		off += i + 1
	}
	off += col - 1
	if off > len(src) {
		return 0, false
	}
	return off, true
}

// sourceFiles lists the .xo files under dirs, skipping testdata and hidden
// directories below the named ones (as check does).
func sourceFiles(dirs []string) ([]string, error) {
	var out []string
	for _, d := range dirs {
		info, err := os.Stat(d)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%s is not a directory", d)
		}
		err = filepath.WalkDir(d, func(path string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if e.IsDir() {
				if path != d && (e.Name() == "testdata" || strings.HasPrefix(e.Name(), ".")) {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(path, ".xo") {
				out = append(out, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

func sortedKeys(m map[string][]byte) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func stateKey(m map[string][]byte) string {
	var b strings.Builder
	for _, k := range sortedKeys(m) {
		b.WriteString(k)
		b.WriteByte(0)
		b.Write(m[k])
		b.WriteByte(0)
	}
	return b.String()
}
