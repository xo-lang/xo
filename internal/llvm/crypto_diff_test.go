package llvm_test

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/std/rt"
)

// TestCryptoDiff runs std/llrt/testdata/crypto_diff.c (with
// AddressSanitizer and UBSan when clang has them) and compares the
// native std/crypto (xo_crypto.c, decision 0083) with Go: SHA-256, HMAC,
// PBKDF2, and base64 of random inputs against Go's packages; Argon2
// against the RFC 9106 section 5 vectors (Argon2d, Argon2i, Argon2id
// with a secret and associated data) and the x/crypto outputs of
// std/rt/testdata/argon2id_xcrypto.txt; verify_password on valid and
// mutated PHC and PBKDF2 strings against std/rt's.
func TestCryptoDiff(t *testing.T) {
	t.Parallel() // builds nothing in the shared build directory
	if _, err := build.Clang(); err != nil {
		t.Skip("clang not found")
	}
	src := filepath.Join(rtDir, "testdata", "crypto_diff.c")
	bin := filepath.Join(t.TempDir(), "crypto_diff")
	if err := build.CompileC([]string{"-O1", "-g", "-fsanitize=address,undefined"}, []string{src}, bin); err != nil {
		if err2 := build.CompileC([]string{"-O1", "-g"}, []string{src}, bin); err2 != nil {
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
		other := append([]byte(nil), data...)
		if len(other) > 0 && r.Intn(2) == 0 {
			other[r.Intn(len(other))] ^= byte(1 + r.Intn(255))
		}
		cs = append(cs, kase{"equal " + hx(data) + " " + hx(other), bit(subtle.ConstantTimeCompare(data, other) == 1)})
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
