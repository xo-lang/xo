// Package repotest holds the checks of the repository's own files: the
// decision index, the spec's code tables against the code table xo
// reports from (compiler/cli/core/explain_data.xo), and the golden
// scripts. They lived in internal/diag until decision 0215 deleted it
// with the Go implementation of the compiler; the package has no Go code.
package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const root = "../.."

// TestDecisionNumbersUnique keeps knowledgebase/decisions free of two
// decisions with one number (sessions working in parallel each took the
// next free number twice, 0132 and 0133 on 2026-10-10), and checks that
// the folder index lists every decision once.
func TestDecisionNumbersUnique(t *testing.T) {
	dir := filepath.Join(root, "knowledgebase", "decisions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	name := regexp.MustCompile(`^(\d{4})-[a-z0-9-]+\.md$`)
	byNum := map[string]string{}
	for _, e := range entries {
		if e.Name() == "README.md" {
			continue
		}
		m := name.FindStringSubmatch(e.Name())
		if m == nil {
			t.Errorf("%s: decision file names are NNNN-short-title.md", e.Name())
			continue
		}
		if prev, ok := byNum[m[1]]; ok {
			t.Errorf("decision number %s is used twice: %s and %s; renumber the newer one to the next free number and update its references", m[1], prev, e.Name())
		}
		byNum[m[1]] = e.Name()
		if n := strings.Count(string(index), "("+e.Name()+")"); n != 1 {
			t.Errorf("%s is linked %d times in decisions/README.md, want 1", e.Name(), n)
		}
	}
}

// TestScriptsReadWantBytes keeps the golden harness scripts reading
// want.txt without newline translation. Python's text mode open turns a
// carriage return into "\n", so a golden whose output holds a raw "\r"
// (show_table prints '\r') never matched the program's output
// (gotchas/python-text-mode-want-cr.md).
func TestScriptsReadWantBytes(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(root, "scripts", "*.py"))
	if err != nil {
		t.Fatal(err)
	}
	read := regexp.MustCompile(`open\([^\n]*'want\.txt'\)[^\n]*`)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			m := read.FindString(line)
			if m != "" && !strings.Contains(m, "newline=''") && !strings.Contains(m, "'rb'") {
				t.Errorf("%s: %q reads want.txt in text mode, which turns \\r into \\n; pass newline='' (or 'rb')", filepath.Base(f), strings.TrimSpace(line))
			}
		}
	}
}

// explainRow is one code of compiler/cli/core/explain_data.xo, the table
// `xo explain` prints and every diagnostic's severity comes from.
var explainRow = regexp.MustCompile(`(?m)^    "(XO\d{4})" => Some\(Expl\{title: "((?:[^"\\]|\\.)*)", severity: "(\w+)"`)

// explainCodes is code -> severity of explain_data.xo.
func explainCodes(t *testing.T) (map[string]string, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "compiler", "cli", "core", "explain_data.xo"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, m := range explainRow.FindAllStringSubmatch(string(data), -1) {
		if _, dup := out[m[1]]; dup {
			t.Errorf("explain_data.xo has %s twice", m[1])
		}
		out[m[1]] = m[3]
	}
	if len(out) < 100 {
		t.Fatalf("found only %d codes in explain_data.xo", len(out))
	}
	return out, string(data)
}

// specSection returns the text of the core.md section that starts with
// heading, up to the next `### ` or `## ` heading.
func specSection(t *testing.T, heading string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "spec", "core.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	i := strings.Index(s, "\n"+heading+"\n")
	if i < 0 {
		t.Fatalf("core.md has no heading %q", heading)
	}
	s = s[i+len(heading)+2:]
	if j := regexp.MustCompile(`\n##+ `).FindStringIndex(s); j != nil {
		s = s[:j[0]]
	}
	return s
}

// TestSpecListsEveryCode keeps the code table of core 10.6 equal to the
// code table xo reports from (explain_data.xo): every code listed once,
// with its severity, and no code in the spec the table does not have.
func TestSpecListsEveryCode(t *testing.T) {
	sec := specSection(t, "### 10.6 Diagnostic codes")
	row := regexp.MustCompile("(?m)^\\| `(XO\\d{4})` \\| (\\w+) \\| ([^|]+) \\|")
	listed := map[string]string{}
	for _, m := range row.FindAllStringSubmatch(sec, -1) {
		if _, dup := listed[m[1]]; dup {
			t.Errorf("core 10.6 lists %s twice", m[1])
		}
		listed[m[1]] = m[2]
	}
	codes, _ := explainCodes(t)
	for c, sev := range codes {
		got, ok := listed[c]
		if !ok {
			t.Errorf("%s is not listed in core 10.6", c)
			continue
		}
		if got != sev {
			t.Errorf("core 10.6 gives %s severity %s, xo reports %s", c, got, sev)
		}
		delete(listed, c)
	}
	for c := range listed {
		t.Errorf("core 10.6 lists %s, which explain_data.xo does not define", c)
	}
}

// TestExplanationsNoEmDash keeps the explanation texts free of em-dashes
// (the owner's writing rule).
func TestExplanationsNoEmDash(t *testing.T) {
	_, text := explainCodes(t)
	for i, l := range strings.Split(text, "\n") {
		if strings.ContainsRune(l, '—') {
			t.Errorf("explain_data.xo:%d contains an em-dash", i+1)
		}
	}
}

// TestSpecListsEveryFaultCode keeps the fault code table of core 10.6
// equal to the `XO-Fnnn` codes the runtimes (std/rt, std/llrt,
// std/native) raise.
func TestSpecListsEveryFaultCode(t *testing.T) {
	sec := specSection(t, "### 10.6 Diagnostic codes")
	listed := map[string]bool{}
	for _, m := range regexp.MustCompile("(?m)^\\| `(XO-F\\d{3})` \\|").FindAllStringSubmatch(sec, -1) {
		listed[m[1]] = true
	}
	used := map[string]bool{}
	fault := regexp.MustCompile(`XO-F\d{3}`)
	// the runtimes: the Go backend's, the C runtime, and the native
	// runtime in Xo (std/native, whose llrt/ holds the ports of the C)
	for _, dir := range []string{filepath.Join(root, "std", "rt"), filepath.Join(root, "std", "llrt"), filepath.Join(root, "std", "native")} {
		err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(p, "_test.go") || strings.HasSuffix(p, "_test.xo") || !(strings.HasSuffix(p, ".go") || strings.HasSuffix(p, ".c") || strings.HasSuffix(p, ".h") || strings.HasSuffix(p, ".xo")) {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for _, c := range fault.FindAllString(string(data), -1) {
				used[c] = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(used) == 0 {
		t.Fatal("no fault codes found in the runtimes")
	}
	for c := range used {
		if !listed[c] {
			t.Errorf("the runtime raises %s, which core 10.6 does not list", c)
		}
	}
	for c := range listed {
		if !used[c] {
			t.Errorf("core 10.6 lists %s, which no runtime raises", c)
		}
	}
}

// TestStaticWords keeps the program's 64 static words (rt.static_word,
// std/native/rt.xo) from being taken twice: sessions working in parallel
// each claimed words in 14 to 23 on 2026-10-14 (fault.xo, tzhost.xo,
// ctx.xo, heapos.xo), and a collision corrupts both modules' state at
// run time with no build error. rt.xo's doc lists one range per module;
// every rt.static_word(K) of a module (K a literal or a const of the
// same file) must fall in that module's range, and no two ranges may
// overlap.
func TestStaticWords(t *testing.T) {
	rtSrc, err := os.ReadFile(filepath.Join(root, "std", "native", "rt.xo"))
	if err != nil {
		t.Fatal(err)
	}
	type span struct{ lo, hi int }
	ranges := map[string]span{}
	// the words std may claim: the table's 256 (decision 0237; the 0.17.0
	// seed's SSA builder takes words past 63, the 0.16.0 seed's did not)
	const seedWords = 256
	owner := make([]string, seedWords)
	entry := regexp.MustCompile(`(?m)^///\s+(\d+)(?:-(\d+))?\s+(std/native/\S+\.xo):`)
	for _, m := range entry.FindAllStringSubmatch(string(rtSrc), -1) {
		lo, _ := strconv.Atoi(m[1])
		hi := lo
		if m[2] != "" {
			hi, _ = strconv.Atoi(m[2])
		}
		if lo > hi || hi >= seedWords {
			t.Errorf("rt.xo: static words %d-%d of %s: not a range in 0 to %d (decision 0237)", lo, hi, m[3], seedWords-1)
			continue
		}
		if _, dup := ranges[m[3]]; dup {
			t.Errorf("rt.xo: %s has two ranges of static words", m[3])
		}
		ranges[m[3]] = span{lo, hi}
		for k := lo; k <= hi; k++ {
			if owner[k] != "" {
				t.Errorf("rt.xo: static word %d is listed for both %s and %s", k, owner[k], m[3])
			}
			owner[k] = m[3]
		}
	}
	if len(ranges) == 0 {
		t.Fatal("rt.xo: no table of static words found (lines `///   LO-HI std/native/...xo: ...`)")
	}
	use := regexp.MustCompile(`rt\.static_word\(\s*([A-Za-z_][A-Za-z_0-9]*|\d+)\s*\)`)
	err = filepath.WalkDir(filepath.Join(root, "std", "native"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".xo") {
			return err
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		uses := use.FindAllStringSubmatch(string(src), -1)
		if len(uses) == 0 {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if rel == "std/native/rt.xo" {
			return nil
		}
		r, ok := ranges[rel]
		if !ok {
			t.Errorf("%s calls rt.static_word but rt.xo's table lists no range for it", rel)
			return nil
		}
		for _, u := range uses {
			k, err := strconv.Atoi(u[1])
			if err != nil {
				c := regexp.MustCompile(`(?m)^(?:pub\s+)?const\s+` + u[1] + `(?:\s*:\s*Int)?\s*=\s*(\d+)\s*$`).FindStringSubmatch(string(src))
				if c == nil {
					t.Errorf("%s: rt.static_word(%s): no `const %s = N` in the same file", rel, u[1], u[1])
					continue
				}
				k, _ = strconv.Atoi(c[1])
			}
			if k < r.lo || k > r.hi {
				t.Errorf("%s: rt.static_word(%s) is word %d, outside its range %d-%d in rt.xo's table", rel, u[1], k, r.lo, r.hi)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestAtomicOrderConstants keeps every module's copy of the slot
// atomics' orders equal to rt.xo's (0 relaxed, 1 acquire, 2 release, 3
// acq_rel, 4 seq_cst): B21's heapos.xo defined ACQUIRE = 2 (release) and
// its central lists' lock took no acquire, so on arm64 two threads got
// one magazine (2026-10-14); tzhost.xo had RELEASE = 3 (acq_rel).
func TestAtomicOrderConstants(t *testing.T) {
	want := map[string]int{"RELAXED": 0, "ACQUIRE": 1, "RELEASE": 2, "ACQ_REL": 3, "SEQ_CST": 4}
	def := regexp.MustCompile(`(?m)^(?:pub\s+)?const\s+(RELAXED|ACQUIRE|RELEASE|ACQ_REL|SEQ_CST)(?:\s*:\s*Int)?\s*=\s*(\d+)\s*$`)
	err := filepath.WalkDir(filepath.Join(root, "std"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".xo") {
			return err
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range def.FindAllStringSubmatch(string(src), -1) {
			if v, _ := strconv.Atoi(m[2]); v != want[m[1]] {
				rel, _ := filepath.Rel(root, p)
				t.Errorf("%s: const %s = %d, want %d (rt.xo's slot atomics' orders)", filepath.ToSlash(rel), m[1], v, want[m[1]])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// rawAddrIssues finds, in the source of a std/native file, a local buffer
// whose address is bound to a variable (`let t = raw.bytes_addr(b)` or
// raw.data(b)) and which is not mentioned again in its function: reference
// counting releases a value after its last use, so the Int address
// outlives the buffer and later reads and writes through it hit freed
// memory (lz4.compress, a SIGSEGV in cold xo builds when the large block
// cache was full, 2026-10-27). A later mention keeps it alive:
// `keep_bytes(b)` after the last raw access, or the buffer returned or
// passed on. An address used inside the statement that takes it is
// consumed before the release, and a literal is never released. Each
// result is "line: name".
func rawAddrIssues(src string) []string {
	fnLine := regexp.MustCompile(`^\s*(?:pub\s+)?fn\s`)
	local := regexp.MustCompile(`^\s*(?:let|var)\s+(\w+)\b`)
	literal := regexp.MustCompile(`^\s*(?:let|var)\s+(\w+)(?:\s*:\s*\w+)?\s*=\s*b?"[^"$]*"\s*$`)
	bound := regexp.MustCompile(`^\s*(?:let|var)\s+\w+(?:\s*:\s*\w+)?\s*=\s*raw\.(?:bytes_addr|data)\((\w+)\)`)
	lines := strings.Split(src, "\n")
	var out []string
	start := 0
	flush := func(end int) {
		body := lines[start:end]
		defined := map[string]bool{}
		for _, l := range body {
			if m := local.FindStringSubmatch(l); m != nil {
				defined[m[1]] = true
			}
			if m := literal.FindStringSubmatch(l); m != nil {
				defined[m[1]] = false // never released
			}
		}
		for i, l := range body {
			m := bound.FindStringSubmatch(l)
			if m == nil || !defined[m[1]] {
				continue // not bound to a variable, a parameter or a field, or a literal
			}
			word := regexp.MustCompile(`\b` + m[1] + `\b`)
			kept := false
			for _, later := range body[i+1:] {
				if word.MatchString(later) {
					kept = true
				}
			}
			if !kept {
				out = append(out, strconv.Itoa(start+i+1)+": "+m[1])
			}
		}
	}
	for i, l := range lines {
		if fnLine.MatchString(l) {
			flush(i)
			start = i
		}
	}
	flush(len(lines))
	return out
}

// TestRawAddrKeepsBuffer: no function of std/native takes the address of
// a local buffer it does not use again (see rawAddrIssues).
func TestRawAddrKeepsBuffer(t *testing.T) {
	bad := "fn f() {\n  let tab = raw.bytes_buf(8)\n  let t = raw.bytes_addr(tab)\n  rt.mem_set(t, 0, 8)\n}\n"
	if got := rawAddrIssues(bad); len(got) != 1 || got[0] != "3: tab" {
		t.Fatalf("the checker missed lz4.compress's old pattern: %v", got)
	}
	good := "fn f() {\n  let tab = raw.bytes_buf(8)\n  let t = raw.bytes_addr(tab)\n  rt.mem_set(t, 0, 8)\n  keep_bytes(tab)\n}\nfn g(p: Bytes) {\n  let a = raw.bytes_addr(p)\n}\n"
	if got := rawAddrIssues(good); len(got) != 0 {
		t.Fatalf("the checker flags a kept buffer or a parameter: %v", got)
	}
	err := filepath.WalkDir(filepath.Join(root, "std", "native"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".xo") || strings.HasSuffix(p, "_test.xo") {
			return err
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		for _, issue := range rawAddrIssues(string(src)) {
			ln, name, _ := strings.Cut(issue, ": ")
			t.Errorf("%s:%s: the address of %s is taken and %s is not used again in the function; reference counting releases it after its last use (add keep_bytes(%s) after the last raw access)", filepath.ToSlash(rel), ln, name, name, name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
