package arm64

import (
	"debug/macho"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/gotool"
)

// gnuForms are instruction forms instruction selection and emission
// produce for Go mode functions (goasm.go), in the backend's syntax.
var gnuForms = []string{
	"add x0, x1, x2", "add x0, x1, #16", "add x0, x1, x2, lsl #3", "add x9, x10, x10, lsr #63",
	"add x3, x4, x5, lsl #4", "sub x0, x1, x2", "sub x0, x1, #1", "adds x0, x1, x2", "subs x0, x1, #7",
	"and x0, x1, #7", "and x0, x1, #-8", "and x0, x1, x2", "orr x0, x1, x2", "eor x0, x1, x2", "eor x0, x1, #1",
	"mul x0, x1, x2", "sdiv x0, x1, x2", "smulh x0, x1, x2", "madd x0, x1, x2, x3", "msub x0, x1, x2, x3",
	"lsl x0, x1, #3", "lsr x0, x1, #3", "asr x0, x1, #63", "lsl x0, x1, x2", "asr x0, x1, x2",
	"neg x0, x1", "negs x0, x1", "mvn x0, x1",
	"mov x0, x1", "mov x0, #5", "mov x0, #-1", "mov x0, #65535", "mov x0, #-65536", "mov x0, xzr", "mov x21, #0",
	"movz x0, #4660, lsl #16", "movk x0, #22136, lsl #32", "movn x0, #1, lsl #48", "movz x5, #1, lsl #0",
	"cmp x0, x1", "cmp x0, #4095", "cmp x0, x1, asr #63", "cmn x0, #5", "tst x0, #1", "tst x0, #7", "cmp w0, #0",
	"csel x0, x1, x2, lt", "csel x0, x1, x2, ne", "cset x0, eq", "cset x0, hs", "cset x0, vs", "cneg x0, x1, lt",
	"ccmn x2, #1, #0, eq", "fcsel d0, d1, d2, lt", "fccmp d1, d2, #0, vc",
	"fadd d0, d1, d2", "fsub d0, d1, d2", "fmul d0, d1, d2", "fdiv d0, d1, d2",
	"fneg d0, d1", "fabs d0, d1", "fsqrt d0, d1", "frintz d0, d1", "frintm d0, d1", "frintp d0, d1", "frinta d0, d1",
	"fcmp d0, d1", "fcmp d0, #0.0", "fmov d0, x1", "fmov x0, d1", "fmov d0, d1", "fmov d0, xzr",
	"scvtf d0, x1", "fcvtzs x0, d1",
	"ldr x0, [x1]", "ldr x0, [x1, #8]", "ldr x0, [sp, #16]", "ldr d0, [x1, #24]", "str x0, [x1, #8]", "str xzr, [x1]",
	"str d3, [sp, #40]", "ldrb w0, [x1, #3]", "ldrb w0, [x1, x2]", "strb w0, [x1]", "strb wzr, [x1, #1]",
	"ldp x0, x1, [x2, #16]", "ldp x0, x1, [sp, #8]",
	"b.eq Lt", "b.ne Lt", "b.lt Lt", "b.ge Lt", "b.gt Lt", "b.le Lt", "b.lo Lt", "b.hs Lt", "b.hi Lt", "b.ls Lt",
	"b.mi Lt", "b.pl Lt", "b.vs Lt", "b.vc Lt", "b Lt", "cbz x3, Lt", "cbnz x3, Lt", "cbz w3, Lt",
	"tbz x3, #63, Lt", "tbnz x3, #0, Lt", "ret",
	// preemption polls (decision 0131)
	"ldr x16, [x28, #16]", "tbnz x16, #63, Lt",
}

// TestGoSyntaxEncoding assembles every form with the system assembler
// and its translation with Go's assembler and compares the machine code
// (darwin/arm64 with cc only).
func TestGoSyntaxEncoding(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("needs darwin/arm64")
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no cc")
	}
	dir := t.TempDir()
	var gnu, gos strings.Builder
	gnu.WriteString("\t.text\n_f:\nLt:\n")
	gos.WriteString("#include \"textflag.h\"\nTEXT ·f(SB), NOSPLIT|NOFRAME, $0-0\nLt:\n")
	for _, f := range gnuForms {
		g, err := goInstr(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		gnu.WriteString("\t" + f + "\n")
		gos.WriteString("\t" + g + "\n")
	}
	gs, gobj := filepath.Join(dir, "a.s"), filepath.Join(dir, "a.o")
	os.WriteFile(gs, []byte(gnu.String()), 0o644)
	if out, err := exec.Command(cc, "-c", "-o", gobj, gs).CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, out)
	}
	mf, err := macho.Open(gobj)
	if err != nil {
		t.Fatal(err)
	}
	text, err := mf.Section("__text").Data()
	if err != nil {
		t.Fatal(err)
	}
	var want []uint32
	for i := 0; i+4 <= len(text); i += 4 {
		want = append(want, binary.LittleEndian.Uint32(text[i:]))
	}
	ps, pobj := filepath.Join(dir, "b.s"), filepath.Join(dir, "b.o")
	os.WriteFile(ps, []byte(gos.String()), 0o644)
	inc := filepath.Join(runtime.GOROOT(), "pkg", "include")
	// go tool objdump is built on first use (Go 1.25 ships no binary for
	// it), so both run on Xo's Go cache (decision 0176)
	asm := exec.Command("go", "tool", "asm", "-p", "main", "-I", inc, "-o", pobj, ps)
	asm.Env = append(os.Environ(), "GOWORK=off", gotool.CacheEnv())
	defer gotool.HoldCache()()
	if out, err := asm.CombinedOutput(); err != nil {
		t.Fatalf("go tool asm: %v\n%s\n%s", err, out, gos.String())
	}
	objdump := exec.Command("go", "tool", "objdump", pobj)
	objdump.Env = asm.Env
	out, err := objdump.CombinedOutput()
	if err != nil {
		t.Fatalf("objdump: %v\n%s", err, out)
	}
	re := regexp.MustCompile(`0x[0-9a-f]+\s+([0-9a-f]{8})\s`)
	var got []uint32
	for _, m := range re.FindAllStringSubmatch(string(out), -1) {
		var w uint32
		for _, c := range m[1] {
			w <<= 4
			if c >= 'a' {
				w |= uint32(c-'a') + 10
			} else {
				w |= uint32(c - '0')
			}
		}
		got = append(got, w)
	}
	// the function's text is padded with zero words to 16 bytes
	for len(got) > len(gnuForms) && got[len(got)-1] == 0 {
		got = got[:len(got)-1]
	}
	if len(got) != len(want) {
		t.Fatalf("%d instructions from Go's assembler, %d from cc\n%s", len(got), len(want), out)
	}
	for i, f := range gnuForms {
		// Go's assembler writes `MOVD $0, R` as `mov x, xzr` (orr), an
		// equivalent instruction
		if got[i] != want[i] && !(f == "mov x21, #0" && got[i] == 0xaa1f03f5) {
			g, _ := goInstr(f)
			t.Errorf("%-28s -> %-36s: Go %08x, cc %08x", f, g, got[i], want[i])
		}
	}
}
