package stdcoverage

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEmitted: a prelude helper counts as emitted only when compiler/ssa
// spells its whole name (with or without `prelude.` or `llrt_`), an emitting
// call's template fixes its family (`chan_${f}_w`), or std calls its
// intrinsic rt.X; a word in a message, a literal outside compiler/ssa, a test
// file and an `llrt_${name}` template credit nothing.
func TestEmitted(t *testing.T) {
	root := t.TempDir()
	write := func(rel, text string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("compiler/ssa/lower.xo", `fn f(var b: Builder) {
  b.llrt_call("chan_send_w", [])
  b.llrt_call("chan_${fname}_w", [])
  b.prelude_impl("llrt_${name}")
  b.member(nm, "prelude.slice_fail")
  b.unsupported("the runtime's word_in_prose without the native prelude")
  if name is "eprintln" | "env_get" {}
}
`)
	write("compiler/ssa/lower_test.xo", `test "x" { b.llrt_call("only_in_a_test", []) }`)
	write("compiler/parser/habits.xo", `fn g() { ["time"] }`)
	write("std/clock.xo", "fn now() -> Int { rt.clock_now() }\n")
	write("std/native/prelude.xo", "fn llrt_chan_send_w(c: Int) {}\n"+
		"fn llrt_chan_try_recv_w(c: Int) {}\n"+
		"fn llrt_chan_w(c: Int) {}\n"+
		"fn slice_fail() {}\n"+
		"fn llrt_eprintln(s: Str) {}\n"+
		"fn llrt_clock_now() -> Int {}\n"+
		"fn llrt_time(s: Str) -> Int {}\n"+
		"fn llrt_word_in_prose() {}\n"+
		"fn llrt_only_in_a_test() {}\n"+
		"fn llrt_anything() {}\n")
	got, err := Emitted(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"llrt_chan_send_w":     true, // a literal without llrt_
		"llrt_chan_try_recv_w": true, // chan_${fname}_w
		"slice_fail":           true, // "prelude.slice_fail"
		"llrt_eprintln":        true, // a literal a match passes on
		"llrt_clock_now":       true, // rt.clock_now in std
	}
	for n := range want {
		if !got[n] {
			t.Errorf("%s not emitted", n)
		}
	}
	for n := range got {
		if !want[n] {
			t.Errorf("%s emitted, want not (llrt_chan_w has no middle for the template, llrt_time is a parser literal, llrt_word_in_prose a word in a message, llrt_only_in_a_test a test file, llrt_anything only llrt_${name})", n)
		}
	}
}
