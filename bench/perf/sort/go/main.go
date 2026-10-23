// Go twin of bench/perf/sort/xo: slices.Sort (pdqsort) of one million
// pseudo random Ints and of 200 000 Strs. Usage: sortgo ints|strs|floats.
// Prints "<workload> <checksum>".
package main

import (
	"fmt"
	"os"
	"slices"
	"strconv"
)

func ints() int {
	xs := make([]int, 0, 1000000)
	seed := 42
	for range 1000000 {
		seed = (seed*1103515245 + 12345) % 2147483648
		xs = append(xs, seed%1000000)
	}
	xs = slices.Clone(xs)
	slices.Sort(xs)
	h := xs[0] + xs[len(xs)/2]*3 + xs[len(xs)-1]
	for i := 0; i < len(xs); i += 997 {
		h = (h*31 + xs[i]) % 1000000007
	}
	return h
}

func strs() int {
	xs := make([]string, 0, 200000)
	seed := 7
	for range 200000 {
		seed = (seed*1103515245 + 12345) % 2147483648
		xs = append(xs, "key"+strconv.Itoa(seed%1000000))
	}
	slices.Sort(xs)
	h := len(xs[0]) + len(xs[len(xs)/2])*3 + len(xs[len(xs)-1])
	for i := 0; i < len(xs); i += 997 {
		h = (h*31 + int(xs[i][len(xs[i])-1])) % 1000000007
	}
	return h
}

func floats() int {
	xs := make([]float64, 0, 1000000)
	seed := 99
	for range 1000000 {
		seed = (seed*1103515245 + 12345) % 2147483648
		xs = append(xs, float64(seed%1000000)/7.0)
	}
	slices.Sort(xs)
	h := int(xs[0]) + int(xs[len(xs)/2])*3 + int(xs[len(xs)-1])
	for i := 0; i < len(xs); i += 997 {
		h = (h*31 + int(xs[i])) % 1000000007
	}
	return h
}

func main() {
	w := "ints"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	switch w {
	case "ints":
		fmt.Println("ints", ints())
	case "strs":
		fmt.Println("strs", strs())
	case "floats":
		fmt.Println("floats", floats())
	default:
		fmt.Fprintln(os.Stderr, "unknown workload", w)
		os.Exit(2)
	}
}
