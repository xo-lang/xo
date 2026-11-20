//go:build xo_no_tls

package rt

// The TLS reference ops of internal.crypto_op (internal_tls12.go,
// internal_tls13.go, internal_tls13srv.go) are left out of a program
// that does not reach std/tls (task #472): only std/tls's tests call
// them, and tests build without the optional tags.

func cryptoTLS12(what string, a, b, c, d Bytes) (Bytes, bool)    { return "", false }
func cryptoTLS13(what string, a, b, c, d Bytes) (Bytes, bool)    { return "", false }
func cryptoTLS13Srv(what string, a, b, c, d Bytes) (Bytes, bool) { return "", false }
