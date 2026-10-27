package stdcoverage

import (
	"os"
	"path/filepath"
	"testing"
)

// TestUncoveredResolvedCrossesImplementationBodies: a std module's front
// declaration lives in std/<module>.xo, which has no bodies, and the body
// lives in std/native/<module>.xo; `xo dev edges` keys a body's sites by the
// body's file and line. A call to the front declaration must therefore reach
// what the body calls, or every declaration the body names reads as unreached
// (the 2026-10-26 audit found 28 std/sql.xo declarations reached from roots
// and 166 further targets behind them). The chain crosses a declaration that
// exists only in the body file, like std/native/sql.xo's Pool: such a
// declaration has no front twin, so it has to be a node of the walk itself or
// the walk stops there. It crosses a std/native/llrt helper the same way, and
// a helper whose file has the same base name as a front file must not join
// that file's alias group (std/trace.xo and std/native/llrt/trace.xo both
// exist).
func TestUncoveredResolvedCrossesImplementationBodies(t *testing.T) {
	root := t.TempDir()
	// corpusTextIn walks these directories, so they must exist even when empty
	for _, d := range []string{
		"internal/stdtest/testdata/corpus",
		"internal/llvm/testdata/golden",
		"internal/arm64/testdata",
		"internal/gen/testdata",
		"cmd/xo/testdata",
		"bench/perf",
		"scripts",
	} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(rel, text string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// the front declarations, of which the body file implements two
	write("std/example.xo", "fn front(x: Int) -> Int\nfn behind() -> Int\nfn only_alpha() -> Int\n"+
		"fn untouched() -> Int\nfn twinned() -> Int\nfn (a: Alpha) run() -> Int\nfn (b: Beta) run() -> Int\n")
	// the body: `front` calls `hop`, which lives only in this file and calls
	// the llrt helper `rt_hop`; Alpha.run calls `only_alpha`; Beta.run has no
	// body here and must not be credited through Alpha
	write("std/native/example.xo", "fn front(x: Int) -> Int {\n  hop()\n}\n\n"+
		"fn hop() -> Int {\n  rt_hop()\n}\n\n"+
		"fn (a: Alpha) run() -> Int {\n  only_alpha()\n}\n")
	// the runtime helper: `rt_hop` names the reported `behind`, so the walk has
	// to treat this file's declarations as nodes too; `twinned` shares its name
	// with the front file's `twinned` because this file's base name is the same
	write("std/native/llrt/example.xo", "fn rt_hop() -> Int {\n  behind()\n}\n\n"+
		"fn twinned() -> Int {\n  behind()\n}\n")
	// loadDecls always reads the native prelude, so the fixture needs one
	write("std/native/prelude.xo", "fn lowered_by_a_method(x: Int) -> Int {\n  x\n}\n")
	write("internal/stdtest/testdata/corpus/example/example_test.xo", "test \"front\" {\n  expect(front(1)) == 1\n}\n")

	edges := []Edge{
		// a root site: the corpus test calls the front declaration
		{SrcPath: "", TgtPath: "std/example.xo", TgtLine: 1, TgtName: "front", SitePath: "internal/stdtest/testdata/corpus/example/example_test.xo"},
		// the body's own site names `hop`, a declaration only the body file has
		{SrcPath: "std/native/example.xo", SrcLine: 1, TgtPath: "std/native/example.xo", TgtLine: 5, TgtName: "hop", SitePath: "std/native/example.xo"},
		// hop's body names the llrt helper `rt_hop`
		{SrcPath: "std/native/example.xo", SrcLine: 5, TgtPath: "std/native/llrt/example.xo", TgtLine: 1, TgtName: "rt_hop", SitePath: "std/native/example.xo"},
		// rt_hop's body names `behind`, a reported declaration
		{SrcPath: "std/native/llrt/example.xo", SrcLine: 1, TgtPath: "std/example.xo", TgtLine: 2, TgtName: "behind", SitePath: "std/native/llrt/example.xo"},
		// a root site reaches the llrt `twinned`, and must not thereby reach
		// the front file's `twinned`: they share module, symbol and kind but are
		// different declarations in different directories
		{SrcPath: "", TgtPath: "std/native/llrt/example.xo", TgtLine: 5, TgtName: "twinned", SitePath: "internal/stdtest/testdata/corpus/example/example_test.xo"},
		// a second root site calls Alpha.run (std/example.xo line 6)
		{SrcPath: "", TgtPath: "std/example.xo", TgtLine: 6, TgtName: "Alpha.run", SitePath: "internal/stdtest/testdata/corpus/example/example_test.xo"},
		// and Alpha.run's body names `only_alpha`
		{SrcPath: "std/native/example.xo", SrcLine: 9, TgtPath: "std/example.xo", TgtLine: 3, TgtName: "only_alpha", SitePath: "std/native/example.xo"},
	}
	entries, total, _, err := UncoveredResolved(root, edges)
	if err != nil {
		t.Fatal(err)
	}
	// the reported declarations are the std API surface: the five functions
	// and two methods of std/example.xo, plus the one declaration of the
	// native prelude fixture; the body and llrt files' declarations add nothing
	if total != 8 {
		t.Errorf("total = %d, want 8", total)
	}
	got := map[string]bool{}
	for _, e := range entries {
		got[e.Module+"."+e.Symbol] = true
	}
	// the body-only and llrt helpers are nodes of the walk, not reported
	// declarations; the llrt file's module is its base name, so rt_hop would
	// be reported as example.rt_hop if llrt files were reported
	if got["example.hop"] || got["example.rt_hop"] {
		t.Errorf("a body-only helper is not an API declaration: %v", entries)
	}
	// front is a root target, and behind is named only through `hop` and the
	// llrt `rt_hop`, so the walk has to cross into the body and then treat the
	// llrt helper as a node of its own; the same for Alpha.run and only_alpha
	for _, reached := range []string{"example.front", "example.behind", "example.Alpha.run", "example.only_alpha"} {
		if got[reached] {
			t.Errorf("%s reported uncovered, so the walk did not cross into the body: %v", reached, entries)
		}
	}
	if !got["example.untouched"] {
		t.Errorf("example.untouched is named by no edge and must stay uncovered: %v", entries)
	}
	// the llrt file's `twinned` is reached and the front file's `twinned` is
	// not: a pair is only a pair across std/<m>.xo and std/native/<m>.xo
	nTwinned := 0
	for _, e := range entries {
		if e.Module+"."+e.Symbol == "example.twinned" {
			nTwinned++
		}
	}
	if !got["example.twinned"] || nTwinned != 1 {
		t.Errorf("the reached std/native/llrt/example.xo twinned credited the front twinned: %v", entries)
	}
	// the alias key carries the receiver: Beta.run shares only the method
	// name with Alpha.run, is a different declaration, and stays uncovered
	if !got["example.Beta.run"] {
		t.Errorf("example.Beta.run was credited through Alpha.run: %v", entries)
	}
}

// TestCallsIn: the scanner finds a bare call, a qualified call, one with
// type arguments, and a call after a field access.
// TestDeclaredGenericFunctions: a generic function's name is followed by its
// type parameters in brackets (`fn map_all[T](xs: List[T])`), so the
// function and interface method patterns have to accept `[` as well as `(`;
// until 2026-10-27 they did not, and no generic std function or interface
// method (Rand.shuffle) was ever counted, covered or reported.
func TestDeclaredGenericFunctions(t *testing.T) {
	p := filepath.Join(t.TempDir(), "example.xo")
	text := "fn plain(x: Int) -> Int\n" +
		"pub fn generic[T](x: T) -> T\n" +
		"fn two[K, V](m: Map[K, V]) -> Int\n" +
		"fn (xs: List[T]) first_of() -> T\n" +
		"interface Pick {\n  fn one(self) -> Int\n  fn many[T](self, xs: List[T]) -> List[T]\n}\n"
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	ds, err := declared("example", p)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, d := range ds {
		got[d.Symbol] = d.Kind
	}
	want := map[string]string{"plain": "fn", "generic": "fn", "two": "fn", "List.first_of": "method",
		"Pick.one": "ifmethod", "Pick.many": "ifmethod"}
	for sym, kind := range want {
		if got[sym] != kind {
			t.Errorf("%s: kind %q, want %q (declared %v)", sym, got[sym], kind, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("declared %v, want exactly %v", got, want)
	}
}

func TestCallsIn(t *testing.T) {
	calls := callsIn(`let x = s.contains("a") + y.find_where[E](f) + Int.parse(s) + t.await`)
	for _, want := range []string{"contains", "s.contains", "find_where", "y.find_where", "parse", "Int.parse"} {
		if !calls[want] {
			t.Errorf("callsIn missed %q: %v", want, calls)
		}
	}
	if calls["await"] {
		t.Errorf("callsIn treated t.await as a call")
	}
}

// TestRuleMatch: exact symbols and families, and nothing else.
func TestRuleMatch(t *testing.T) {
	exact := Rule{Pattern: "prelude.Cmd.inherit"}
	if !exact.Match(Entry{Module: "prelude", Symbol: "Cmd.inherit"}) {
		t.Error("exact rule did not match")
	}
	if exact.Match(Entry{Module: "prelude", Symbol: "Cmd.inherit2"}) {
		t.Error("exact rule matched another symbol")
	}
	family := Rule{Pattern: "prelude.llrt_*"}
	if !family.Match(Entry{Module: "prelude", Symbol: "llrt_print"}) {
		t.Error("family rule did not match")
	}
	if family.Match(Entry{Module: "debug", Symbol: "llrt_print"}) {
		t.Error("family rule matched another module")
	}
}

// TestParseAllow: a reason is required, so an allowlist line cannot be a
// bare symbol.
func TestParseAllow(t *testing.T) {
	rules, err := ParseAllow("# comment\n\nprelude.Cmd.inherit  needs a real child\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].Reason != "needs a real child" {
		t.Fatalf("parsed %+v", rules)
	}
	if _, err := ParseAllow("prelude.Cmd.inherit\n"); err == nil {
		t.Fatal("a line with no reason was accepted")
	}
}

// TestAllowParses keeps the embedded file well formed.
func TestAllowParses(t *testing.T) {
	rules, err := Allow()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) < 5 {
		t.Fatalf("allow.txt has %d rules, too few for the internals it covers", len(rules))
	}
}
