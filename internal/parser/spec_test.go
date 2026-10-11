package parser

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/lexer"
)

// fence is one fenced code block from a Markdown file.
type fence struct {
	line int    // line of the opening fence
	info string // info string, such as "json"
	body string // content with the fence indentation removed
}

var fenceRE = regexp.MustCompile("^(\\s*)```(\\S*)\\s*$")

func readFences(t *testing.T, path string) []fence {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out []fence
	var cur *fence
	var indent string
	var lines []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	n := 0
	for sc.Scan() {
		n++
		ln := sc.Text()
		m := fenceRE.FindStringSubmatch(ln)
		if cur == nil {
			if m != nil {
				cur = &fence{line: n, info: m[2]}
				indent = m[1]
				lines = nil
			}
			continue
		}
		if m != nil && m[2] == "" {
			cur.body = strings.Join(lines, "\n") + "\n"
			out = append(out, *cur)
			cur = nil
			continue
		}
		lines = append(lines, strings.TrimPrefix(ln, indent))
	}
	return out
}

// firstLine returns the first non blank line of s, trimmed.
func firstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		if strings.TrimSpace(ln) != "" {
			return strings.TrimSpace(ln)
		}
	}
	return ""
}

// elide replaces the spec's `{ ... }` placeholder bodies with `{}`.
func elide(s string) string { return strings.ReplaceAll(s, "{ ... }", "{}") }

// specSkips lists core.md blocks that are not xo code, keyed by a prefix of
// their first line. Every entry must match a block.
var specSkips = map[string]string{
	"as        break":        "keyword list (plain text)",
	"module github.com/acme": "xo.mod file, not xo code",
	"module example.com/":    "xo.mod file with deny lines, not xo code",
	"gc min_heap 64MiB":      "xo.mod gc lines, not xo code",
	"replay rerun go":        "xo.mod replay rerun lines, not xo code",
	"vouch github.com/acme":  "xo.mod vouch lines, not xo code",
	"xo preview":             "xo.work file, not xo code",
	"ensures failed:":        "contract failure output sample",
	"xo query":               "shell commands",
	"xo edit":                "shell commands",
	"LoadErr.Db(msg":         "error trace output sample",
	"File        = {":        "EBNF grammar",
}

// specTokenOnly lists blocks that are listings of independent literals, not
// programs: they are checked with the lexer only.
var specTokenOnly = map[string]string{
	"42  1_000_000": "literal listing (core 1.6): literals separated by spaces, one per example",
}

// specKnownFailures lists core.md xo blocks that do not parse because of a
// problem in the spec, not the parser. See the final report and
// knowledgebase/gotchas/spec-examples-drift.md. Empty since spec 0.4 (local
// `const` made the core 3.1 block valid).
var specKnownFailures = map[string]string{}

type exampleResult struct {
	mode string // file, body, signatures, tokens, skip
	errs []diag.Diagnostic
}

func parseExample(src string) exampleResult {
	if _, ds := ParseFile("example.xo", []byte(src), 0); len(ds) == 0 {
		return exampleResult{mode: "file"}
	}
	wrapped := "fn spec_example() {\n" + src + "}\n"
	if _, ds := ParseFile("example.xo", []byte(wrapped), 0); len(ds) == 0 {
		return exampleResult{mode: "body"}
	}
	_, ds := ParseFile("example.xo", []byte(src), SignatureMode)
	if len(ds) == 0 {
		return exampleResult{mode: "signatures"}
	}
	// Report the errors from the plain file attempt, which is the most
	// informative for real declarations.
	_, fileDs := ParseFile("example.xo", []byte(src), 0)
	return exampleResult{mode: "fail", errs: fileDs}
}

func matchKey(first string, table map[string]string) (string, bool) {
	for k := range table {
		if strings.HasPrefix(first, k) {
			return k, true
		}
	}
	return "", false
}

func TestSpecCoreExamples(t *testing.T) {
	fences := readFences(t, "../../spec/core.md")
	if len(fences) < 30 {
		t.Fatalf("found only %d fenced blocks in core.md", len(fences))
	}
	usedSkip := map[string]bool{}
	usedKnown := map[string]bool{}
	counts := map[string]int{}
	for _, f := range fences {
		first := firstLine(f.body)
		name := fmt.Sprintf("core.md:%d", f.line)
		if f.info == "json" {
			counts["skip"]++
			continue
		}
		if k, ok := matchKey(first, specSkips); ok {
			usedSkip[k] = true
			counts["skip"]++
			continue
		}
		src := elide(f.body)
		if _, ok := matchKey(first, specTokenOnly); ok {
			var ds diag.List
			lexer.Lex("example.xo", []byte(src), &ds)
			if len(ds.Items) > 0 {
				t.Errorf("%s: lexer errors in literal listing: %v", name, ds.Items)
			}
			counts["tokens"]++
			continue
		}
		r := parseExample(src)
		known, isKnown := matchKey(first, specKnownFailures)
		switch {
		case r.mode == "fail" && isKnown:
			usedKnown[known] = true
			counts["known failure"]++
			t.Logf("%s: known spec failure (%s)", name, specKnownFailures[known])
		case r.mode == "fail":
			var msgs []string
			for _, d := range r.errs {
				msgs = append(msgs, diag.Human(d))
			}
			t.Errorf("%s: example does not parse:\n%s\n--- block ---\n%s", name, strings.Join(msgs, "\n"), f.body)
		case isKnown:
			usedKnown[known] = true
			t.Errorf("%s: listed as a known failure but parses as %s; remove it from specKnownFailures", name, r.mode)
		default:
			counts[r.mode]++
		}
	}
	for k := range specSkips {
		if !usedSkip[k] {
			t.Errorf("skip entry %q matched no block", k)
		}
	}
	for k := range specKnownFailures {
		if !usedKnown[k] {
			t.Errorf("known failure entry %q matched no block", k)
		}
	}
	t.Logf("core.md blocks: %d total; parsed as file %d, as function body %d, as signatures %d, lexed only %d, known spec failures %d, skipped (not xo) %d",
		len(fences), counts["file"], counts["body"], counts["signatures"], counts["tokens"], counts["known failure"], counts["skip"])
}

// TestSpecStdExamples parses std.md blocks. Most are signature listings:
// they are parsed in SignatureMode after rewriting the documentation only
// receiver marker `[var]` to `var`. Lines that are not declarations at all
// (`Int.parse(s: Str) -> Int?`, `xs + ys`, `o ?? default`) are pseudo
// signatures; they are dropped and counted.
func TestSpecStdExamples(t *testing.T) {
	fences := readFences(t, "../../spec/std.md")
	declStart := regexp.MustCompile(`^(pub |fn |struct |enum |interface |type |const |use |test |\}|//|\s|$)`)
	parsed, pseudo := 0, 0
	for _, f := range fences {
		name := fmt.Sprintf("std.md:%d", f.line)
		src := strings.ReplaceAll(f.body, "([var] ", "(var ")
		if r := parseExample(src); r.mode != "fail" {
			parsed++
			continue
		}
		var kept []string
		for _, ln := range strings.Split(src, "\n") {
			if declStart.MatchString(ln) {
				kept = append(kept, ln)
			} else {
				pseudo++
				t.Logf("%s: pseudo signature skipped: %s", name, strings.TrimSpace(ln))
			}
		}
		_, ds := ParseFile("std.xo", []byte(strings.Join(kept, "\n")), SignatureMode)
		if len(ds) > 0 {
			var msgs []string
			for _, d := range ds {
				msgs = append(msgs, diag.Human(d))
			}
			t.Errorf("%s: signatures do not parse:\n%s\n--- block ---\n%s", name, strings.Join(msgs, "\n"), f.body)
			continue
		}
		parsed++
	}
	t.Logf("std.md blocks: %d total, %d parsed, %d pseudo signature lines skipped", len(fences), parsed, pseudo)
}
