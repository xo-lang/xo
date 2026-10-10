// Package mbedtls embeds the vendored source of Mbed TLS (with
// TF-PSA-Crypto), the TLS library of the native backends' std/http
// (decision 0105). internal/build compiles it once per C compiler,
// sysroot, and sanitizer flavor into a static archive in the user cache
// directory and links it into native programs that use std/http.
//
// src/ is a subset of the mbedtls-4.1.1 release tarball (the .c and .h
// files of include/, library/, and the TF-PSA-Crypto directories the
// library build uses), unmodified; xo/ holds the Xo runtime's
// configuration and the functions it puts in front of the library's
// (decisions 0117, 0132; xo_p256.c includes fiat-crypto's files, which
// Tree adds from third_party/fiat-crypto). The package embeds mbedtls.tgz, a compressed copy of
// src/ made by gen.go, and xo/. The pinned release, its checksum, and the
// update steps are in knowledgebase/infrastructure/toolchain.md ("TLS
// library") and scripts/vendor-mbedtls.sh.
package mbedtls

//go:generate go run gen.go

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"embed"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	fiatcrypto "github.com/xo-lang/xo/third_party/fiat-crypto"
)

// Files holds mbedtls.tgz and xo/.
//
//go:embed mbedtls.tgz xo
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
	"fiat-crypto/src",
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
	"xo",
}

// FileDefines are extra compile flags of single library files, so the
// vendored files stay unmodified (decision 0117): a rename of a library
// function that xo/xo_tls_accel.c defines in front of it, and the macOS
// feature macro sha512.c's run time check of the SHA-512 instructions
// needs (it includes <sys/sysctl.h>, which does not compile under the
// library's _POSIX_C_SOURCE alone; nothing on Linux).
var FileDefines = map[string][]string{
	"src/tf-psa-crypto/drivers/builtin/src/sha512.c": {"-D_DARWIN_C_SOURCE"},
	"src/tf-psa-crypto/drivers/builtin/src/bignum.c": {
		"-Dmbedtls_mpi_gcd_modinv_odd=xo_builtin_mpi_gcd_modinv_odd",
	},
	"src/tf-psa-crypto/drivers/builtin/src/psa_crypto_ecp.c": {
		"-Dmbedtls_psa_ecp_export_public_key=xo_builtin_ecp_export_public_key",
		"-Dmbedtls_psa_ecdsa_sign_hash=xo_builtin_psa_ecdsa_sign_hash",
		"-Dmbedtls_psa_ecdsa_verify_hash=xo_builtin_psa_ecdsa_verify_hash",
	},
}

// Exclude lists files of SourceDirs that are not compiled on their own
// (Hacl_Curve25519_joined.c includes Hacl_Curve25519.c).
var Exclude = []string{
	"src/tf-psa-crypto/drivers/everest/library/Hacl_Curve25519.c",
}

// FiatDir is where Tree puts the vendored fiat-crypto files
// (third_party/fiat-crypto/src), which xo/xo_p256.c includes (decision
// 0132).
const FiatDir = "fiat-crypto/src"

// File is one file of the library tree: its slash separated path
// (src/... or xo/...) and contents.
type File struct {
	Name string
	Data []byte
}

// Tree returns every file of the embedded tree, sorted by name: the
// files of mbedtls.tgz under src/, xo/, and fiat-crypto's C files under
// FiatDir.
func Tree() ([]File, error) {
	tgz, err := Files.ReadFile("mbedtls.tgz")
	if err != nil {
		return nil, err
	}
	out, err := untar(tgz)
	if err != nil {
		return nil, err
	}
	err = fs.WalkDir(Files, "xo", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := Files.ReadFile(p)
		out = append(out, File{Name: p, Data: data})
		return err
	})
	if err != nil {
		return nil, err
	}
	err = fs.WalkDir(fiatcrypto.Files, "src", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fiatcrypto.Files.ReadFile(p)
		out = append(out, File{Name: path.Join(FiatDir, path.Base(p)), Data: data})
		return err
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func untar(tgz []byte) ([]File, error) {
	zr, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(zr)
	var out []File
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		out = append(out, File{Name: h.Name, Data: data})
	}
}

// Archive makes the deterministic tar.gz of the files under dir (on
// disk, relative to the working directory), named src/... (gen.go).
func Archive(dir string) ([]byte, error) {
	var names []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		names = append(names, p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	tw := tar.NewWriter(zw)
	for _, n := range names {
		data, err := os.ReadFile(n)
		if err != nil {
			return nil, err
		}
		rel, _ := filepath.Rel(dir, n)
		name := path.Join("src", filepath.ToSlash(rel))
		if strings.Contains(name, "..") {
			continue
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg, Format: tar.FormatPAX}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
