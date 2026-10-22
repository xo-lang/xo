package llvm

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/bits"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// opDiffProgram reads lines `op a b c d` (hex arguments, "-" for empty)
// and prints internal.crypto_op(op, ...) in hex.
const opDiffProgram = `use std/internal

fn h(s: Str) -> Bytes {
  if s == "-" { b"" } else { Bytes.from_hex(s) ?? b"" }
}

fn main(os: Os) {
  while let Some(l) = os.stdio.read_line().ok() ?? None {
    let f = l.split(" ")
    os.stdio.println(internal.crypto_op(f[0], h(f[1]), h(f[2]), h(f[3]), h(f[4])).to_hex())
  }
}
`

// sha256Compress is FIPS 180-4 6.2.2 on one block (digest_test.go's
// sha256K), the reference of cpu.sha256_block.
func sha256Compress(st *[8]uint32, p []byte) {
	var w [64]uint32
	for i := 0; i < 16; i++ {
		w[i] = binary.BigEndian.Uint32(p[4*i:])
	}
	for i := 16; i < 64; i++ {
		s0 := bits.RotateLeft32(w[i-15], -7) ^ bits.RotateLeft32(w[i-15], -18) ^ w[i-15]>>3
		s1 := bits.RotateLeft32(w[i-2], -17) ^ bits.RotateLeft32(w[i-2], -19) ^ w[i-2]>>10
		w[i] = w[i-16] + s0 + w[i-7] + s1
	}
	a, b, c, d, e, f, g, h := st[0], st[1], st[2], st[3], st[4], st[5], st[6], st[7]
	for i := 0; i < 64; i++ {
		t1 := h + (bits.RotateLeft32(e, -6) ^ bits.RotateLeft32(e, -11) ^ bits.RotateLeft32(e, -25)) + (g ^ (e & (f ^ g))) + sha256K[i] + w[i]
		t2 := (bits.RotateLeft32(a, -2) ^ bits.RotateLeft32(a, -13) ^ bits.RotateLeft32(a, -22)) + ((a & b) | (c & (a | b)))
		h, g, f, e, d, c, b, a = g, f, e, d+t1, c, b, a, t1+t2
	}
	st[0] += a
	st[1] += b
	st[2] += c
	st[3] += d
	st[4] += e
	st[5] += f
	st[6] += g
	st[7] += h
}

// TestSHA256HWDiff checks cpu.sha256_block (decision 0231's SHA-256
// section): its instructions (SHA256H and the rest on arm64, through
// both native backends) and its body (the `_soft` op, which no emitter
// replaces) against a Go reference compression, on random and edge
// states and blocks; then std's SHA-256 (sha256.xo and digest.xo, which
// take the instructions when the CPU has them), one shot and split,
// against crypto/sha256 on lengths around every block boundary.
func TestSHA256HWDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program on each native backend")
	}
	if runtime.GOARCH != "arm64" {
		t.Skip("the SHA-256 instructions are arm64's")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	var kb []byte
	for _, k := range sha256K {
		kb = binary.LittleEndian.AppendUint32(kb, k)
	}
	kh := hex.EncodeToString(kb)
	type tcase struct {
		line string
		want string
	}
	var cases []tcase
	block := func(st [8]uint32, p []byte) {
		var sb []byte
		for _, x := range st {
			sb = binary.LittleEndian.AppendUint32(sb, x)
		}
		ref := st
		sha256Compress(&ref, p)
		var wb []byte
		for _, x := range ref {
			wb = binary.LittleEndian.AppendUint32(wb, x)
		}
		for _, op := range []string{"cpu.sha256_block", "cpu.sha256_block_soft"} {
			cases = append(cases, tcase{fmt.Sprintf("%s %x %x %s -", op, sb, p, kh), hex.EncodeToString(wb)})
		}
	}
	var edges = []uint32{0, 1, 0x7fffffff, 0x80000000, 0xfffffffe, 0xffffffff}
	for _, x := range edges {
		var st [8]uint32
		for i := range st {
			st[i] = x
		}
		for _, fill := range []byte{0, 0xff, 0x80} {
			block(st, bytes.Repeat([]byte{fill}, 64))
		}
	}
	for i := 0; i < 200; i++ {
		var st [8]uint32
		var sb [32]byte
		rand.Read(sb[:])
		for j := range st {
			st[j] = binary.LittleEndian.Uint32(sb[4*j:])
		}
		p := make([]byte, 64)
		rand.Read(p)
		block(st, p)
	}
	var lens []int
	for n := 0; n <= 200; n++ {
		lens = append(lens, n)
	}
	lens = append(lens, 1000, 4095, 4096, 4097, 100000)
	for _, n := range lens {
		m := make([]byte, n)
		rand.Read(m)
		s := sha256.Sum256(m)
		mh := hex.EncodeToString(m)
		if mh == "" {
			mh = "-"
		}
		cases = append(cases, tcase{fmt.Sprintf("hash.sha256 %s - - -", mh), hex.EncodeToString(s[:])})
		k := n / 3
		s1 := sha256.Sum256(m[:k])
		a, b := hex.EncodeToString(m[:k]), hex.EncodeToString(m[k:])
		if a == "" {
			a = "-"
		}
		if b == "" {
			b = "-"
		}
		cases = append(cases, tcase{fmt.Sprintf("hash.split256 %s %s - -", a, b), hex.EncodeToString(s1[:]) + hex.EncodeToString(s[:]) + hex.EncodeToString(s1[:])})
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(opDiffProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module github.com/xo-lang/xo/test/sha256hw\nedition preview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var in strings.Builder
	for _, c := range cases {
		in.WriteString(c.line + "\n")
	}
	for _, be := range []string{xotool.BackendLLVM, xotool.BackendArm64} {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be, NoGC: true})
		if err != nil || res.Failed {
			t.Fatalf("build (%s): %v %v", be, err, res.Output)
		}
		cmd := testtmp.Guard(exec.Command(res.Binaries[xotool.Host()]))
		cmd.Stdin = strings.NewReader(in.String())
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %v", be, err)
		}
		sc := bufio.NewScanner(bytes.NewReader(out))
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		i, bad := 0, 0
		for sc.Scan() {
			if i < len(cases) && sc.Text() != cases[i].want {
				if bad++; bad <= 5 {
					l := cases[i].line
					t.Errorf("%s: %s...: got %s, want %s", be, l[:min(len(l), 60)], sc.Text(), cases[i].want)
				}
			}
			i++
		}
		if i != len(cases) || bad > 0 {
			t.Errorf("%s: %d results for %d cases, %d wrong", be, i, len(cases), bad)
		}
	}
}
