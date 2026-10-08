package rt

import (
	"math/rand/v2"
	"strings"
	"testing"
)

func TestCryptoPasswordHash(t *testing.T) {
	r := &seededRand{r: rand.New(rand.NewPCG(1, 2))}
	h := Crypto_hash_password(nil, r, "pw", 5)
	if !strings.HasPrefix(h, "pbkdf2-sha256$5$") || !Crypto_verify_password(nil, "pw", h) {
		t.Fatalf("hash %q does not verify", h)
	}
	// RFC 7914 style vector: PBKDF2-HMAC-SHA256("password", "salt", 1, 32)
	known := "pbkdf2-sha256$1$c2FsdA$Eg+2z/z4syxD5yJSVsT4N6hlSMkszDVICAWYfLcL4Xs"
	if !Crypto_verify_password(nil, "password", known) {
		t.Fatal("known vector does not verify")
	}
	for _, bad := range []string{
		"", "pbkdf2-sha256$1$c2FsdA", "md5$1$c2FsdA$Eg", "pbkdf2-sha256$0$c2FsdA$Eg",
		"pbkdf2-sha256$x$c2FsdA$Eg", "pbkdf2-sha256$1$!!$Eg", "pbkdf2-sha256$1$c2FsdA$",
		"pbkdf2-sha256$999999999999$c2FsdA$Eg",
	} {
		if Crypto_verify_password(nil, "password", bad) {
			t.Errorf("malformed hash %q verified", bad)
		}
	}
}

func TestCryptoEqual(t *testing.T) {
	cases := []struct {
		a, b Bytes
		want bool
	}{{"", "", true}, {"a", "a", true}, {"a", "b", false}, {"ab", "a", false}}
	for _, c := range cases {
		if got := Crypto_equal(nil, c.a, c.b); got != c.want {
			t.Errorf("equal(%q, %q) = %v", c.a, c.b, got)
		}
	}
}
