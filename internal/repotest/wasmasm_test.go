package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestWasmAsmOpcodes keeps compiler/wasm's WAT assembler able to read the
// WAT that compiler/wasm writes (gotchas/wasm-assembler-opcode-table.md:
// i64.rotr in 2026-10, f32.convert_i64_u and i32.extend8_s later, each
// found only when a program first used the function holding it). Every
// i32., i64., f32. and f64. mnemonic in compiler/wasm's .xo files appears
// quoted in asm.xo's opcode table; the two consts are parsed apart.
func TestWasmAsmOpcodes(t *testing.T) {
	dir := filepath.Join(root, "compiler", "wasm")
	asm, err := os.ReadFile(filepath.Join(dir, "asm.xo"))
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.xo"))
	if err != nil {
		t.Fatal(err)
	}
	mn := regexp.MustCompile(`\b(?:i32|i64|f32|f64)\.[a-z0-9_]+`)
	missing := map[string][]string{}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range mn.FindAllString(string(src), -1) {
			if m == "f32.const" || m == "f64.const" || strings.Contains(string(asm), `"`+m+`"`) {
				continue
			}
			missing[m] = append(missing[m], filepath.Base(f))
		}
	}
	var names []string
	for m := range missing {
		names = append(names, m)
	}
	sort.Strings(names)
	for _, m := range names {
		t.Errorf("%s (in %s) is not in compiler/wasm/asm.xo's opcode table: every program whose module holds it fails to assemble", m, strings.Join(dedup(missing[m]), ", "))
	}
}

func dedup(xs []string) []string {
	sort.Strings(xs)
	var out []string
	for i, x := range xs {
		if i == 0 || x != xs[i-1] {
			out = append(out, x)
		}
	}
	return out
}
