package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
)

// brotliCorpus runs brotlioracle, its own module (github.com/andybalholm/brotli is
// std/brotli's reference, not part of Go's standard library).
//
// The oracle is built with FMA fusion off (-d=fmahash=n): Go may fuse
// x*y+z into one rounding on arm64 and not on amd64, and the encoder's
// cost model is floating point, so arm64 Go chose other blocks than amd64
// Go for some inputs (#116, brotli fuzz seed 182: a quality 10 stream of
// 5203 bytes on arm64, 5202 on amd64). Xo never fuses (decision 0287) and
// matches Go without fusion on both.
func brotliCorpus() string {
	cmd := exec.Command("go", "run", "-gcflags=all=-d=fmahash=n", ".")
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "brotlioracle")
	// the compiler reports each fusion it declined ("fmahash triggered
	// [DISABLED]"): stderr is shown only when the oracle fails
	var errb bytes.Buffer
	cmd.Stderr = &errb
	out, err := cmd.Output()
	if err != nil {
		os.Stderr.Write(errb.Bytes())
		panic("brotlioracle: " + err.Error())
	}
	return string(out)
}
