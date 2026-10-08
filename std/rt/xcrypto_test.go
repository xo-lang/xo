package rt

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"testing"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestArgon2RFC9106 checks the vendored Argon2 against the test vectors
// of RFC 9106 section 5 (password 32 x 0x01, salt 16 x 0x02, secret 8 x
// 0x03, associated data 12 x 0x04, m=32 KiB, t=3, p=4, 32 byte tag).
func TestArgon2RFC9106(t *testing.T) {
	pw := bytes.Repeat([]byte{1}, 32)
	salt := bytes.Repeat([]byte{2}, 16)
	secret := bytes.Repeat([]byte{3}, 8)
	ad := bytes.Repeat([]byte{4}, 12)
	cases := []struct {
		name string
		mode int
		tag  string
	}{
		{"Argon2d", argon2ModeD, "512b391b6f1162975371d30919734294f868e3be3984f3c1a13a4db9fabe4acb"},
		{"Argon2i", argon2ModeI, "c814d9d1dc7f37aa13f0d77f2494bda1c8de6b016dd388d29952a4c4672b6ce8"},
		{"Argon2id", argon2ModeID, "0d640df58d78766c08c037a34a8b53c9d01ef0452d75b65eb52520e96b01e659"},
	}
	for _, c := range cases {
		got := argon2Key(c.mode, pw, salt, secret, ad, 3, 32, 4, 32)
		if hex.EncodeToString(got) != c.tag {
			t.Errorf("%s: tag %x, want %s", c.name, got, c.tag)
		}
	}
}

// TestBlake2bRFC7693 checks the vendored BLAKE2b-512 of "abc" (RFC 7693
// appendix A) and a multi block input through incremental writes.
func TestBlake2bRFC7693(t *testing.T) {
	d := newB2b(b2bSize)
	d.Write([]byte("abc"))
	want := "ba80a53f981c4d0d6a2797b69f12f6e94c212f14685ac4b74b12bb6fdbffa2d17d87c5392aab792dc252d5de4533cc9518d38aa8dbf1925ab92386edd4009923"
	if got := hex.EncodeToString(d.Sum(nil)); got != want {
		t.Fatalf("blake2b(abc) = %s", got)
	}
	data := bytes.Repeat([]byte("0123456789"), 70)
	one := newB2b(48)
	one.Write(data)
	parts := newB2b(48)
	for i := 0; i < len(data); i += 37 {
		parts.Write(data[i:min(i+37, len(data))])
	}
	if !bytes.Equal(one.Sum(nil), parts.Sum(nil)) {
		t.Fatal("incremental writes differ from one write")
	}
}

// TestArgon2idMatchesXCrypto compares with golang.org/x/crypto/argon2.IDKey
// outputs recorded in testdata (random passwords, salts, and parameters,
// generated with x/crypto v0.55.0; decision 0059).
func TestArgon2idMatchesXCrypto(t *testing.T) {
	f, err := os.Open("testdata/argon2id_xcrypto.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	n := 0
	for sc.Scan() {
		fs := strings.Split(sc.Text(), " ") // an empty password is an empty field
		if len(fs) != 7 {
			t.Fatalf("bad line %q", sc.Text())
		}
		num := func(s string) uint32 {
			v, err := strconv.ParseUint(s, 10, 32)
			if err != nil {
				t.Fatal(err)
			}
			return uint32(v)
		}
		pw, salt := unhex(t, fs[0]), unhex(t, fs[1])
		got := argon2Key(argon2ModeID, pw, salt, nil, nil, num(fs[2]), num(fs[3]), uint8(num(fs[4])), num(fs[5]))
		if hex.EncodeToString(got) != fs[6] {
			t.Errorf("case %d: key %x, want %s", n, got, fs[6])
		}
		n++
	}
	if n == 0 {
		t.Fatal("no cases")
	}
}

// BenchmarkArgon2idDefault is one hash at the std/crypto defaults (19456
// KiB, 2 passes, 1 lane).
func BenchmarkArgon2idDefault(b *testing.B) {
	salt := bytes.Repeat([]byte{2}, 16)
	for b.Loop() {
		argon2Key(argon2ModeID, []byte("password"), salt, nil, nil, 2, 19456, 1, 32)
	}
}

func TestCryptoArgon2idHash(t *testing.T) {
	r := &seededRand{r: rand.New(rand.NewPCG(1, 2))}
	h := Crypto_hash_password_argon2id(nil, r, "pw", 64, 1, 1)
	if !strings.HasPrefix(h, "$argon2id$v=19$m=64,t=1,p=1$") || !Crypto_verify_password(nil, "pw", h) {
		t.Fatalf("hash %q does not verify", h)
	}
	if Crypto_verify_password(nil, "pW", h) {
		t.Fatal("wrong password verified")
	}
	parts := strings.Split(h, "$")
	if len(parts) != 6 || len(parts[4]) != 22 || len(parts[5]) != 43 {
		t.Fatalf("want a 16 byte salt and a 32 byte key: %q", h)
	}
	// a PHC string made with golang.org/x/crypto/argon2.IDKey
	known := "$argon2id$v=19$m=64,t=2,p=1$c29tZXNhbHRzb21lc2FsdA$Gpj7qOY5RCXJvcMzqcdQqvgR3wcPX7SleI4c9NtXk6E"
	if !Crypto_verify_password(nil, "password", known) {
		t.Fatal("x/crypto hash does not verify")
	}
	for _, bad := range []string{
		"$argon2id$v=19$m=64,t=2,p=1$c29tZXNhbHRzb21lc2FsdA",
		"$argon2id$v=16$m=64,t=2,p=1$c29tZXNhbHRzb21lc2FsdA$Gpj7qOY5RCXJvcMzqcdQqvgR3wcPX7SleI4c9NtXk6E",
		"$argon2id$v=19$m=64,t=3,p=1$c29tZXNhbHRzb21lc2FsdA$Gpj7qOY5RCXJvcMzqcdQqvgR3wcPX7SleI4c9NtXk6E",
		"$argon2id$v=19$t=2,m=64,p=1$c29tZXNhbHRzb21lc2FsdA$Gpj7qOY5RCXJvcMzqcdQqvgR3wcPX7SleI4c9NtXk6E",
		"$argon2id$v=19$m=64,t=2,p=0$c29tZXNhbHRzb21lc2FsdA$Gpj7qOY5RCXJvcMzqcdQqvgR3wcPX7SleI4c9NtXk6E",
		"$argon2id$v=19$m=99999999,t=2,p=1$c29tZXNhbHRzb21lc2FsdA$Gpj7qOY5RCXJvcMzqcdQqvgR3wcPX7SleI4c9NtXk6E",
		"$argon2id$v=19$m=64,t=2,p=1$!!$Gpj7qOY5RCXJvcMzqcdQqvgR3wcPX7SleI4c9NtXk6E",
		"$argon2id$v=19$m=64,t=2,p=1$c29tZXNhbHRzb21lc2FsdA$",
		"$argon2i$v=19$m=64,t=2,p=1$c29tZXNhbHRzb21lc2FsdA$Gpj7qOY5RCXJvcMzqcdQqvgR3wcPX7SleI4c9NtXk6E",
		"$argon2id$v=19$m=64,t=2,p=1$c29tZXNhbHRzb21lc2FsdA$Gpj7qOY5RCXJvcMzqcdQqvgR3wcPX7SleI4c9NtXk6E$",
	} {
		if Crypto_verify_password(nil, "password", bad) {
			t.Errorf("malformed or altered hash %q verified", bad)
		}
	}
	// the PBKDF2 format still verifies
	if !Crypto_verify_password(nil, "password", "pbkdf2-sha256$1$c2FsdA$Eg+2z/z4syxD5yJSVsT4N6hlSMkszDVICAWYfLcL4Xs") {
		t.Fatal("pbkdf2 hash no longer verifies")
	}
}
