package rt

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"hash"
	"strconv"
	"strings"
)

// Internal_crypto_op is `internal.crypto_op(op, a, b, c, d)`: Go's crypto
// as the reference for the native TLS primitives (work item B31). The one
// dispatcher: each area of std/internal.xo's list is one case calling
// its function here.
func Internal_crypto_op(_ *Task, op string, a, b, c, d Bytes) Bytes {
	area, what, _ := strings.Cut(op, ".")
	var r Bytes
	ok := false
	switch area {
	case "hash":
		r, ok = cryptoHash(what, a, b)
	case "hmac":
		r, ok = cryptoHMAC(what, a, b)
	case "hkdf":
		r, ok = cryptoHKDF(what, a, b, c, d)
	}
	if !ok {
		panic("internal.crypto_op: unknown op " + op)
	}
	return r
}

// cryptoHashOf is the hash a crypto_op name ends in ("...256", "...384", or "...512").
func cryptoHashOf(what string) (func() hash.Hash, string) {
	f := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(what, "256"), "384"), "512")
	if strings.HasSuffix(what, "384") {
		return sha512.New384, f
	}
	if strings.HasSuffix(what, "512") {
		return sha512.New, f
	}
	return sha256.New, f
}

func cryptoNum(x Bytes) int { n, _ := strconv.Atoi(string(x)); return n }

func cryptoSum(x hash.Hash) Bytes { return Bytes(x.Sum(nil)) }

// cryptoHash is sha256.xo's op.
func cryptoHash(what string, a, b Bytes) (Bytes, bool) {
	h, f := cryptoHashOf(what)
	switch f {
	case "sha":
		x := h()
		x.Write([]byte(a))
		return cryptoSum(x), true
	case "split":
		x := h()
		x.Write([]byte(a))
		s1 := cryptoSum(x)
		x.Write([]byte(b))
		return s1 + cryptoSum(x) + s1, true
	}
	return "", false
}

// cryptoHMAC is hmac.xo's op.
func cryptoHMAC(what string, a, b Bytes) (Bytes, bool) {
	h, f := cryptoHashOf(what)
	if f != "sha" {
		return "", false
	}
	m := hmac.New(h, []byte(a))
	m.Write([]byte(b))
	return cryptoSum(m), true
}

// cryptoHKDF is hkdf.xo's op.
func cryptoHKDF(what string, a, b, c, d Bytes) (Bytes, bool) {
	h, f := cryptoHashOf(what)
	var k []byte
	var err error
	switch f {
	case "extract":
		k, err = hkdf.Extract(h, []byte(b), []byte(a))
	case "expand":
		k, err = hkdf.Expand(h, []byte(a), string(b), cryptoNum(c))
	case "label":
		full := "tls13 " + string(b)
		n := cryptoNum(d)
		info := []byte{byte(n >> 8), byte(n), byte(len(full))}
		info = append(info, full...)
		info = append(info, byte(len(c)))
		info = append(info, c...)
		k, err = hkdf.Expand(h, []byte(a), string(info), n)
	default:
		return "", false
	}
	if err != nil {
		panic(err)
	}
	return Bytes(k), true
}
