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

// std/crypto (std.md 11, decisions 0056 and 0059). Go standard library,
// plus Argon2id vendored from golang.org/x/crypto (xcrypto_argon2.go,
// xcrypto_blake2b.go).

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
// whether password matches a hash_password or hash_password_argon2id
// result, told apart by the prefix. A malformed hash is false. The key
// comparison is constant time.
func Crypto_verify_password(_ *Task, password, hash string) bool {
	if strings.HasPrefix(hash, argon2idPrefix) {
		return verifyArgon2id(password, hash)
	}
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

// Argon2id (RFC 9106) in the PHC string format:
// $argon2id$v=19$m=<KiB>,t=<passes>,p=<lanes>$<salt>$<key>, salt and key
// in unpadded standard base64.
const (
	argon2idPrefix  = "$argon2id$"
	argon2idSaltLen = 16
	argon2idKeyLen  = 32
	// The bounds hash_password_argon2id accepts; verify_password rejects a
	// stored hash outside them rather than doing unbounded work.
	argon2idMaxMemory = 4 * 1024 * 1024 // KiB, 4 GiB
	argon2idMaxPasses = 1000
	argon2idMaxLanes  = 255
	argon2idMaxKeyLen = 1024
)

// argon2idParamsOK reports whether m, t, p are within the accepted bounds.
func argon2idParamsOK(m, t, p int64) bool {
	return p >= 1 && p <= argon2idMaxLanes && m >= 8*p && m <= argon2idMaxMemory && t >= 1 && t <= argon2idMaxPasses
}

// Crypto_hash_password_argon2id is `crypto.hash_password_argon2id(r,
// password, memory_kib, iterations, parallelism)`: Argon2id with a 16 byte
// salt from r and a 32 byte key. Faults when the parameters are outside
// 1..=255 lanes, 8 * lanes..=4194304 KiB, or 1..=1000 passes.
func Crypto_hash_password_argon2id(tk *Task, r Rand, password string, memoryKiB, iterations, parallelism int64) string {
	if !argon2idParamsOK(memoryKiB, iterations, parallelism) {
		FaultHere("crypto.hash_password_argon2id: need parallelism in 1..=%d, memory_kib in 8 * parallelism..=%d, iterations in 1..=%d; got memory_kib %d, iterations %d, parallelism %d",
			argon2idMaxLanes, argon2idMaxMemory, argon2idMaxPasses, memoryKiB, iterations, parallelism)
	}
	salt := []byte(Crypto_random_bytes(tk, r, argon2idSaltLen))
	key := argon2Key(argon2ModeID, []byte(password), salt, nil, nil, uint32(iterations), uint32(memoryKiB), uint8(parallelism), argon2idKeyLen)
	enc := base64.RawStdEncoding
	return argon2idPrefix + "v=19$m=" + strconv.FormatInt(memoryKiB, 10) + ",t=" + strconv.FormatInt(iterations, 10) + ",p=" + strconv.FormatInt(parallelism, 10) +
		"$" + enc.EncodeToString(salt) + "$" + enc.EncodeToString(key)
}

// verifyArgon2id checks password against a PHC Argon2id string. Only
// version 19 (0x13) and the parameter order m, t, p are accepted.
func verifyArgon2id(password, hash string) bool {
	parts := strings.Split(hash, "$")
	// "", "argon2id", "v=19", "m=..,t=..,p=..", salt, key
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	ps := strings.Split(parts[3], ",")
	if len(ps) != 3 {
		return false
	}
	var vals [3]int64
	for i, name := range []string{"m=", "t=", "p="} {
		if !strings.HasPrefix(ps[i], name) {
			return false
		}
		v, err := strconv.ParseInt(ps[i][2:], 10, 64)
		if err != nil {
			return false
		}
		vals[i] = v
	}
	m, t, p := vals[0], vals[1], vals[2]
	if !argon2idParamsOK(m, t, p) {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[4])
	if err != nil || len(salt) < 8 {
		return false
	}
	want, err := enc.DecodeString(parts[5])
	if err != nil || len(want) < 4 || len(want) > argon2idMaxKeyLen {
		return false
	}
	got := argon2Key(argon2ModeID, []byte(password), salt, nil, nil, uint32(t), uint32(m), uint8(p), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
