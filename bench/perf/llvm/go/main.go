// Hand written Go versions of the LLVM backend benchmarks in ../xo.
// Usage: llperf <workload>. Each prints the same checksum line as the Xo
// program, using the same algorithm and sizes, written as idiomatic Go
// (plain int arithmetic: Go does not check overflow).
package main

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

func fib(n int) int {
	if n < 2 {
		return n
	}
	return fib(n-1) + fib(n-2)
}

func sieve(n int) int {
	flags := make([]bool, 0)
	for i := 0; i < n+1; i++ {
		flags = append(flags, true)
	}
	count := 0
	for i := 2; i < n+1; i++ {
		if flags[i] {
			count++
			for j := i * i; j <= n; j += i {
				flags[j] = false
			}
		}
	}
	return count
}

type body struct{ x, y, z, vx, vy, vz, mass float64 }

const (
	pi   = 3.141592653589793
	days = 365.24
)

func solarMass() float64 { return 4.0 * pi * pi }

func bodies() []body {
	sm := solarMass()
	return []body{
		{0, 0, 0, 0, 0, 0, sm},
		{4.84143144246472090e+00, -1.16032004402742839e+00, -1.03622044471123109e-01, 1.66007664274403694e-03 * days, 7.69901118419740425e-03 * days, -6.90460016972063023e-05 * days, 9.54791938424326609e-04 * sm},
		{8.34336671824457987e+00, 4.12479856412430479e+00, -4.03523417114321381e-01, -2.76742510726862411e-03 * days, 4.99852801234917238e-03 * days, 2.30417297573763929e-05 * days, 2.85885980666130812e-04 * sm},
		{1.28943695621391310e+01, -1.51111514016986312e+01, -2.23307578892655734e-01, 2.96460137564761618e-03 * days, 2.37847173959480950e-03 * days, -2.96589568540237556e-05 * days, 4.36624404335156298e-05 * sm},
		{1.53796971148509165e+01, -2.59193146099879641e+01, 1.79258772950371181e-01, 2.68067772490389322e-03 * days, 1.62824170038242295e-03 * days, -9.51592254519715870e-05 * days, 5.15138902046611451e-05 * sm},
	}
}

func offset(bs []body) {
	var px, py, pz float64
	for _, b := range bs {
		px += b.vx * b.mass
		py += b.vy * b.mass
		pz += b.vz * b.mass
	}
	sm := solarMass()
	bs[0].vx = 0 - px/sm
	bs[0].vy = 0 - py/sm
	bs[0].vz = 0 - pz/sm
}

func energy(bs []body) float64 {
	e := 0.0
	for i := range bs {
		b := bs[i]
		e += 0.5 * b.mass * (b.vx*b.vx + b.vy*b.vy + b.vz*b.vz)
		for j := i + 1; j < len(bs); j++ {
			c := bs[j]
			dx, dy, dz := b.x-c.x, b.y-c.y, b.z-c.z
			e -= b.mass * c.mass / math.Sqrt(dx*dx+dy*dy+dz*dz)
		}
	}
	return e
}

func advance(bs []body, dt float64) {
	for i := range bs {
		for j := i + 1; j < len(bs); j++ {
			dx := bs[i].x - bs[j].x
			dy := bs[i].y - bs[j].y
			dz := bs[i].z - bs[j].z
			d2 := dx*dx + dy*dy + dz*dz
			mag := dt / (d2 * math.Sqrt(d2))
			mi := bs[i].mass * mag
			mj := bs[j].mass * mag
			bs[i].vx -= dx * mj
			bs[i].vy -= dy * mj
			bs[i].vz -= dz * mj
			bs[j].vx += dx * mi
			bs[j].vy += dy * mi
			bs[j].vz += dz * mi
		}
	}
	for i := range bs {
		bs[i].x += dt * bs[i].vx
		bs[i].y += dt * bs[i].vy
		bs[i].z += dt * bs[i].vz
	}
}

type rec struct {
	key, id int
	weight  float64
}

func less(a, b rec) bool {
	if a.key != b.key {
		return a.key < b.key
	}
	return a.id < b.id
}

func quicksort(xs []rec, lo, hi int) {
	if lo >= hi {
		return
	}
	pivot := xs[lo+(hi-lo)/2]
	i, j := lo, hi
	for i <= j {
		for less(xs[i], pivot) {
			i++
		}
		for less(pivot, xs[j]) {
			j--
		}
		if i <= j {
			xs[i], xs[j] = xs[j], xs[i]
			i++
			j--
		}
	}
	quicksort(xs, lo, j)
	quicksort(xs, i, hi)
}

func sortWorkload() int {
	sum := 0
	seed := 42
	for round := 0; round < 3; round++ {
		xs := make([]rec, 0)
		for i := 0; i < 1000000; i++ {
			seed = (seed*1103515245 + 12345) % 2147483648
			xs = append(xs, rec{key: seed % 100000, id: i, weight: float64(i%97) * 0.5})
		}
		quicksort(xs, 0, len(xs)-1)
		ok := 1
		for i := 1; i < len(xs); i++ {
			if less(xs[i], xs[i-1]) {
				ok = 0
			}
		}
		sum += xs[0].key + xs[500000].id + xs[999999].key + ok*(round+1)
	}
	return sum
}

type opKind int

const (
	opPush opKind = iota
	opLoad
	opStore
	opAdd
	opSub
	opMul
	opMod
	opJumpIfZero
	opJump
	opHalt
)

type op struct {
	kind opKind
	arg  int
}

func pop(stack *[]int, def int) int {
	s := *stack
	if len(s) == 0 {
		return def
	}
	v := s[len(s)-1]
	*stack = s[:len(s)-1]
	return v
}

func run(prog []op, n int) int {
	var stack []int
	slots := []int{0, 0, 0, 0}
	slots[0] = n
	pc := 0
	steps := 0
loop:
	for {
		steps++
		o := prog[pc]
		switch o.kind {
		case opPush:
			stack = append(stack, o.arg)
		case opLoad:
			stack = append(stack, slots[o.arg])
		case opStore:
			slots[o.arg] = pop(&stack, 0)
		case opAdd:
			b := pop(&stack, 0)
			a := pop(&stack, 0)
			stack = append(stack, a+b)
		case opSub:
			b := pop(&stack, 0)
			a := pop(&stack, 0)
			stack = append(stack, a-b)
		case opMul:
			b := pop(&stack, 0)
			a := pop(&stack, 0)
			stack = append(stack, a*b)
		case opMod:
			b := pop(&stack, 1)
			a := pop(&stack, 0)
			stack = append(stack, a%b)
		case opJumpIfZero:
			if pop(&stack, 0) == 0 {
				pc = o.arg - 1
			}
		case opJump:
			pc = o.arg - 1
		case opHalt:
			break loop
		}
		pc++
	}
	return slots[1]*1000 + steps%1000
}

func interpProg() []op {
	return []op{
		{opLoad, 0}, {opJumpIfZero, 19},
		{opLoad, 1}, {opLoad, 0}, {opLoad, 0}, {opMul, 0}, {opAdd, 0}, {opPush, 1000003}, {opMod, 0}, {opStore, 1},
		{opLoad, 0}, {opPush, 1}, {opSub, 0}, {opStore, 0},
		{opPush, 0}, {opJumpIfZero, 0},
		{opHalt, 0}, {opHalt, 0}, {opHalt, 0}, {opHalt, 0},
	}
}

// runTuned is the interpreter as a Go programmer tuning it would write
// it: a fixed array stack with an index instead of a slice with
// pops that default on empty. Not a translation of the Xo program; it
// shows how much of the interp gap is the algorithm (`llperf
// interp_tuned`, not part of run.sh).
func runTuned(prog []op, n int) int {
	var stack [64]int
	sp := 0
	slots := [4]int{n, 0, 0, 0}
	pc, steps := 0, 0
	for {
		steps++
		o := prog[pc]
		switch o.kind {
		case opPush:
			stack[sp] = o.arg
			sp++
		case opLoad:
			stack[sp] = slots[o.arg]
			sp++
		case opStore:
			sp--
			slots[o.arg] = stack[sp]
		case opAdd:
			sp--
			stack[sp-1] += stack[sp]
		case opSub:
			sp--
			stack[sp-1] -= stack[sp]
		case opMul:
			sp--
			stack[sp-1] *= stack[sp]
		case opMod:
			sp--
			stack[sp-1] %= stack[sp]
		case opJumpIfZero:
			sp--
			if stack[sp] == 0 {
				pc = o.arg - 1
			}
		case opJump:
			pc = o.arg - 1
		case opHalt:
			return slots[1]*1000 + steps%1000
		}
		pc++
	}
}

func collatzSteps(start int) int {
	n, s := start, 0
	for n != 1 {
		if n%2 == 0 {
			n = n / 2
		} else {
			n = 3*n + 1
		}
		s++
	}
	return s
}

// stringsWorkload builds strings the idiomatic Go way: concatenation for
// the words and a strings.Builder for the accumulated string (Xo's
// `acc = acc + w` appends in place when acc is unique).
func stringsWorkload() int {
	total := 0
	for round := 0; round < 10; round++ {
		words := make([]string, 0)
		for i := 0; i < 200000; i++ {
			words = append(words, "w"+strconv.Itoa(i%1000)+"-"+strconv.Itoa(round))
		}
		var acc strings.Builder
		k := 0
		for len(words) > 0 {
			w := words[len(words)-1]
			words = words[:len(words)-1]
			if k%10 == 0 {
				acc.WriteString(w)
			}
			k++
			total += len(w)
		}
		total += acc.Len()
	}
	return total
}

func main() {
	w := "fib"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	switch w {
	case "fib":
		fmt.Println("fib " + strconv.Itoa(fib(40)))
	case "sieve":
		total := 0
		for i := 0; i < 5; i++ {
			total += sieve(10000000)
		}
		fmt.Println("sieve " + strconv.Itoa(total))
	case "nbody":
		bs := bodies()
		offset(bs)
		before := energy(bs)
		for i := 0; i < 5000000; i++ {
			advance(bs, 0.01)
		}
		after := energy(bs)
		fmt.Printf("nbody %d %d\n", int64(before*1e9), int64(after*1e9))
	case "sort":
		fmt.Println("sort " + strconv.Itoa(sortWorkload()))
	case "interp":
		fmt.Println("interp " + strconv.Itoa(run(interpProg(), 3000000)))
	case "interp_tuned":
		fmt.Println("interp " + strconv.Itoa(runTuned(interpProg(), 3000000)))
	case "collatz":
		total, best := 0, 0
		for i := 1; i < 3000000; i++ {
			s := collatzSteps(i)
			total += s
			best = max(best, s)
		}
		fmt.Printf("collatz %d %d\n", total, best)
	case "strings":
		fmt.Println("strings " + strconv.Itoa(stringsWorkload()))
	default:
		fmt.Fprintln(os.Stderr, "unknown workload", w)
		os.Exit(2)
	}
}
