// A fuzz harness for std/hpke against Go's crypto/hpke, for decision
// 0616's fuzz kind (task #99). Each case derives a recipient key from
// seeded bytes for a KEM, KDF and AEAD of Go's (every identifier, the
// export-only AEAD too), lets Go's Sender seal two messages with their
// additional data (testing/cryptotest makes the ephemeral keys follow
// the seed), then changes what an attacker could: the encapsulated key
// (a bit flipped, cut, a byte added, random bytes), a ciphertext (a bit
// flipped, cut, extended), the info or the additional data, or the order
// the messages arrive in; and changes the recipient's public key bytes
// for NewPublicKey. Xo must answer as Go's NewRecipient, both Opens in
// order, an Export (lengths past the KDF's limit too) and NewPublicKey:
// the same bytes or the same error text. TestHpkeFuzz runs a seed
// (XO_HPKEFUZZ_SEED, default 2; XO_HPKEFUZZ_N cases, default 400) on
// XO_HPKEFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"crypto/hpke"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"testing/cryptotest"
)

var hpfKEMs = []uint16{0x0010, 0x0011, 0x0012, 0x0020, 0x0041, 0x0042, 0x647a, 0x0050, 0x0051}
var hpfKDFs = []uint16{0x0001, 0x0002, 0x0003, 0x0010, 0x0011}
var hpfAEADs = []uint16{0x0001, 0x0002, 0x0003, 0xffff}

// hpfChanges names the change of each case, for the coverage check.
var hpfChanges = []string{"none", "enc bit", "enc cut", "enc longer", "enc random", "ct bit", "ct cut", "ct longer", "info", "aad", "order"}

type hpfCase struct {
	kem, kdf, aead                       uint16
	ikm, enc, info, aad1, ct1, aad2, ct2 []byte
	ctx                                  []byte
	n                                    int
	pk                                   []byte
	change                               int
	want                                 string
}

func hpfAnswer(b []byte, err error) string {
	if err != nil {
		return "err " + err.Error()
	}
	return "ok " + hex.EncodeToString(b)
}

func hpfBytes(r *rand.Rand, lo, hi int) []byte {
	b := make([]byte, lo+r.Intn(hi-lo+1))
	r.Read(b)
	return b
}

// hpfMutate changes b as change says: a bit flipped, cut, or longer.
func hpfMutate(r *rand.Rand, b []byte, how int) []byte {
	c := append([]byte(nil), b...)
	switch how {
	case 0:
		if len(c) > 0 {
			c[r.Intn(len(c))] ^= 1 << r.Intn(8)
		}
	case 1:
		if len(c) > 0 {
			c = c[:r.Intn(len(c))]
		}
	case 2:
		c = append(c, byte(r.Intn(256)))
	}
	return c
}

func hpfCases(t *testing.T, seed int64, n int) []hpfCase {
	cryptotest.SetGlobalRandom(t, uint64(seed))
	r := rand.New(rand.NewSource(seed))
	var cases []hpfCase
	for len(cases) < n {
		c := hpfCase{kem: hpfKEMs[r.Intn(len(hpfKEMs))], kdf: hpfKDFs[r.Intn(len(hpfKDFs))], aead: hpfAEADs[r.Intn(len(hpfAEADs))]}
		kem, _ := hpke.NewKEM(c.kem)
		kdf, _ := hpke.NewKDF(c.kdf)
		aead, _ := hpke.NewAEAD(c.aead)
		c.ikm = hpfBytes(r, 32, 64)
		k, err := kem.DeriveKeyPair(c.ikm)
		if err != nil {
			t.Fatalf("DeriveKeyPair: %v", err)
		}
		c.info = hpfBytes(r, 0, 40)
		enc, s, err := hpke.NewSender(k.PublicKey(), kdf, aead, c.info)
		if err != nil {
			t.Fatalf("NewSender: %v", err)
		}
		c.enc = enc
		c.aad1, c.aad2 = hpfBytes(r, 0, 30), hpfBytes(r, 0, 30)
		if c.aead != 0xffff {
			c.ct1, _ = s.Seal(c.aad1, hpfBytes(r, 0, 300))
			c.ct2, _ = s.Seal(c.aad2, hpfBytes(r, 0, 300))
		} else {
			c.ct1, c.ct2 = hpfBytes(r, 0, 40), hpfBytes(r, 0, 40)
		}
		c.change = r.Intn(len(hpfChanges))
		switch c.change {
		case 1, 2, 3:
			c.enc = hpfMutate(r, c.enc, c.change-1)
		case 4:
			c.enc = hpfBytes(r, len(c.enc), len(c.enc))
		case 5, 6, 7:
			c.ct1 = hpfMutate(r, c.ct1, c.change-5)
		case 8:
			c.info = hpfMutate(r, c.info, 2)
		case 9:
			c.aad1 = hpfMutate(r, c.aad1, 2)
		case 10:
			c.ct1, c.ct2, c.aad1, c.aad2 = c.ct2, c.ct1, c.aad2, c.aad1
		}
		c.ctx = hpfBytes(r, 0, 20)
		switch r.Intn(4) {
		case 0:
			c.n = 0
		case 1:
			c.n = 1 + r.Intn(64)
		case 2:
			c.n = 255*64 + r.Intn(3) - 1 // the HKDF-SHA512 limit, either side
		default:
			c.n = 255*32 + r.Intn(3) - 1 // the HKDF-SHA256 limit, either side
		}
		pk := k.PublicKey().Bytes()
		switch r.Intn(4) {
		case 0:
			c.pk = pk
		default:
			c.pk = hpfMutate(r, pk, r.Intn(3))
		}
		var w strings.Builder
		pkAns := "err"
		if p, err := kem.NewPublicKey(c.pk); err != nil {
			pkAns = "err " + err.Error()
		} else {
			pkAns = "ok " + hex.EncodeToString(p.Bytes())
		}
		rec, err := hpke.NewRecipient(c.enc, k, kdf, aead, c.info)
		if err != nil {
			fmt.Fprintf(&w, "rec err %s; pk %s", err.Error(), pkAns)
		} else {
			a, aerr := rec.Open(c.aad1, c.ct1)
			b, berr := rec.Open(c.aad2, c.ct2)
			x, xerr := rec.Export(string(c.ctx), c.n)
			fmt.Fprintf(&w, "open %s; open %s; export %s; pk %s", hpfAnswer(a, aerr), hpfAnswer(b, berr), hpfAnswer(x, xerr), pkAns)
		}
		c.want = w.String()
		cases = append(cases, c)
	}
	return cases
}

const hpfCheckXo = `use std/hpke

fn ans(r: Result[Bytes, hpke.HpkeErr]) -> Str {
  match r {
    Ok(b) => "ok ${b.to_hex()}"
    Err(e) => "err ${e.display()}"
  }
}

fn hx(s: Str) -> Bytes { Bytes.from_hex(s).or_fault("hex") }

fn answer(kem: Int, kdf: Int, aead: Int, ikm: Str, enc: Str, info: Str, aad1: Str, ct1: Str, aad2: Str, ct2: Str, ctx: Str, n: Int, pk: Str) -> Str {
  let k = hpke.new_kem(kem).or_fault("kem")
  let key = k.derive_key_pair(hx(ikm)).or_fault("derive")
  let kd = hpke.new_kdf(kdf).or_fault("kdf")
  let ae = hpke.new_aead(aead).or_fault("aead")
  let pka = match k.public_key(hx(pk)) {
    Ok(p) => "ok ${p.bytes().to_hex()}"
    Err(e) => "err ${e.display()}"
  }
  match hpke.new_recipient(hx(enc), key, kd, ae, hx(info)) {
    Err(e) => "rec err ${e.display()}; pk ${pka}"
    Ok(r0) => {
      var r = r0
      let a = ans(r.open(hx(aad1), hx(ct1)))
      let b = ans(r.open(hx(aad2), hx(ct2)))
      let x = ans(r.export(hx(ctx), n))
      "open ${a}; open ${b}; export ${x}; pk ${pka}"
    }
  }
}

test "std/hpke opens and exports as Go's crypto/hpke" {
  var bad: List[Str] = []
  var i = 0
  for (kem, kdf, aead, ikm, enc, info, aad1, ct1, aad2, ct2, ctx, n, pk, want) in CASES {
    let got = answer(kem, kdf, aead, ikm, enc, info, aad1, ct1, aad2, ct2, ctx, n, pk)
    if got != want {
      bad.push("case ${i} (kem ${kem} kdf ${kdf} aead ${aead}): got ${got}\n  want ${want}")
    }
    i += 1
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${CASES.len()} cases differ, first: ${bad[0]}")
  }
}
`

func hpfXoFile(seed int64, cases []hpfCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/hpkefuzz_test.go (seed %d, %d cases) from Go's crypto/hpke.\n", seed, len(cases))
	b.WriteString(hpfCheckXo)
	b.WriteString("\n/// (kem, kdf, aead, ikm, enc, info, aad1, ct1, aad2, ct2, export context, length, public key; hex) and Go's answer\nconst CASES: List[(Int, Int, Int, Str, Str, Str, Str, Str, Str, Str, Str, Int, Str, Str)] = [\n")
	h := hex.EncodeToString
	for _, c := range cases {
		fmt.Fprintf(&b, "  (%d, %d, %d, %q, %q, %q, %q, %q, %q, %q, %q, %d, %q, %q),\n", c.kem, c.kdf, c.aead, h(c.ikm), h(c.enc), h(c.info), h(c.aad1), h(c.ct1), h(c.aad2), h(c.ct2), h(c.ctx), c.n, h(c.pk), c.want)
	}
	b.WriteString("]\n")
	return b.String()
}

// TestHpkeFuzzCoverage: the cases reach every KEM, KDF, AEAD and change,
// and both a recipient error and a message that opens.
func TestHpkeFuzzCoverage(t *testing.T) {
	cases := hpfCases(t, 1, 400)
	kems, kdfs, aeads, changes := map[uint16]bool{}, map[uint16]bool{}, map[uint16]bool{}, map[int]bool{}
	rec, opened, failed := 0, 0, 0
	for _, c := range cases {
		kems[c.kem], kdfs[c.kdf], aeads[c.aead], changes[c.change] = true, true, true, true
		switch {
		case strings.HasPrefix(c.want, "rec err"):
			rec++
		case strings.HasPrefix(c.want, "open ok"):
			opened++
		case strings.HasPrefix(c.want, "open err"):
			failed++
		}
	}
	if len(kems) != len(hpfKEMs) || len(kdfs) != len(hpfKDFs) || len(aeads) != len(hpfAEADs) || len(changes) != len(hpfChanges) || rec == 0 || opened == 0 || failed == 0 {
		t.Fatalf("coverage: %d KEMs, %d KDFs, %d AEADs, %d changes; %d recipient errors, %d opened, %d failed", len(kems), len(kdfs), len(aeads), len(changes), rec, opened, failed)
	}
}

// TestHpkeFuzz runs a fresh seed's cases.
func TestHpkeFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the hpke fuzz is a heavy test")
	}
	seed := int64(envInt(t, "XO_HPKEFUZZ_SEED", 2))
	n := envInt(t, "XO_HPKEFUZZ_N", 400)
	cases := hpfCases(t, seed, n)
	runFuzzDir(t, "hpkefuzz_test.xo", hpfXoFile(seed, cases), envBackends("XO_HPKEFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
