package llvm

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
	"github.com/xo-lang/xo/std/rt"
)

// TestCryptoDiff runs std/llrt/testdata/crypto_diff.c (with
// AddressSanitizer and UBSan when clang has them) and compares the
// native std/crypto (xo_crypto.c, decision 0083) with Go: SHA-256, HMAC,
// PBKDF2, and base64 of random inputs against Go's packages; Argon2
// against the RFC 9106 section 5 vectors (Argon2d, Argon2i, Argon2id
// with a secret and associated data) and the x/crypto outputs of
// std/rt/testdata/argon2id_xcrypto.txt; verify_password on valid and
// mutated PHC and PBKDF2 strings against std/rt's. crypto.equal and
// Ed25519 are Xo (decision 0222): TestCryptoXoDiff.
func TestCryptoDiff(t *testing.T) {
	t.Parallel() // builds nothing in the shared build directory
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	src := filepath.Join(rtDir, "testdata", "crypto_diff.c")
	bin := filepath.Join(t.TempDir(), "crypto_diff")
	// a program that uses std/crypto, for the runtime's C ABI exports
	// (crypto_ref.c calls xo_x_b64_val)
	srcs := cOnlyWith(t, src, "cexports-crypto", "use std/crypto\n\nfn main(os: Os) {\n  if crypto.ed25519_verify(b\"\", b\"\", b\"\") {\n    os.stdio.println(\"yes\")\n  }\n}\n")
	if err := xotool.CompileC(asanFlags("-O1", "-g", "-fsanitize=address,undefined"), srcs, bin); err != nil {
		if err2 := xotool.CompileC([]string{"-O1", "-g"}, srcs, bin); err2 != nil {
			t.Fatal(err2)
		}
	}
	hx := func(b []byte) string {
		if len(b) == 0 {
			return "-"
		}
		return hex.EncodeToString(b)
	}
	bit := func(ok bool) string {
		if ok {
			return "1"
		}
		return "0"
	}
	r := rand.New(rand.NewSource(1))
	rb := func(n int) []byte {
		b := make([]byte, n)
		r.Read(b)
		return b
	}
	type kase struct{ in, want string }
	var cs []kase
	n := 400
	if testing.Short() {
		n = 60
	}
	for i := 0; i < n; i++ {
		data := rb(r.Intn(300))
		sum := sha256.Sum256(data)
		cs = append(cs, kase{"sha256 " + hx(data), hx(sum[:])})
		key := rb(r.Intn(200))
		m := hmac.New(sha256.New, key)
		m.Write(data)
		cs = append(cs, kase{"hmac " + hx(key) + " " + hx(data), hx(m.Sum(nil))})
		url := r.Intn(2)
		enc := base64.RawStdEncoding
		if url == 1 {
			enc = base64.RawURLEncoding
		}
		cs = append(cs, kase{fmt.Sprintf("b64 %d %s", url, hx(data)), hx([]byte(enc.EncodeToString(data)))})
	}
	for i := 0; i < n/10; i++ {
		pw, salt := rb(r.Intn(100)), rb(r.Intn(40))
		iter, kl := 1+r.Intn(40), 1+r.Intn(100)
		k, err := pbkdf2.Key(sha256.New, string(pw), salt, iter, kl)
		if err != nil {
			t.Fatal(err)
		}
		cs = append(cs, kase{fmt.Sprintf("pbkdf2 %s %s %d %d", hx(pw), hx(salt), iter, kl), hx(k)})
	}
	// RFC 9106 section 5: password 32 x 0x01, salt 16 x 0x02, secret 8 x
	// 0x03, associated data 12 x 0x04, t=3, m=32, p=4, 32 byte tag
	rfc := fmt.Sprintf("%s %s %s %s 3 32 4 32", hx(bytes.Repeat([]byte{1}, 32)), hx(bytes.Repeat([]byte{2}, 16)), hx(bytes.Repeat([]byte{3}, 8)), hx(bytes.Repeat([]byte{4}, 12)))
	cs = append(cs,
		kase{"argon2 0 " + rfc, "512b391b6f1162975371d30919734294f868e3be3984f3c1a13a4db9fabe4acb"},
		kase{"argon2 1 " + rfc, "c814d9d1dc7f37aa13f0d77f2494bda1c8de6b016dd388d29952a4c4672b6ce8"},
		kase{"argon2 2 " + rfc, "0d640df58d78766c08c037a34a8b53c9d01ef0452d75b65eb52520e96b01e659"},
	)
	// x/crypto's IDKey outputs, and PHC strings made of them
	f, err := os.Open(filepath.Join("..", "..", "std", "rt", "testdata", "argon2id_xcrypto.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var hashes []struct{ pw, hash string }
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		fs := strings.Split(sc.Text(), " ")
		if len(fs) != 7 {
			t.Fatalf("bad line %q", sc.Text())
		}
		pw, _ := hex.DecodeString(fs[0])
		salt, _ := hex.DecodeString(fs[1])
		key, _ := hex.DecodeString(fs[6])
		cs = append(cs, kase{fmt.Sprintf("argon2 2 %s %s - - %s %s %s %s", hx(pw), hx(salt), fs[2], fs[3], fs[4], fs[5]), fs[6]})
		enc := base64.RawStdEncoding
		hashes = append(hashes, struct{ pw, hash string }{string(pw), "$argon2id$v=19$m=" + fs[3] + ",t=" + fs[2] + ",p=" + fs[4] + "$" + enc.EncodeToString(salt) + "$" + enc.EncodeToString(key)})
	}
	for i := 0; i < 20; i++ {
		pw, salt := rb(r.Intn(30)), rb(r.Intn(24))
		iter := 1 + r.Intn(30)
		k, _ := pbkdf2.Key(sha256.New, string(pw), salt, iter, 1+r.Intn(48))
		enc := base64.RawStdEncoding
		hashes = append(hashes, struct{ pw, hash string }{string(pw), "pbkdf2-sha256$" + strconv.Itoa(iter) + "$" + enc.EncodeToString(salt) + "$" + enc.EncodeToString(k)})
	}
	alphabet := []byte("$=,\n\r+-0123456789aZ/_mtpv")
	for _, h := range hashes {
		cs = append(cs, kase{"verify " + hx([]byte(h.pw)) + " " + hx([]byte(h.hash)), bit(rt.Crypto_verify_password(nil, h.pw, h.hash))})
		for j := 0; j < 6; j++ {
			s := []byte(h.hash)
			switch r.Intn(4) {
			case 0:
				if len(s) > 0 {
					k := r.Intn(len(s))
					s = append(s[:k:k], s[k+1:]...)
				}
			case 1:
				k := r.Intn(len(s) + 1)
				s = append(s[:k:k], append([]byte{alphabet[r.Intn(len(alphabet))]}, s[k:]...)...)
			case 2:
				if len(s) > 0 {
					s[r.Intn(len(s))] = alphabet[r.Intn(len(alphabet))]
				}
			case 3:
				if len(s) > 0 {
					s = s[:r.Intn(len(s))]
				}
			}
			pw := h.pw
			if r.Intn(8) == 0 {
				pw += "x"
			}
			cs = append(cs, kase{"verify " + hx([]byte(pw)) + " " + hx(s), bit(rt.Crypto_verify_password(nil, pw, string(s)))})
		}
	}
	var in bytes.Buffer
	for _, c := range cs {
		in.WriteString(c.in + "\n")
	}
	cmd := exec.Command(bin)
	cmd.Stdin = &in
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("crypto_diff: %v\n%s", err, stderr.String())
	}
	got := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(got) != len(cs) {
		t.Fatalf("%d output lines for %d inputs\n%s", len(got), len(cs), stderr.String())
	}
	bad := 0
	for i, c := range cs {
		if got[i] != c.want && bad < 20 {
			bad++
			t.Errorf("%s:\n got %s\nwant %s", c.in, got[i], c.want)
		}
	}
	t.Logf("%d operations agree", len(cs))
}

// ed25519Cases are the Ed25519 operations of TestCryptoXoDiff
// (decision 0189): the RFC 8032 section 7.1 vectors, then random seeds
// and messages signed and verified as Go's crypto/ed25519 does, with
// mutated signatures, keys, and messages, random public keys (about
// half are no point), non canonical encodings of A (y of p or more,
// x = 0 with the sign bit), S of L or more, and wrong lengths.
func ed25519Cases(r *rand.Rand, n int) []struct{ in, want string } {
	type kase = struct{ in, want string }
	hx := func(b []byte) string {
		if len(b) == 0 {
			return "-"
		}
		return hex.EncodeToString(b)
	}
	bit := func(ok bool) string {
		if ok {
			return "1"
		}
		return "0"
	}
	rb := func(k int) []byte {
		b := make([]byte, k)
		r.Read(b)
		return b
	}
	var cs []kase
	for _, v := range [][4]string{
		{"9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60", "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a", "-", "e5564300c360ac729086e2cc806e828a84877f1eb8e5d974d873e065224901555fb8821590a33bacc61e39701cf9b46bd25bf5f0595bbe24655141438e7a100b"},
		{"4ccd089b28ff96da9db6c346ec114e0f5b8a319f35aba624da8cf6ed4fb8a6fb", "3d4017c3e843895a92b70aa74d1b7ebc9c982ccf2ec4968cc0cd55f12af4660c", "72", "92a009a9f0d4cab8720e820b5f642540a2b27b5416503f8fb3762223ebdb69da085ac1e43e15996e458f3613d0f11d8c387b2eaeb4302aeeb00d291612bb0c00"},
		{"c5aa8df43f9f837bedb7442f31dcb7b166d38535076f094b85ce3a2e0b4458f7", "fc51cd8e6218a1a38da47ed00230f0580816ed13ba3303ac5deb911548908025", "af82", "6291d657deec24024827e69c3abe01a30ce548a284743a445e3680d7db5ac3ac18ff9b538d16f290ae67f760984dc6594a7c15e9716ed28dc027beceea1ec40a"},
	} {
		cs = append(cs, kase{"edpub " + v[0], v[1]}, kase{"edsign " + v[0] + " " + v[2], v[3]}, kase{"edverify " + v[1] + " " + v[2] + " " + v[3], "1"})
	}
	// messages whose SHA-512 inputs (32 byte prefix + message, 64 bytes
	// of R and A + message) end around the block and padding boundaries
	// of SHA-512 (std/native/llrt/digest.xo's sha512_le)
	for _, k := range []int{47, 48, 49, 63, 64, 65, 79, 80, 81, 95, 96, 97, 111, 112, 113, 127, 128, 129, 175, 176, 177, 191, 192, 193, 207, 208, 209, 223, 224, 225, 1000} {
		seed, msg := rb(32), rb(k)
		priv := ed25519.NewKeyFromSeed(seed)
		sig := ed25519.Sign(priv, msg)
		cs = append(cs, kase{"edsign " + hx(seed) + " " + hx(msg), hx(sig)}, kase{"edverify " + hx([]byte(priv.Public().(ed25519.PublicKey))) + " " + hx(msg) + " " + hx(sig), "1"})
	}
	verify := func(pub, msg, sig []byte) kase {
		ok := len(pub) == ed25519.PublicKeySize && len(sig) == ed25519.SignatureSize && ed25519.Verify(ed25519.PublicKey(pub), msg, sig)
		return kase{"edverify " + hx(pub) + " " + hx(msg) + " " + hx(sig), bit(ok)}
	}
	for i := 0; i < n/2; i++ {
		seed := rb(32)
		msg := rb(r.Intn(300))
		priv := ed25519.NewKeyFromSeed(seed)
		pub := []byte(priv.Public().(ed25519.PublicKey))
		sig := ed25519.Sign(priv, msg)
		cs = append(cs, kase{"edpub " + hx(seed), hx(pub)}, kase{"edsign " + hx(seed) + " " + hx(msg), hx(sig)}, verify(pub, msg, sig))
		bad := append([]byte(nil), sig...)
		bad[r.Intn(64)] ^= byte(1 << r.Intn(8))
		cs = append(cs, verify(pub, msg, bad))
		if len(msg) > 0 {
			m2 := append([]byte(nil), msg...)
			m2[r.Intn(len(m2))] ^= 1
			cs = append(cs, verify(pub, m2, sig))
		}
		p2 := append([]byte(nil), pub...)
		p2[r.Intn(32)] ^= byte(1 << r.Intn(8))
		cs = append(cs, verify(p2, msg, sig), verify(rb(32), msg, sig), verify(pub[:31], msg, sig), verify(pub, msg, sig[:63]))
		// S + L: the same point equation, refused as non canonical
		s := new(big.Int).SetBytes(reverse(sig[32:]))
		s.Add(s, edL)
		if s.BitLen() <= 256 {
			high := append(append([]byte(nil), sig[:32]...), reverse(leftPad(s.Bytes(), 32))...)
			cs = append(cs, verify(pub, msg, high))
		}
	}
	// non canonical encodings of A: y = p + j (j < 19), both sign bits,
	// and x = 0 with the sign bit (y = 1 and y = p - 1)
	p := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(19))
	sig := ed25519.Sign(ed25519.NewKeyFromSeed(rb(32)), []byte("m"))
	for j := int64(0); j < 19; j++ {
		y := reverse(leftPad(new(big.Int).Add(p, big.NewInt(j)).Bytes(), 32))
		for _, sign := range []byte{0, 0x80} {
			a := append([]byte(nil), y...)
			a[31] |= sign
			cs = append(cs, verify(a, []byte("m"), sig))
		}
	}
	for _, y := range []*big.Int{big.NewInt(1), new(big.Int).Sub(p, big.NewInt(1))} {
		a := reverse(leftPad(y.Bytes(), 32))
		a[31] |= 0x80
		cs = append(cs, verify(a, []byte("m"), sig))
	}
	return cs
}

// edL is the order of the Ed25519 base point.
var edL, _ = new(big.Int).SetString("7237005577332262213973186563042994240857116359379907606001950938285454250989", 10)

func reverse(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[i] = b[len(b)-1-i]
	}
	return out
}

func leftPad(b []byte, n int) []byte {
	if len(b) >= n {
		return b
	}
	return append(make([]byte, n-len(b)), b...)
}
