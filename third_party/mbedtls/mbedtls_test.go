package mbedtls

import (
	"bytes"
	"os"
	"testing"
)

// TestArchive requires mbedtls.tgz to be exactly the archive of src/
// (run `go generate ./third_party/mbedtls` after changing src/).
func TestArchive(t *testing.T) {
	want, err := Archive("src")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("mbedtls.tgz")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("mbedtls.tgz does not match src/: run go generate ./third_party/mbedtls")
	}
	tree, err := Tree()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range tree {
		if f.Name == "src/library/ssl_tls.c" || f.Name == "xo/xo_tls_config.h" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("Tree lacks src/library/ssl_tls.c or xo/xo_tls_config.h (%d files)", len(tree))
	}
}
