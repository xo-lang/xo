// Go twin of bench/perf/sizewaste.
package main

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func rss() int {
	b, _ := os.ReadFile("/proc/self/status")
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "VmRSS:") {
			f := strings.Fields(l)
			n, _ := strconv.Atoi(f[1])
			return n
		}
	}
	return 0
}

func main() {
	size, count := 8192, 1000
	if len(os.Args) > 1 {
		size, _ = strconv.Atoi(os.Args[1])
	}
	if len(os.Args) > 2 {
		count, _ = strconv.Atoi(os.Args[2])
	}
	base := rss()
	held := make([][]byte, 0, count)
	for i := 0; i < count; i++ {
		held = append(held, bytes.Repeat([]byte("x"), size))
	}
	r := rss() - base
	sum := 0
	for _, b := range held {
		sum += len(b) + int(b[len(b)/2])
	}
	p := size * count / 1024
	fmt.Println(size, count, p, r, float64(r)/float64(p), sum)
}
