// Package fiatcrypto embeds the vendored C code of fiat-crypto
// (github.com/mit-plv/fiat-crypto): the formally verified 64 bit
// Montgomery arithmetic modulo P-256's prime and group order, which the
// native TLS library's P-256 signer (third_party/mbedtls/xo/xo_p256.c,
// decision 0132) includes. The files are unmodified; the pinned commit,
// their checksums, and the update steps are in README.md and
// scripts/vendor-fiat-crypto.sh.
package fiatcrypto

import "embed"

// Files holds src/p256_64.c and src/p256_scalar_64.c.
//
//go:embed src/*.c
var Files embed.FS

// Commit is the vendored fiat-crypto commit.
const Commit = "f3ecb2cf1f771de34771e967193ee7f0ee0b0acf"
