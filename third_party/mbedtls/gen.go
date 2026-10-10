//go:build ignore

// gen writes mbedtls.tgz, the compressed copy of src/ that package
// mbedtls embeds (the xo binary carries about 1.4 MB instead of the 6.5 MB
// of the sources). Run after changing src/:
//
//	go generate ./third_party/mbedtls
//
// The archive is deterministic (sorted names, no times or owners);
// TestArchive fails when it does not match src/.
package main

import (
	"fmt"
	"os"

	"github.com/xo-lang/xo/third_party/mbedtls"
)

func main() {
	data, err := mbedtls.Archive("src")
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
	if err := os.WriteFile("mbedtls.tgz", data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}
