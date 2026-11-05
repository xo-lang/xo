// The twin of bench/perf/repeat: strings.Repeat and bytes.Repeat.
package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

func main() {
	w := "none"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	p, k := "", 0
	switch w {
	case "byte1":
		p, k = "x", 160<<20
	case "byte3":
		p, k = "xyz", 50000000
	case "byte100":
		p, k = strings.Repeat("p", 99)+"q", 1600000
	}
	sum := 0
	if k > 0 {
		for i := 0; i < 10; i++ {
			s := strings.Repeat(p, k)
			b := bytes.Repeat([]byte(p), k)
			sum += len(s) + len(b) + int(b[len(b)-1])
		}
	}
	fmt.Printf("%s %d\n", w, sum)
}
