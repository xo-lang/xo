// Command perfgo is the hand written Go version of bench/perf/xo: the same
// workloads over Go maps and slices. Insertion order is kept with a key
// slice where the Xo workload depends on it (iteration), so the work is
// comparable. Usage: perfgo <workload>; prints "<workload> <checksum>
// <elapsed microseconds>".
package main

import (
	"fmt"
	"maps"
	"os"
	"time"
)

const (
	n    = 3000000
	keys = 50000
)

func build(c int) map[int64]int64 {
	m := map[int64]int64{}
	for i := range int64(c) {
		m[i] = i
	}
	return m
}

func putGet() int64 {
	m := map[int64]int64{}
	for i := range int64(n) {
		m[i%keys] = i
	}
	var sum int64
	for i := range int64(n) {
		sum += m[i%keys]
	}
	return sum
}

func getOnly(m map[int64]int64) int64 {
	var sum int64
	for i := range int64(n) {
		sum += m[i%keys]
	}
	return sum
}

func pushSum() int64 {
	var xs []int64
	for i := range int64(n) {
		xs = append(xs, i)
	}
	var sum int64
	for _, x := range xs {
		sum += x
	}
	return sum
}

func updateLoop() int64 {
	m := map[int64]int64{}
	for i := range int64(n) {
		m[i%keys]++
	}
	return m[7]
}

// iterate walks the map in insertion order through a key slice, as the
// Xo Map does.
func iterate(m map[int64]int64, order []int64) int64 {
	var sum int64
	for range 60 {
		for _, k := range order {
			sum += k + m[k]
		}
	}
	return sum
}

func copyMutate(m map[int64]int64) int64 {
	var sum int64
	for range 10 {
		c := maps.Clone(m)
		for k := range int64(keys) {
			c[k] = k * 2
		}
		for k := range int64(keys) {
			sum += m[k]
		}
		sum += c[keys-1]
	}
	return sum
}

func branch(m map[int64]int64) int64 {
	var sum int64
	for i := range int64(2000) {
		c := maps.Clone(m)
		c[keys+i] = i
		sum += int64(len(c)) + c[i]
	}
	return sum
}

func main() {
	w := "putget"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	base := build(keys)
	order := make([]int64, keys)
	for i := range order {
		order[i] = int64(i)
	}
	t := time.Now()
	var sum int64
	switch w {
	case "putget":
		sum = putGet()
	case "get":
		sum = getOnly(base)
	case "push":
		sum = pushSum()
	case "update":
		sum = updateLoop()
	case "iter":
		sum = iterate(base, order)
	case "copymut":
		sum = copyMutate(base)
	case "branch":
		sum = branch(base)
	default:
		sum = -1
	}
	fmt.Printf("%s %d %d\n", w, sum, time.Since(t).Microseconds())
}
