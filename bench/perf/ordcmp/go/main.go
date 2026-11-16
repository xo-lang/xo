// Go twin of bench/perf/ordcmp: bytes.Compare and strings.Compare.
package main

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	w := "bytes"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	labels := make([]string, 0, 64)
	for i := 0; i < 64; i++ {
		labels = append(labels, "abcdefghijklmnopqr"[0:3+i%16]+strconv.Itoa(i%7))
	}
	bl := make([][]byte, 0, 64)
	for _, l := range labels {
		bl = append(bl, []byte(l))
	}
	n := 0
	if w != "none" {
		for k := 0; k < 20000000; k++ {
			i := k & 63
			j := (k >> 6) & 63
			var o int
			if w == "bytes" {
				o = bytes.Compare(bl[i], bl[j])
			} else {
				o = strings.Compare(labels[i], labels[j])
			}
			n += o + 2
		}
	}
	fmt.Printf("%s %d\n", w, n)
}
