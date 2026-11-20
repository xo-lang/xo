//go:build xo_no_x509

package rt

// The x509, chain and rsa reference ops of internal.crypto_op
// (internal_crypto_x509.go) are left out of a program that does not
// reach std/x509 (task #472): only the tests of std/x509, std/tls and
// std/crypto call them, and tests build without the optional tags.

func cryptoX509(what string, a, b Bytes) (Bytes, bool)        { return "", false }
func cryptoChain(what string, a, b, c, d Bytes) (Bytes, bool) { return "", false }
func cryptoRSA(what string, a, b, c, d Bytes) (Bytes, bool)   { return "", false }
