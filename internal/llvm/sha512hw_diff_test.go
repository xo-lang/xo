package llvm

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha512"
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

// sha512Compress is FIPS 180-4 6.4.2 on one block (digest_test.go's
// sha512K), the reference of
// cpu.sha512_block.
func sha512Compress(st *[8]uint64, p []byte) {
	var w [80]uint64
	for i := 0; i < 16; i++ {
		w[i] = binary.BigEndian.Uint64(p[8*i:])
	}
	for i := 16; i < 80; i++ {
		s0 := bits.RotateLeft64(w[i-15], -1) ^ bits.RotateLeft64(w[i-15], -8) ^ w[i-15]>>7
		s1 := bits.RotateLeft64(w[i-2], -19) ^ bits.RotateLeft64(w[i-2], -61) ^ w[i-2]>>6
		w[i] = w[i-16] + s0 + w[i-7] + s1
	}
	a, b, c, d, e, f, g, h := st[0], st[1], st[2], st[3], st[4], st[5], st[6], st[7]
	for i := 0; i < 80; i++ {
		t1 := h + (bits.RotateLeft64(e, -14) ^ bits.RotateLeft64(e, -18) ^ bits.RotateLeft64(e, -41)) + (g ^ (e & (f ^ g))) + sha512K[i] + w[i]
		t2 := (bits.RotateLeft64(a, -28) ^ bits.RotateLeft64(a, -34) ^ bits.RotateLeft64(a, -39)) + ((a & b) | (c & (a | b)))
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

// TestSHA512HWDiff checks cpu.sha512_block (decision 0231's SHA-512
// addendum): its instructions (SHA512H, SHA512H2, SHA512SU0, SHA512SU1,
// through LLVM and the native arm64 backend) and its body (the `_soft`
// op) against a Go reference compression on edge and random states and
// blocks; then std's SHA-512 and SHA-384 (sha256.xo, which takes the
// instructions when the CPU has them), one shot, split and as HMAC,
// against crypto/sha512 on lengths around every block boundary.
func TestSHA512HWDiff(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds a program on each native backend")
	}
	if runtime.GOARCH != "arm64" {
		t.Skip("the SHA-512 instructions are arm64's")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	var kb []byte
	for _, k := range sha512K {
		kb = binary.LittleEndian.AppendUint64(kb, k)
	}
	kh := hex.EncodeToString(kb)
	type tcase struct {
		line string
		want string
	}
	var cases []tcase
	block := func(st [8]uint64, p []byte) {
		var sb []byte
		for _, x := range st {
			sb = binary.LittleEndian.AppendUint64(sb, x)
		}
		ref := st
		sha512Compress(&ref, p)
		var wb []byte
		for _, x := range ref {
			wb = binary.LittleEndian.AppendUint64(wb, x)
		}
		for _, op := range []string{"cpu.sha512_block", "cpu.sha512_block_soft"} {
			cases = append(cases, tcase{fmt.Sprintf("%s %x %x %s -", op, sb, p, kh), hex.EncodeToString(wb)})
		}
	}
	for _, x := range []uint64{0, 1, 0x7fffffffffffffff, 0x8000000000000000, 0xfffffffffffffffe, 0xffffffffffffffff} {
		var st [8]uint64
		for i := range st {
			st[i] = x
		}
		for _, fill := range []byte{0, 0xff, 0x80} {
			block(st, bytes.Repeat([]byte{fill}, 128))
		}
	}
	for i := 0; i < 200; i++ {
		var st [8]uint64
		var sb [64]byte
		rand.Read(sb[:])
		for j := range st {
			st[j] = binary.LittleEndian.Uint64(sb[8*j:])
		}
		p := make([]byte, 128)
		rand.Read(p)
		block(st, p)
	}
	hx := func(b []byte) string {
		if len(b) == 0 {
			return "-"
		}
		return hex.EncodeToString(b)
	}
	var lens []int
	for n := 0; n <= 300; n++ {
		lens = append(lens, n)
	}
	lens = append(lens, 1000, 4095, 4096, 4097, 100000)
	for _, n := range lens {
		m := make([]byte, n)
		rand.Read(m)
		s := sha512.Sum512(m)
		cases = append(cases, tcase{fmt.Sprintf("hash.sha512 %s - - -", hx(m)), hex.EncodeToString(s[:])})
		s384 := sha512.Sum384(m)
		cases = append(cases, tcase{fmt.Sprintf("hash.sha384 %s - - -", hx(m)), hex.EncodeToString(s384[:])})
		k := n / 3
		s1 := sha512.Sum512(m[:k])
		cases = append(cases, tcase{fmt.Sprintf("hash.split512 %s %s - -", hx(m[:k]), hx(m[k:])), hex.EncodeToString(s1[:]) + hex.EncodeToString(s[:]) + hex.EncodeToString(s1[:])})
		key := m[:min(n, 200)]
		mac := hmac.New(sha512.New, key)
		mac.Write(m)
		cases = append(cases, tcase{fmt.Sprintf("hmac.sha512 %s %s - -", hx(key), hx(m)), hex.EncodeToString(mac.Sum(nil))})
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(opDiffProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module github.com/xo-lang/xo/test/sha512hw\nedition preview\n"), 0o644); err != nil {
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
