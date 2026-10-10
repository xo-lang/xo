package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTLSLibrary builds the vendored Mbed TLS (decision 0105) into a
// fresh cache and links a C program that sets up a TLS client and server
// configuration with it.
func TestTLSLibrary(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles the TLS library (a few seconds)")
	}
	clang, err := Clang()
	if err != nil {
		t.Skip("clang not found")
	}
	t.Setenv("XO_TLS_CACHE", t.TempDir())
	dir := t.TempDir()
	src := filepath.Join(dir, "main.c")
	prog := `#include <stdio.h>
#include "mbedtls/ssl.h"
#include "psa/crypto.h"
int main(void) {
	if (psa_crypto_init() != PSA_SUCCESS) return 1;
	mbedtls_ssl_config c;
	mbedtls_ssl_config_init(&c);
	if (mbedtls_ssl_config_defaults(&c, MBEDTLS_SSL_IS_CLIENT, MBEDTLS_SSL_TRANSPORT_STREAM, MBEDTLS_SSL_PRESET_DEFAULT)) return 2;
	mbedtls_ssl_config_free(&c);
	printf("ok\n");
	return 0;
}
`
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	// a link of generated code that calls the client gets the library
	ll := filepath.Join(dir, "prog.s")
	if err := os.WriteFile(ll, []byte("// bl _xo_cl_do\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !usesTLS([]string{ll, src}) {
		t.Fatal("usesTLS: false for a source naming xo_cl_do")
	}
	if usesTLS([]string{src}) {
		t.Fatal("usesTLS: true for C sources only")
	}
	bin := filepath.Join(dir, "prog")
	compile, link, err := tlsArgs(clang, []string{"-O1"}, sysrootFor(t, clang))
	if err != nil {
		t.Fatal(err)
	}
	args := append(append(append([]string{"-O1", "-o", bin}, compile...), src), link...)
	if root := sysrootFor(t, clang); root != "" {
		args = append(args, "-isysroot", root)
	}
	if out, err := exec.Command(clang, args...).CombinedOutput(); err != nil {
		t.Fatalf("link: %v\n%s", err, out)
	}
	out, err := exec.Command(bin).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "ok" {
		t.Fatalf("run: %v\n%s", err, out)
	}
}

// sysrootFor is the macOS SDK ccBuild chose (a link of nothing forces
// the choice).
func sysrootFor(t *testing.T, clang string) string {
	dir := t.TempDir()
	src := filepath.Join(dir, "x.c")
	os.WriteFile(src, []byte("int main(void) { return 0; }\n"), 0o644)
	if err := ccBuild(clang, []string{"-O0"}, []string{src}, filepath.Join(dir, "x")); err != nil {
		t.Fatal(err)
	}
	sysrootMu.Lock()
	defer sysrootMu.Unlock()
	return sysroot
}
