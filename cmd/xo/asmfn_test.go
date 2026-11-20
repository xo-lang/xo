package main

import (
	"strings"
	"testing"
)

// TestDumpAsmFnIsBuildCode (task #468): `xo dev dump asm --fn=NAME DIR`
// prints the function as the build generates it, the same text as
// `xo asm --backend=arm64 --fn=NAME DIR`, which is that function's
// section of the whole `xo asm` output. The program's List[UInt8]
// literal is packed by the build (the dump's own pipeline left it
// unpacked, and read no XO_PASSES). sq, not main: --fn=main also
// names std's cabi_c.main (ssa.named_in, as dump ssa --fn).
func TestDumpAsmFnIsBuildCode(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"xo.mod":  "module example.com/asmfn\n",
		"main.xo": "fn sq(x: Int) -> Int { x * x + 1 }\n\nfn main(os: Os) {\n  let b: List[UInt8] = [1, 2, 3, 4, 5, 6, 7, 8, 9]\n  os.stdio.println(\"${sq(Int(b[1]))} ${b.len()}\")\n}\n",
	})
	chdir(t, dir)
	whole := xo(t, "asm", "--backend=arm64", ".").want(0).out
	one := xo(t, "asm", "--backend=arm64", "--fn=sq", ".").want(0).out
	dump := xo(t, "dev", "dump", "asm", "--fn=sq", ".").want(0).out
	if !strings.HasPrefix(one, "// func sq\n") || strings.Count(one, "// func ") != 1 {
		t.Fatalf("xo asm --fn=sq is not sq alone:\n%s", firstLines(one, 5))
	}
	if !strings.Contains(whole, one) {
		t.Errorf("xo asm --fn=sq is not a section of xo asm:\n%s", firstLines(one, 5))
	}
	if dump != one {
		t.Errorf("xo dev dump asm --fn=sq (%d lines) differs from xo asm --fn=sq (%d lines):\n%s", strings.Count(dump, "\n"), strings.Count(one, "\n"), firstLines(dump, 5))
	}
	if got := xo(t, "asm", "--backend=arm64", "--fn=gone", ".").want(0).out; got != "== no function matches gone\n" {
		t.Errorf("xo asm --fn=gone: %q", got)
	}
}

func firstLines(s string, n int) string {
	l := strings.SplitN(s, "\n", n+1)
	if len(l) > n {
		l = l[:n]
	}
	return strings.Join(l, "\n")
}
