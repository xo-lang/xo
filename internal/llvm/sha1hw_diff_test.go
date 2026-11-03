package llvm

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
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

// sha1K is FIPS 180-4's four SHA-1 constants.
var sha1K = [4]uint32{0x5a827999, 0x6ed9eba1, 0x8f1bbcdc, 0xca62c1d6}

// sha1Compress is FIPS 180-4 6.1.2 on one block, the reference of
// cpu.sha1_block.
func sha1Compress(st *[5]uint32, p []byte) {
	var w [80]uint32
	for i := 0; i < 16; i++ {
		w[i] = binary.BigEndian.Uint32(p[4*i:])
	}
	for i := 16; i < 80; i++ {
		w[i] = bits.RotateLeft32(w[i-3]^w[i-8]^w[i-14]^w[i-16], 1)
	}
	a, b, c, d, e := st[0], st[1], st[2], st[3], st[4]
	for i := 0; i < 80; i++ {
		var f uint32
		switch {
		case i < 20:
			f = d ^ (b & (c ^ d))
		case i >= 40 && i < 60:
			f = (b & c) | (d & (b | c))
		default:
			f = b ^ c ^ d
		}
		t := bits.RotateLeft32(a, 5) + f + e + sha1K[i/20] + w[i]
		e, d, c, b, a = d, c, bits.RotateLeft32(b, 30), a, t
	}
	st[0] += a
	st[1] += b
	st[2] += c
	st[3] += d
	st[4] += e
}

// TestSHA1HWDiff checks cpu.sha1_block (decision 0231's SHA-1 addendum):
// its instructions (SHA1C and the rest on arm64, through both native
// backends) and its body (the `_soft` op) against a Go reference
// compression on edge and random states and blocks; then std's SHA-1
// (sha1.xo, which takes the instructions when the CPU has them), one
// shot, split and as HMAC, against crypto/sha1 on lengths around every
// block boundary.
func TestSHA1HWDiff(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds a program on each native backend")
	}
	if runtime.GOARCH != "arm64" {
		t.Skip("the SHA-1 instructions checked here are arm64's (x86-64's SHA-NI: the cpu_sha1 golden in the amd64 sample)")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	var kb []byte
	for _, k := range sha1K {
		for j := 0; j < 4; j++ {
			kb = binary.LittleEndian.AppendUint32(kb, k)
		}
	}
	kh := hex.EncodeToString(kb)
	type tcase struct {
		line string
		want string
	}
	var cases []tcase
	block := func(st [5]uint32, p []byte) {
		var sb []byte
		for _, x := range st {
			sb = binary.LittleEndian.AppendUint32(sb, x)
		}
		ref := st
		sha1Compress(&ref, p)
		var wb []byte
		for _, x := range ref {
			wb = binary.LittleEndian.AppendUint32(wb, x)
		}
		for _, op := range []string{"cpu.sha1_block", "cpu.sha1_block_soft"} {
			cases = append(cases, tcase{fmt.Sprintf("%s %x %x %s -", op, sb, p, kh), hex.EncodeToString(wb)})
		}
	}
	for _, x := range []uint32{0, 1, 0x7fffffff, 0x80000000, 0xfffffffe, 0xffffffff} {
		var st [5]uint32
		for i := range st {
			st[i] = x
		}
		for _, fill := range []byte{0, 0xff, 0x80} {
			block(st, bytes.Repeat([]byte{fill}, 64))
		}
	}
	for i := 0; i < 200; i++ {
		var st [5]uint32
		var sb [20]byte
		rand.Read(sb[:])
		for j := range st {
			st[j] = binary.LittleEndian.Uint32(sb[4*j:])
		}
		p := make([]byte, 64)
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
	for n := 0; n <= 200; n++ {
		lens = append(lens, n)
	}
	lens = append(lens, 1000, 4095, 4096, 4097, 100000)
	for _, n := range lens {
		m := make([]byte, n)
		rand.Read(m)
		s := sha1.Sum(m)
		cases = append(cases, tcase{fmt.Sprintf("sha1.sum %s - - -", hx(m)), hex.EncodeToString(s[:])})
		k := n / 3
		s1 := sha1.Sum(m[:k])
		cases = append(cases, tcase{fmt.Sprintf("sha1.split %s %s - -", hx(m[:k]), hx(m[k:])), hex.EncodeToString(s1[:]) + hex.EncodeToString(s[:])})
		key := m[:min(n, 100)]
		mac := hmac.New(sha1.New, key)
		mac.Write(m)
		cases = append(cases, tcase{fmt.Sprintf("sha1.hmac %s %s - -", hx(key), hx(m)), hex.EncodeToString(mac.Sum(nil))})
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(opDiffProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module github.com/xo-lang/xo/test/sha1hw\nedition preview\n"), 0o644); err != nil {
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
