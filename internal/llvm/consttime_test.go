package llvm

import (
	"bytes"
	"crypto/subtle"
	"encoding/hex"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// cryptoXoDiffProg runs crypto.equal and Ed25519 on its input lines (byte
// strings in hex, "-" empty): `equal A B`, `edpub SEED`, `edsign SEED
// MSG`, `edverify PUB MSG SIG`; one result a line, hex or 0 or 1.
const cryptoXoDiffProg = `use std/crypto

fn unhex(f: List[Str], i: Int) -> Bytes {
  let s = f.get(i) ?? "-"
  if s == "-" { b"" } else { Bytes.from_hex(s) ?? b"?" }
}

fn hx(b: Bytes) -> Str { if b.is_empty() { "-" } else { b.to_hex() } }

fn bit(ok: Bool) -> Str { if ok { "1" } else { "0" } }

fn main(os: Os) {
  let input = match os.stdio.read_all() {
    Ok(s) => s
    Err(_) => ""
  }
  var out: List[Str] = []
  for line in input.lines() {
    let f = line.split(" ")
    match f[0] {
      "equal" => out.push(bit(crypto.equal(unhex(f, 1), unhex(f, 2))))
      "edpub" => out.push(hx(crypto.ed25519_public_key(unhex(f, 1))))
      "edsign" => out.push(hx(crypto.ed25519_sign(unhex(f, 1), unhex(f, 2))))
      "edverify" => out.push(bit(crypto.ed25519_verify(unhex(f, 1), unhex(f, 2), unhex(f, 3))))
      _ => out.push("?")
    }
  }
  os.stdio.println(out.join("\n"))
}
`

// TestCryptoXoDiff runs cryptoXoDiffProg on both native backends and
// requires Go's answers: crypto.equal (std/native/llrt/ct.xo, decision
// 0222) against subtle.ConstantTimeCompare on random byte strings of
// every length to 40 and longer ones, equal, one byte changed, and of
// other lengths; Ed25519 on ed25519Cases (the RFC 8032 vectors, random
// keys and messages, mutated and non canonical inputs).
func TestCryptoXoDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two programs")
	}
	r := rand.New(rand.NewSource(5))
	rb := func(n int) []byte {
		b := make([]byte, n)
		r.Read(b)
		return b
	}
	hx := func(b []byte) string {
		if len(b) == 0 {
			return "-"
		}
		return hex.EncodeToString(b)
	}
	var in strings.Builder
	var want []string
	add := func(line string, w string) {
		in.WriteString(line + "\n")
		want = append(want, w)
	}
	for i := 0; i < 400; i++ {
		n := i % 41
		if i >= 300 {
			n = r.Intn(2000)
		}
		a := rb(n)
		b := append([]byte(nil), a...)
		switch r.Intn(4) {
		case 0:
			if n > 0 {
				b[r.Intn(n)] ^= byte(1 << r.Intn(8))
			}
		case 1:
			b = rb(n)
		case 2:
			b = append(b, byte(r.Intn(256)))
		}
		w := "0"
		if subtle.ConstantTimeCompare(a, b) == 1 {
			w = "1"
		}
		add("equal "+hx(a)+" "+hx(b), w)
	}
	for _, c := range ed25519Cases(r, 200) {
		if strings.HasPrefix(c.in, "sha512 ") {
			continue // not part of std/crypto's surface
		}
		add(c.in, c.want)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(cryptoXoDiffProg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stdin.txt"), []byte(in.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d input lines", len(want))
	for _, be := range []string{xotool.BackendArm64, xotool.BackendLLVM} {
		t.Run(be, func(t *testing.T) {
			r := buildAndRun(t, dir, xotool.Options{Backend: be})
			if r.exit != 0 {
				t.Fatalf("exit %d: %s", r.exit, r.stderr)
			}
			diffLines(t, strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n"), want)
		})
	}
}

// ctLLVMProg pulls the runtime's constant time code into a program.
const ctLLVMProg = `use std/crypto

fn main(os: Os) {
  let n = os.env.args().len()
  let seed = "0123456789abcdef0123456789abcde${n}".bytes()
  let sig = crypto.ed25519_sign(seed, b"m")
  os.stdio.println("${crypto.equal(b"abcdefghij", "abcdefghi${n}".bytes())} ${crypto.ed25519_verify(crypto.ed25519_public_key(seed), b"m", sig)}")
}
`

// ctCheck is what TestConstantTimeLLVM requires of one function after
// clang -O2: barriers is the least number of rt.ct_barrier asm blocks
// left in it; branchFree forbids conditional branches; noSelect forbids
// conditional selects and moves (csel and cset are constant time, but a
// select LLVM formed from a mask means it saw through the barrier, and
// x86's cmov conversion may turn it into a branch).
type ctCheck struct {
	fn         string
	barriers   int
	branchFree bool
	noSelect   bool
}

var ctChecks = []ctCheck{
	{fn: "ct.mask_nonzero", barriers: 1, branchFree: true, noSelect: true},
	// the loops keep a barrier each (no early exit); their branches and
	// selects are on the public lengths
	{fn: "ct.bytes_equal", barriers: 2},
}

// TestConstantTimeLLVM compiles the LLVM IR xo emits for ctLLVMProg with
// clang -O2 for the host, keeping the functions of ctChecks as separate
// functions (external and noinline, so the optimizer's view of their
// bodies is the one inlining sees), and requires each to keep its value
// barriers (decision 0222: the empty asm of rt.ct_barrier) and to have
// no conditional branch or select where ctChecks says. The self-hosted
// arm64 emitter's code is checked by test/codegen/consttime.
func TestConstantTimeLLVM(t *testing.T) {
	clang, err := xotool.Clang()
	if err != nil {
		t.Skip("clang not found")
	}
	var branchRE, selectRE *regexp.Regexp
	switch runtime.GOARCH {
	case "arm64":
		branchRE = regexp.MustCompile(`(?m)^\s+(b\.[a-z]+|cbn?z|tbn?z)\s`)
		selectRE = regexp.MustCompile(`(?m)^\s+(csel|csinc|csinv|csneg)\s`)
	case "amd64":
		branchRE = regexp.MustCompile(`(?m)^\s+j(?:[a-ln-z][a-z]*|m[a-oq-z][a-z]*)\s`) // every jcc, not jmp
		selectRE = regexp.MustCompile(`(?m)^\s+cmov[a-z]+\s`)
	default:
		t.Skipf("no instruction patterns for %s", runtime.GOARCH)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(ctLLVMProg), 0o644); err != nil {
		t.Fatal(err)
	}
	ir, err := xotool.LLVMIR(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range ctChecks {
		names = append(names, c.fn)
	}
	kept, err := ctKeep(string(ir), names)
	if err != nil {
		t.Fatal(err)
	}
	ll := filepath.Join(dir, "prog.ll")
	if err := os.WriteFile(ll, []byte(kept), 0o644); err != nil {
		t.Fatal(err)
	}
	asm := filepath.Join(dir, "prog.s")
	cmd := exec.Command(clang, "-O2", "-S", "-Wno-override-module", "-o", asm, ll)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("clang -O2: %v\n%s", err, stderr.String())
	}
	out, err := os.ReadFile(asm)
	if err != nil {
		t.Fatal(err)
	}
	asmRE := regexp.MustCompile(`(?m)InlineAsm Start|^\s*[#/;]+APP$`)
	for _, c := range ctChecks {
		body := ctAsmFunc(string(out), "x."+c.fn)
		if body == "" {
			t.Errorf("%s: no function in clang's output", c.fn)
			continue
		}
		if n := len(asmRE.FindAllString(body, -1)); n < c.barriers {
			t.Errorf("%s: %d value barriers after -O2, want at least %d:\n%s", c.fn, n, c.barriers, body)
		}
		if c.branchFree && branchRE.MatchString(body) {
			t.Errorf("%s: a conditional branch after -O2:\n%s", c.fn, body)
		}
		if c.noSelect && selectRE.MatchString(body) {
			t.Errorf("%s: a conditional select after -O2:\n%s", c.fn, body)
		}
	}
}

// ctKeep makes the functions x.NAME of ir external and noinline and drops
// the alwaysinline of their call sites (xo's inliner marks those), so
// clang -O2 keeps them as functions.
func ctKeep(ir string, names []string) (string, error) {
	attrs := regexp.MustCompile(`(?m)^attributes #(\d+) = \{ alwaysinline \}$`).FindAllStringSubmatch(ir, -1)
	for _, n := range names {
		q := regexp.QuoteMeta(`@"x.` + n + `"`)
		def := regexp.MustCompile(`(?m)^define internal (.*` + q + `\(.*\)) (#\d+ (?:personality [^{]*)?\{)$`)
		if !def.MatchString(ir) {
			return "", &ctError{"no definition of x." + n + " in the IR"}
		}
		ir = def.ReplaceAllString(ir, "define $1 noinline $2")
		for _, a := range attrs {
			call := regexp.MustCompile(`(` + q + `\([^\n]*\)) #` + a[1] + `\b`)
			ir = call.ReplaceAllString(ir, "$1")
		}
	}
	return ir, nil
}

type ctError struct{ s string }

func (e *ctError) Error() string { return e.s }

// ctAsmFunc is the assembly of the function sym (Mach-O prefixes an
// underscore; ELF may quote the name) up to its .cfi_endproc.
func ctAsmFunc(asm, sym string) string {
	for _, label := range []string{"_" + sym + ":", sym + ":", `"` + sym + `":`} {
		i := strings.Index(asm, "\n"+label)
		if i < 0 {
			continue
		}
		rest := asm[i+1:]
		if j := strings.Index(rest, ".cfi_endproc"); j >= 0 {
			return rest[:j]
		}
		return rest
	}
	return ""
}
