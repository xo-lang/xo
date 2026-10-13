package llvm_test

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xo-lang/xo/internal/xotool"
	"github.com/xo-lang/xo/std/rt"
)

// digestTablesPath is the generated table module of the native runtime's
// hashes, written in Xo (std/native/llrt/digest.xo, decision 0195).
var digestTablesPath = filepath.Join("..", "..", "std", "native", "llrt", "digest_tables.xo")

// sha256K are SHA-256's round constants (FIPS 180-4 4.2.2).
var sha256K = [64]uint32{
	0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
	0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
	0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
	0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
	0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
	0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
	0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
	0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
}

// blake2bIV is BLAKE2b's initialization vector (RFC 7693 2.6).
var blake2bIV = [8]uint64{
	0x6a09e667f3bcc908, 0xbb67ae8584caa73b, 0x3c6ef372fe94f82b, 0xa54ff53a5f1d36f1,
	0x510e527fade682d1, 0x9b05688c2b3e6c1f, 0x1f83d9abfb41bd6b, 0x5be0cd19137e2179,
}

// blake2bSigma is BLAKE2b's message schedule (RFC 7693 2.7), rounds 0 to
// 11.
var blake2bSigma = [12][16]int{
	{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	{14, 10, 4, 8, 9, 15, 13, 6, 1, 12, 0, 2, 11, 7, 5, 3},
	{11, 8, 12, 0, 5, 2, 15, 13, 10, 14, 3, 6, 7, 1, 9, 4},
	{7, 9, 3, 1, 13, 12, 11, 14, 2, 6, 5, 10, 4, 0, 15, 8},
	{9, 0, 5, 7, 2, 4, 10, 15, 14, 1, 11, 12, 6, 8, 3, 13},
	{2, 12, 6, 10, 0, 11, 8, 3, 4, 13, 7, 5, 15, 14, 1, 9},
	{12, 5, 1, 15, 14, 13, 4, 10, 0, 7, 6, 3, 9, 2, 8, 11},
	{13, 11, 7, 14, 12, 1, 3, 9, 5, 0, 15, 4, 8, 6, 2, 10},
	{6, 15, 14, 9, 11, 3, 0, 8, 12, 2, 13, 7, 1, 4, 10, 5},
	{10, 2, 8, 4, 7, 6, 1, 5, 15, 11, 9, 14, 3, 12, 13, 0},
	{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	{14, 10, 4, 8, 9, 15, 13, 6, 1, 12, 0, 2, 11, 7, 5, 3},
}

// mixG writes BLAKE2b's G on the locals v<a> to v<d>, adding x and y
// (RFC 7693 3.1), or, with x empty, Argon2's G (fBlaMka: a + b + 2 *
// lo(a) * lo(b), RFC 9106 3.6).
func mixG(b *bytes.Buffer, a, bb, c, d int, x, y string) {
	add := func(p, q string) string {
		if x == "" {
			return fmt.Sprintf("fbm(%s, %s)", p, q)
		}
		return fmt.Sprintf("rt.int_wrap(0, %s, %s)", p, q)
	}
	addm := func(p, q, m string) string {
		if x == "" {
			return add(p, q)
		}
		return fmt.Sprintf("rt.int_wrap(0, rt.int_wrap(0, %s, %s), %s)", p, q, m)
	}
	va, vb, vc, vd := fmt.Sprintf("v%d", a), fmt.Sprintf("v%d", bb), fmt.Sprintf("v%d", c), fmt.Sprintf("v%d", d)
	fmt.Fprintf(b, "  %s = %s\n", va, addm(va, vb, x))
	fmt.Fprintf(b, "  %s = rt.int_wrap(5, %s ^ %s, 32)\n", vd, vd, va)
	fmt.Fprintf(b, "  %s = %s\n", vc, add(vc, vd))
	fmt.Fprintf(b, "  %s = rt.int_wrap(5, %s ^ %s, 24)\n", vb, vb, vc)
	fmt.Fprintf(b, "  %s = %s\n", va, addm(va, vb, y))
	fmt.Fprintf(b, "  %s = rt.int_wrap(5, %s ^ %s, 16)\n", vd, vd, va)
	fmt.Fprintf(b, "  %s = %s\n", vc, add(vc, vd))
	fmt.Fprintf(b, "  %s = rt.int_wrap(5, %s ^ %s, 63)\n", vb, vb, vc)
}

// plus is the index x + k (x alone for 0): the arm64 backend addresses
// x + k from the element base of x (decision 0184 item 6).
func plus(x string, k int) string {
	if k == 0 {
		return x
	}
	return fmt.Sprintf("%s + %d", x, k)
}

// mixRound writes the eight G of a round (columns, then diagonals); m
// names the message word of each of the 16 (empty: Argon2's G).
func mixRound(b *bytes.Buffer, m func(k int) string) {
	gs := [8][4]int{{0, 4, 8, 12}, {1, 5, 9, 13}, {2, 6, 10, 14}, {3, 7, 11, 15}, {0, 5, 10, 15}, {1, 6, 11, 12}, {2, 7, 8, 13}, {3, 4, 9, 14}}
	for k, g := range gs {
		mixG(b, g[0], g[1], g[2], g[3], m(2*k), m(2*k+1))
	}
}

// sha256Rounds writes SHA-256's compression (FIPS 180-4 6.2.2) twice, as
// sha256_block (the 64 bytes of a Bytes from an index, big endian) and
// sha256_words (the first 16 words of a List[Int]): the 64 rounds
// unrolled, the message schedule in 16 locals, the eight working
// variables renamed instead of moved, the constants immediate. Words are
// in the low 32 bits of an Int; the rounds use only rotations of the low
// 32 bits, bitwise operations, and wrapping adds, so the working
// variables are masked only when added into h.
func sha256Rounds(b *bytes.Buffer) {
	rounds := func() {
		names := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
		for k := 0; k < 8; k++ {
			fmt.Fprintf(b, "  let s%d = st[%d]\n", k, k)
			fmt.Fprintf(b, "  var %s = s%d\n", names[k], k)
		}
		for r := 0; r < 64; r++ {
			role := func(k int) string { return names[(k-r%8+8)%8] }
			A, B, C, D, E, F, G, H := role(0), role(1), role(2), role(3), role(4), role(5), role(6), role(7)
			w := fmt.Sprintf("w%d", r%16)
			if r >= 16 {
				x, y := fmt.Sprintf("w%d", (r-15)%16), fmt.Sprintf("w%d", (r-2)%16)
				fmt.Fprintf(b, "  %s = rt.int_wrap(0, rt.int_wrap(0, %s, rt.int_wrap(6, %s, 7) ^ rt.int_wrap(6, %s, 18) ^ (%s >> 3)), rt.int_wrap(0, w%d, rt.int_wrap(6, %s, 17) ^ rt.int_wrap(6, %s, 19) ^ (%s >> 10))) & 0xFFFFFFFF\n",
					w, w, x, x, x, (r-7)%16, y, y, y)
			}
			fmt.Fprintf(b, "  let t%d = rt.int_wrap(0, rt.int_wrap(0, %s, rt.int_wrap(6, %s, 6) ^ rt.int_wrap(6, %s, 11) ^ rt.int_wrap(6, %s, 25)), rt.int_wrap(0, %s ^ (%s & (%s ^ %s)), rt.int_wrap(0, %d, %s)))\n",
				r, H, E, E, E, G, E, F, G, sha256K[r], w)
			fmt.Fprintf(b, "  %s = rt.int_wrap(0, %s, t%d)\n", D, D, r)
			fmt.Fprintf(b, "  %s = rt.int_wrap(0, t%d, rt.int_wrap(0, rt.int_wrap(6, %s, 2) ^ rt.int_wrap(6, %s, 13) ^ rt.int_wrap(6, %s, 22), (%s & %s) | (%s & (%s | %s))))\n",
				H, r, A, A, A, A, B, C, A, B)
		}
		for k := 0; k < 8; k++ {
			fmt.Fprintf(b, "  st[%d] = (st[%d] + (%s & 0xFFFFFFFF)) & 0xFFFFFFFF\n", k, k, names[k])
		}
	}
	b.WriteString("\n/// SHA-256's compression (FIPS 180-4 6.2.2) of the 64 bytes of blk at i\n")
	b.WriteString("/// into the state st (8 words).\n")
	b.WriteString("pub fn sha256_block(var st: List[Int], blk: Bytes, i: Int) {\n")
	for k := 0; k < 8; k++ {
		fmt.Fprintf(b, "  let x%d = rt.int_wrap(7, rt.bytes_word(blk, i + %d), 0)\n", k, 8*k)
		fmt.Fprintf(b, "  var w%d = rt.int_wrap(4, x%d, 32)\n", 2*k, k)
		fmt.Fprintf(b, "  var w%d = x%d & 0xFFFFFFFF\n", 2*k+1, k)
	}
	rounds()
	b.WriteString("}\n")
	b.WriteString("\n/// SHA-256's compression of the block in the first 16 words of m into\n")
	b.WriteString("/// the state st.\n")
	b.WriteString("pub fn sha256_words(var st: List[Int], m: List[Int]) {\n")
	for k := 0; k < 16; k++ {
		fmt.Fprintf(b, "  let m%d = m[%d]\n", k, k)
		fmt.Fprintf(b, "  var w%d = m%d\n", k, k)
	}
	rounds()
	b.WriteString("}\n")
}

// digestTables is the text of std/native/llrt/digest_tables.xo: SHA-256's
// constants, and BLAKE2b's compression and Argon2's permutation written
// out (the rounds unrolled, the state in 16 locals, as x/crypto's
// generic code).
func digestTables() []byte {
	var b bytes.Buffer
	b.WriteString("// Code generated by internal/llvm TestDigestTables -golden-update from the\n")
	b.WriteString("// constants of FIPS 180-4 and RFC 7693 and the rounds of RFC 7693 and RFC\n")
	b.WriteString("// 9106. DO NOT EDIT.\n")
	b.WriteString("//\n")
	b.WriteString("// The tables and unrolled rounds of std/native/llrt/digest.xo (decision\n")
	b.WriteString("// 0195).\n")
	b.WriteString("use native/rt\n")
	sha256Rounds(&b)
	b.WriteString("\n/// BLAKE2b's initialization vector (RFC 7693 2.6).\n")
	b.WriteString("pub fn blake2b_iv() -> List[Int] { [")
	for i, v := range blake2bIV {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%d", int64(v))
	}
	b.WriteString("] }\n")
	b.WriteString("\n/// BLAKE2b's compression F (RFC 7693 3.2) of the 128 byte block of blk\n")
	b.WriteString("/// at i into h, the offset counter t0 (the low word; the high one is 0\n")
	b.WriteString("/// here), the final block when last.\n")
	b.WriteString("pub fn blake2b_compress(var h: List[Int], blk: Bytes, i: Int, t0: Int, last: Bool) {\n")
	for k := 0; k < 16; k++ {
		fmt.Fprintf(&b, "  let m%d = rt.bytes_word(blk, i + %d)\n", k, 8*k)
	}
	for k := 0; k < 8; k++ {
		fmt.Fprintf(&b, "  let h%d = h[%d]\n", k, k)
		fmt.Fprintf(&b, "  var v%d = h%d\n", k, k)
	}
	for k := 0; k < 8; k++ {
		switch k {
		case 4:
			fmt.Fprintf(&b, "  var v%d = %d ^ t0\n", k+8, int64(blake2bIV[k]))
		case 6:
			fmt.Fprintf(&b, "  var v%d = if last { %d } else { %d }\n", k+8, int64(^blake2bIV[k]), int64(blake2bIV[k]))
		default:
			fmt.Fprintf(&b, "  var v%d = %d\n", k+8, int64(blake2bIV[k]))
		}
	}
	for r := 0; r < 12; r++ {
		mixRound(&b, func(k int) string { return fmt.Sprintf("m%d", blake2bSigma[r][k]) })
	}
	for k := 0; k < 8; k++ {
		fmt.Fprintf(&b, "  h[%d] = h[%d] ^ v%d ^ v%d\n", k, k, k, k+8)
	}
	b.WriteString("}\n")
	b.WriteString("\n/// a + b + 2 * lo(a) * lo(b), wrapping (fBlaMka, RFC 9106 3.6).\n")
	b.WriteString("fn fbm(a: Int, b: Int) -> Int { rt.int_wrap(0, rt.int_wrap(0, a, b), rt.int_wrap(3, rt.int_wrap(2, a & 0xFFFFFFFF, b & 0xFFFFFFFF), 1)) }\n")
	for _, kind := range []string{"row", "column"} {
		idx := func(k int) string {
			if kind == "row" {
				return plus("i", k)
			}
			return plus("i", 16*(k/2)+k%2)
		}
		if kind == "row" {
			b.WriteString("\n/// Argon2's permutation P (RFC 9106 3.6) on the 16 words of t from i.\n")
			b.WriteString("pub fn blamka_row(var t: List[Int], i: Int) {\n")
		} else {
			b.WriteString("\n/// Argon2's permutation P on the word pairs of t from i, 16 words apart.\n")
			b.WriteString("pub fn blamka_column(var t: List[Int], i: Int) {\n")
		}
		// the bounds once, so that the 32 accesses are not checked
		// (decision 0184 item 3: i + c under i + last < len)
		fmt.Fprintf(&b, "  if i < 0 || %s >= t.len() {\n    rt.fault_raise(\"digest: block index out of range\", \"\")\n  }\n", idx(15))
		for k := 0; k < 16; k++ {
			fmt.Fprintf(&b, "  let x%d = t[%s]\n", k, idx(k))
			fmt.Fprintf(&b, "  var v%d = x%d\n", k, k)
		}
		mixRound(&b, func(int) string { return "" })
		for k := 0; k < 16; k++ {
			fmt.Fprintf(&b, "  t[%s] = v%d\n", idx(k), k)
		}
		b.WriteString("}\n")
	}
	// Argon2's G on three blocks of the memory, written out so that the
	// accesses are checked once
	b.WriteString("\n/// G of RFC 9106 3.5 on the blocks of bs at the words a and b (128 words\n")
	b.WriteString("/// each) into the block at o: bs[o] = (or ^=, when xor) R ^ P(R) for R =\n")
	b.WriteString("/// bs[a] ^ bs[b]; t and q are scratch of 128 words.\n")
	b.WriteString("pub fn argon2_g(var bs: List[Int], var t: List[Int], var q: List[Int], o: Int, a: Int, b: Int, xor: Bool) {\n")
	b.WriteString("  let n = bs.len()\n")
	b.WriteString("  if o < 0 || a < 0 || b < 0 || o + 127 >= n || a + 127 >= n || b + 127 >= n || 127 >= t.len() || 127 >= q.len() {\n")
	b.WriteString("    rt.fault_raise(\"digest: block index out of range\", \"\")\n  }\n")
	for k := 0; k < 128; k++ {
		fmt.Fprintf(&b, "  let r%d = bs[%s] ^ bs[%s]\n", k, plus("a", k), plus("b", k))
		fmt.Fprintf(&b, "  q[%d] = r%d\n", k, k)
		fmt.Fprintf(&b, "  t[%d] = r%d\n", k, k)
	}
	b.WriteString("  var i = 0\n  while i < 128 {\n    blamka_row(t, i)\n    i += 16\n  }\n")
	b.WriteString("  i = 0\n  while i < 16 {\n    blamka_column(t, i)\n    i += 2\n  }\n")
	// t came back from the calls: its length checked again once, so
	// that the 128 reads of t below are not checked (decision 0208); bs
	// and q too, since LLVM cannot tell that the calls leave their
	// lengths alone (decision 0211)
	b.WriteString("  // bs and q again: LLVM cannot tell that the calls above leave their\n")
	b.WriteString("  // lengths alone, so without this its checks of the indexes below stay\n")
	b.WriteString("  // (decision 0211)\n")
	b.WriteString("  if 127 >= t.len() || o + 127 >= bs.len() || 127 >= q.len() {\n    rt.fault_raise(\"digest: block index out of range\", \"\")\n  }\n")
	b.WriteString("  if xor {\n")
	for k := 0; k < 128; k++ {
		fmt.Fprintf(&b, "    bs[%s] = bs[%s] ^ q[%d] ^ t[%d]\n", plus("o", k), plus("o", k), k, k)
	}
	b.WriteString("  } else {\n")
	for k := 0; k < 128; k++ {
		fmt.Fprintf(&b, "    bs[%s] = q[%d] ^ t[%d]\n", plus("o", k), k, k)
	}
	b.WriteString("  }\n}\n")
	return b.Bytes()
}

// TestDigestTables checks that std/native/llrt/digest_tables.xo is
// digestTables (-golden-update rewrites it).
func TestDigestTables(t *testing.T) {
	want := digestTables()
	if *updateGolden {
		if err := os.WriteFile(digestTablesPath, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(digestTablesPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s is stale: run go test ./internal/llvm -run TestDigestTables -golden-update", digestTablesPath)
	}
}

// digestDiffProg reads lines `s <data>`, `h <key> <msg>`, and `v
// <password> <hash>` (byte strings in hex) and prints, for each,
// crypto.sha256, crypto.hmac_sha256 (in hex), and crypto.verify_password.
const digestDiffProg = `use std/crypto

fn main(os: Os) {
  let input = match os.stdio.read_all() {
    Ok(s) => s
    Err(_) => ""
  }
  var out: List[Str] = []
  for line in input.lines() {
    let f = line.split(" ")
    let a = Bytes.from_hex(if f.len() > 1 { f[1] } else { "" }) ?? b"?"
    let b = Bytes.from_hex(if f.len() > 2 { f[2] } else { "" }) ?? b"?"
    match f[0] {
      "s" => out.push(crypto.sha256(a).to_hex())
      "h" => out.push(crypto.hmac_sha256(a, b).to_hex())
      "v" => out.push("${crypto.verify_password(a.to_str() ?? "?", b.to_str() ?? "?")}")
      _ => out.push("?")
    }
  }
  os.stdio.println(out.join("\n"))
}
`

// digestCorpus is the input of digestDiffProg and the lines Go's crypto
// packages and std/rt's verify_password give: SHA-256 and HMAC of random
// data (keys longer and shorter than a block, data across the padding
// boundaries), and verify_password on the x/crypto Argon2id outputs of
// std/rt/testdata/argon2id_xcrypto.txt and PBKDF2 hashes of Go's
// crypto/pbkdf2 (keys of one to four blocks), each also mutated.
func digestCorpus(t *testing.T) (string, []string) {
	r := rand.New(rand.NewSource(3))
	rb := func(n int) []byte {
		b := make([]byte, n)
		r.Read(b)
		return b
	}
	// a password Str: printable ASCII and a few multibyte runes
	rs := func(n int) string {
		var b strings.Builder
		for range n {
			if r.Intn(10) == 0 {
				b.WriteString("é")
			} else {
				b.WriteByte(byte(33 + r.Intn(94)))
			}
		}
		return b.String()
	}
	var in strings.Builder
	var want []string
	for i := 0; i < 600; i++ {
		n := r.Intn(300)
		if i%50 == 0 {
			n = 1000 + r.Intn(9000)
		}
		if i < 140 {
			// every length to 140: the padding's one and two blocks
			n = i
		}
		data := rb(n)
		sum := sha256.Sum256(data)
		in.WriteString("s " + hex.EncodeToString(data) + "\n")
		want = append(want, hex.EncodeToString(sum[:]))
		key := rb(r.Intn(200))
		m := hmac.New(sha256.New, key)
		m.Write(data)
		in.WriteString("h " + hex.EncodeToString(key) + " " + hex.EncodeToString(data) + "\n")
		want = append(want, hex.EncodeToString(m.Sum(nil)))
	}
	var hashes [][2]string
	f, err := os.Open(filepath.Join("..", "..", "std", "rt", "testdata", "argon2id_xcrypto.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		fs := strings.Split(sc.Text(), " ")
		pw, _ := hex.DecodeString(fs[0])
		salt, _ := hex.DecodeString(fs[1])
		key, _ := hex.DecodeString(fs[6])
		if !utf8.Valid(pw) {
			continue
		}
		enc := base64.RawStdEncoding
		hashes = append(hashes, [2]string{string(pw), "$argon2id$v=19$m=" + fs[3] + ",t=" + fs[2] + ",p=" + fs[4] + "$" + enc.EncodeToString(salt) + "$" + enc.EncodeToString(key)})
	}
	for i := 0; i < 40; i++ {
		pw, salt := rs(r.Intn(30)), rb(r.Intn(24))
		iter := 1 + r.Intn(300)
		kl := 1 + r.Intn(128)
		k, err := pbkdf2.Key(sha256.New, pw, salt, iter, kl)
		if err != nil {
			t.Fatal(err)
		}
		enc := base64.RawStdEncoding
		hashes = append(hashes, [2]string{pw, "pbkdf2-sha256$" + strconv.Itoa(iter) + "$" + enc.EncodeToString(salt) + "$" + enc.EncodeToString(k)})
	}
	alphabet := []byte("$=,\n\r+-0123456789aZ/_mtpv")
	verify := func(pw, h string) {
		in.WriteString("v " + hex.EncodeToString([]byte(pw)) + " " + hex.EncodeToString([]byte(h)) + "\n")
		want = append(want, strconv.FormatBool(rt.Crypto_verify_password(nil, pw, h)))
	}
	for _, h := range hashes {
		verify(h[0], h[1])
		for j := 0; j < 6; j++ {
			s := []byte(h[1])
			switch r.Intn(4) {
			case 0:
				k := r.Intn(len(s))
				s = append(s[:k:k], s[k+1:]...)
			case 1:
				k := r.Intn(len(s) + 1)
				s = append(s[:k:k], append([]byte{alphabet[r.Intn(len(alphabet))]}, s[k:]...)...)
			case 2:
				s[r.Intn(len(s))] = alphabet[r.Intn(len(alphabet))]
			case 3:
				s = s[:r.Intn(len(s))]
			}
			pw := h[0]
			if r.Intn(8) == 0 {
				pw += "x"
			}
			if utf8.Valid(s) {
				verify(pw, string(s))
			}
		}
	}
	return in.String(), want
}

// TestDigestDiff runs digestDiffProg over digestCorpus on both native
// backends, with the hashes in Xo (std/native/llrt/digest.xo, decision
// 0195) and with their C path (XO_LLRT_C=digest), and requires the lines
// of Go's crypto packages and std/rt's verify_password.
func TestDigestDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds four programs")
	}
	in, want := digestCorpus(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(digestDiffProg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stdin.txt"), []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d input lines", len(want))
	for _, path := range []string{"xo", "c"} {
		for _, be := range []string{xotool.BackendArm64, xotool.BackendLLVM} {
			t.Run(path+"/"+be, func(t *testing.T) {
				if path == "c" {
					t.Setenv("XO_LLRT_C", "digest")
				}
				r := buildAndRun(t, dir, xotool.Options{Backend: be})
				if r.exit != 0 {
					t.Fatalf("exit %d: %s", r.exit, r.stderr)
				}
				diffLines(t, strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n"), want)
			})
		}
	}
}
