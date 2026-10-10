// Package mbedtls embeds the vendored source of Mbed TLS (with
// TF-PSA-Crypto), the TLS library of the native backends' std/http
// (decision 0105). internal/build compiles it once per C compiler and
// flavor into a static archive in the user cache directory and links it
// into native programs that use std/http.
//
// src/ is a subset of the mbedtls-4.1.1 release (the .c and .h files of
// include/, library/, and the TF-PSA-Crypto directories the library
// build uses), unmodified; xo/ holds the Xo runtime's configuration.
// See README.md for the pinned release and how to update it.
package mbedtls

import "embed"

// Files holds src/ and xo/.
//
//go:embed src xo
var Files embed.FS

// Version is the vendored Mbed TLS release.
const Version = "4.1.1"

// IncludeDirs are the include directories (relative to the extracted
// tree) of a compile of the library or of code using it: the public
// headers, then the internal ones the library's own files need.
var IncludeDirs = []string{
	"xo",
	"src/include",
	"src/tf-psa-crypto/include",
	"src/tf-psa-crypto/drivers/builtin/include",
	"src/tf-psa-crypto/drivers/everest/include",
	"src/tf-psa-crypto/drivers/everest/include/tf-psa-crypto/private/everest",
	"src/tf-psa-crypto/drivers/everest/include/tf-psa-crypto/private/everest/kremlib",
	"src/tf-psa-crypto/drivers/p256-m/p256-m/include",
	"src/tf-psa-crypto/drivers/p256-m/p256-m/include/p256-m",
	"src/tf-psa-crypto/drivers/p256-m/p256-m_driver_interface",
	"src/library",
	"src/tf-psa-crypto/core",
	"src/tf-psa-crypto/drivers/builtin/src",
	"src/tf-psa-crypto/dispatch",
	"src/tf-psa-crypto/extras",
	"src/tf-psa-crypto/platform",
	"src/tf-psa-crypto/utilities",
}

// Defines select the Xo configuration files (xo/).
var Defines = []string{
	`-DMBEDTLS_USER_CONFIG_FILE="xo_tls_config.h"`,
	`-DTF_PSA_CRYPTO_USER_CONFIG_FILE="xo_crypto_config.h"`,
}

// SourceDirs hold the .c files of the library (not their subdirectories).
var SourceDirs = []string{
	"src/library",
	"src/tf-psa-crypto/core",
	"src/tf-psa-crypto/drivers/builtin/src",
	"src/tf-psa-crypto/extras",
	"src/tf-psa-crypto/platform",
	"src/tf-psa-crypto/utilities",
	"src/tf-psa-crypto/drivers/everest/library",
	"src/tf-psa-crypto/drivers/p256-m",
	"src/tf-psa-crypto/drivers/p256-m/p256-m",
}

// Exclude lists files of SourceDirs that are not compiled on their own
// (Hacl_Curve25519_joined.c includes Hacl_Curve25519.c).
var Exclude = []string{
	"src/tf-psa-crypto/drivers/everest/library/Hacl_Curve25519.c",
}
