package rt

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strconv"
	"strings"
)

// std/crypto (std.md 11, decision 0056). Go standard library only.

// Crypto_sha256 is `crypto.sha256(data)`.
func Crypto_sha256(_ *Task, data Bytes) Bytes {
	sum := sha256.Sum256([]byte(data))
	return Bytes(sum[:])
}

// Crypto_hmac_sha256 is `crypto.hmac_sha256(key, msg)`.
func Crypto_hmac_sha256(_ *Task, key, msg Bytes) Bytes {
	m := hmac.New(sha256.New, []byte(key))
	m.Write([]byte(msg))
	return Bytes(m.Sum(nil))
}

// Crypto_equal is `crypto.equal(a, b)`: constant time in the contents.
func Crypto_equal(_ *Task, a, b Bytes) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Crypto_random_bytes is `crypto.random_bytes(r, n)`: n bytes from the Rand
// capability (crypto/rand for real programs, seeded in tests). Faults when
// n is negative.
func Crypto_random_bytes(tk *Task, r Rand, n int64) Bytes {
	if n < 0 {
		FaultHere("crypto.random_bytes: negative length %d", n)
	}
	return r.M_bytes(tk, n)
}

// Crypto_token is `crypto.token(r, n)`: unpadded URL safe base64 of n
// random bytes.
func Crypto_token(tk *Task, r Rand, n int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(Crypto_random_bytes(tk, r, n)))
}

const (
	pbkdf2Prefix  = "pbkdf2-sha256"
	pbkdf2SaltLen = 16
	pbkdf2KeyLen  = 32
	// pbkdf2MaxIter bounds the work verify_password does for a stored hash.
	pbkdf2MaxIter = 100_000_000
)

// Crypto_hash_password is `crypto.hash_password(r, password, iterations)`:
// PBKDF2-HMAC-SHA256 with a 16 byte salt from r, encoded as
// "pbkdf2-sha256$<iterations>$<salt>$<key>" (unpadded standard base64).
// Faults when iterations is below 1 or above 100,000,000.
func Crypto_hash_password(tk *Task, r Rand, password string, iterations int64) string {
	if iterations < 1 || iterations > pbkdf2MaxIter {
		FaultHere("crypto.hash_password: iterations must be in 1..=%d, got %d", pbkdf2MaxIter, iterations)
	}
	salt := []byte(Crypto_random_bytes(tk, r, pbkdf2SaltLen))
	key, err := pbkdf2.Key(sha256.New, password, salt, int(iterations), pbkdf2KeyLen)
	if err != nil {
		FaultHere("crypto.hash_password: %v", err)
	}
	enc := base64.RawStdEncoding
	return pbkdf2Prefix + "$" + strconv.FormatInt(iterations, 10) + "$" + enc.EncodeToString(salt) + "$" + enc.EncodeToString(key)
}

// Crypto_verify_password is `crypto.verify_password(password, hash)`:
// whether password matches a hash_password result. A malformed hash is
// false. The key comparison is constant time.
func Crypto_verify_password(_ *Task, password, hash string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 4 || parts[0] != pbkdf2Prefix {
		return false
	}
	iter, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || iter < 1 || iter > pbkdf2MaxIter {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := enc.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, int(iter), len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}
