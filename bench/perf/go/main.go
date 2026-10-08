// Command perfgo is the hand written Go version of bench/perf/xo: the same
// workloads over Go maps and slices. Insertion order is kept with a key
// slice where the Xo workload depends on it (iteration), so the work is
// comparable. Usage: perfgo <workload>; prints "<workload> <checksum>
// <elapsed microseconds>".
package main

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"slices"
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

// ---------------------------------------------------------------------------
// List writes: the same algorithms over Go slices.

const ln = 50000

func sumList(xs []int64) int64 {
	var t int64
	for _, x := range xs {
		t += x
	}
	return t
}

func setAt() int64 {
	var xs []int64
	for i := range int64(ln) {
		xs = append(xs, i)
	}
	for i := range int64(n) {
		xs[i%ln] = xs[(i*7)%ln] + i%13
	}
	return sumList(xs)
}

func stack() int64 {
	var xs []int64
	var sum int64
	for i := range int64(n) {
		if i%3 == 2 {
			if len(xs) > 0 {
				sum += xs[len(xs)-1]
				xs = xs[:len(xs)-1]
			}
		} else {
			xs = append(xs, i)
		}
	}
	return sum + int64(len(xs))
}

type particle struct{ x, y, vx, vy, mass int64 }

func fields() int64 {
	var ps []particle
	for i := range int64(1000) {
		ps = append(ps, particle{x: i, vx: i % 7, vy: 1, mass: 1})
	}
	for i := range int64(n) {
		j := i % 1000
		ps[j].x += ps[j].vx
		ps[(i*13)%1000].y = ps[j].x % 1000
	}
	var sum int64
	for _, p := range ps {
		sum += p.x + p.y
	}
	return sum
}

func grid() int64 {
	var g [][]int64
	for i := range int64(300) {
		var row []int64
		for j := range int64(300) {
			row = append(row, i+j)
		}
		g = append(g, row)
	}
	for i := range int64(n) {
		g[i%300][(i*7)%300]++
	}
	for i := range int64(300) {
		g[i] = append(g[i], i)
	}
	var sum int64
	for _, row := range g {
		sum += sumList(row)
	}
	return sum
}

func insRem() int64 {
	var xs []int64
	for i := range int64(20000) {
		xs = slices.Insert(xs, len(xs)/2, i)
	}
	var sum int64
	for range 20000 {
		k := len(xs) / 2
		sum += xs[k]
		xs = slices.Delete(xs, k, k+1)
	}
	return sum + int64(len(xs))
}

func copySet() int64 {
	var base []int64
	for i := range int64(ln) {
		base = append(base, i)
	}
	var sum int64
	for r := range int64(200) {
		c := slices.Clone(base)
		c[r] = -1
		c[r+1] = -2
		sum += base[r] + c[r] + int64(len(c))
	}
	return sum
}

// ---------------------------------------------------------------------------
// Generic workloads, written as idiomatic Go generics: a cmp.Ordered
// container for Int, a comparison function container (the
// slices.BinarySearchFunc style) for a struct, generic Map/Filter/Reduce
// helpers, a MaxBy over a key function, and a method constraint.

const (
	sortN   = 5000
	lookups = 3000000
	hofN    = 1000000
)

func lcg(s int64) int64 { return (s*1103515245 + 12345) % 2147483648 }

// Sorted is a sorted slice of an ordered type.
type Sorted[T cmp.Ordered] struct{ items []T }

func (s *Sorted[T]) search(x T) int {
	lo, hi := 0, len(s.items)
	for lo < hi {
		mid := (lo + hi) / 2
		if s.items[mid] < x {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

func (s *Sorted[T]) Insert(x T) {
	i := s.search(x)
	s.items = slices.Insert(s.items, i, x)
}

func (s *Sorted[T]) Contains(x T) bool {
	i := s.search(x)
	return i < len(s.items) && s.items[i] == x
}

// SortedFunc is a sorted slice ordered by a comparison function.
type SortedFunc[T any] struct {
	items []T
	cmp   func(a, b T) int
}

func (s *SortedFunc[T]) search(x T) int {
	lo, hi := 0, len(s.items)
	for lo < hi {
		mid := (lo + hi) / 2
		if s.cmp(s.items[mid], x) < 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

func (s *SortedFunc[T]) Insert(x T) {
	i := s.search(x)
	s.items = slices.Insert(s.items, i, x)
}

func (s *SortedFunc[T]) Contains(x T) bool {
	i := s.search(x)
	return i < len(s.items) && s.cmp(s.items[i], x) == 0
}

type point struct{ x, y int64 }

func comparePoint(a, b point) int {
	if c := cmp.Compare(a.x, b.x); c != 0 {
		return c
	}
	return cmp.Compare(a.y, b.y)
}

func sortedInt() int64 {
	var s Sorted[int64]
	seed := int64(1)
	for range sortN {
		seed = lcg(seed)
		s.Insert(seed % 1000000)
	}
	var found int64
	for range lookups {
		seed = lcg(seed)
		if s.Contains(seed % 1000000) {
			found++
		}
	}
	return found*1000000 + int64(len(s.items))
}

func sortedPoint() int64 {
	s := SortedFunc[point]{cmp: comparePoint}
	seed := int64(1)
	for range sortN {
		seed = lcg(seed)
		r := seed % 1000000
		s.Insert(point{r % 1000, r / 1000})
	}
	var found int64
	for range lookups {
		seed = lcg(seed)
		r := seed % 1000000
		if s.Contains(point{r % 1000, r / 1000}) {
			found++
		}
	}
	return found*1000000 + int64(len(s.items))
}

func Map[T, U any](xs []T, f func(T) U) []U {
	out := make([]U, 0, len(xs))
	for _, x := range xs {
		out = append(out, f(x))
	}
	return out
}

func Filter[T any](xs []T, f func(T) bool) []T {
	var out []T
	for _, x := range xs {
		if f(x) {
			out = append(out, x)
		}
	}
	return out
}

func Reduce[T, A any](xs []T, init A, f func(A, T) A) A {
	acc := init
	for _, x := range xs {
		acc = f(acc, x)
	}
	return acc
}

func hofChain(xs []int64) int64 {
	var sum int64
	for range 10 {
		ys := Filter(Map(xs, func(x int64) int64 { return x * 3 }), func(x int64) bool { return x%2 == 0 })
		sum += Reduce(ys, 0, func(a, x int64) int64 { return a + x })
	}
	return sum
}

type item struct {
	id, score int64
	name      string
}

func MaxBy[T any, K cmp.Ordered](xs []T, key func(T) K) (T, bool) {
	var zero T
	if len(xs) == 0 {
		return zero, false
	}
	best := xs[0]
	bestK := key(best)
	for _, x := range xs {
		if k := key(x); k > bestK {
			best, bestK = x, k
		}
	}
	return best, true
}

func maxByLoop(items []item) int64 {
	var sum int64
	for i := range int64(20) {
		if a, ok := MaxBy(items, func(it item) int64 { return it.score }); ok {
			sum += a.id
		}
		if b, ok := MaxBy(items, func(it item) string { return it.name }); ok {
			sum += b.id
		}
		sum += i
	}
	return sum
}

type Shape interface{ Area() int64 }

type rect struct{ w, h int64 }

func (r rect) Area() int64 { return r.w * r.h }

func TotalArea[T Shape](xs []T) int64 {
	var s int64
	for _, x := range xs {
		s += x.Area()
	}
	return s
}

func ifaceLoop(rects []rect) int64 {
	var sum int64
	for range 20 {
		sum += TotalArea(rects)
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
	var ints []int64
	var items []item
	var rects []rect
	switch w {
	case "hof":
		for i := range int64(hofN) {
			ints = append(ints, i)
		}
	case "maxby":
		seed := int64(7)
		for i := range int64(hofN) {
			seed = lcg(seed)
			items = append(items, item{id: i, score: seed % 1000003, name: fmt.Sprintf("n%d", seed%999983)})
		}
	case "iface":
		for i := range int64(hofN) {
			rects = append(rects, rect{i % 100, i % 37})
		}
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
	case "sortint":
		sum = sortedInt()
	case "sortpt":
		sum = sortedPoint()
	case "hof":
		sum = hofChain(ints)
	case "maxby":
		sum = maxByLoop(items)
	case "iface":
		sum = ifaceLoop(rects)
	case "setat":
		sum = setAt()
	case "stack":
		sum = stack()
	case "fields":
		sum = fields()
	case "grid":
		sum = grid()
	case "insrem":
		sum = insRem()
	case "copyset":
		sum = copySet()
	default:
		sum = -1
	}
	fmt.Printf("%s %d %d\n", w, sum, time.Since(t).Microseconds())
}
