// Command xokey prints the key of the xo a host builds from a checkout's
// sources (internal/xotool KeyFor; the host's cache holds it at
// <user cache>/xo/checkout/KEY/xo). offload-gate.sh asks the offload host
// whether it has that xo, and sizes its lease for a cold build when not.
//
//	go run ./scripts/xokey [-os linux] [-arch arm64] [DIR]
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/xo-lang/xo/internal/xotool"
)

func main() {
	goos := flag.String("os", "linux", "the host's GOOS")
	goarch := flag.String("arch", "arm64", "the host's GOARCH")
	flag.Parse()
	dir := "."
	if flag.NArg() > 0 {
		dir = flag.Arg(0)
	}
	k, err := xotool.KeyFor(dir, *goos, *goarch)
	if err != nil {
		fmt.Fprintln(os.Stderr, "xokey:", err)
		os.Exit(1)
	}
	fmt.Println(k)
}
