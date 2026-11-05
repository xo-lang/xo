// The Go twin of ../xo/main.xo (the owner's tests/13-perf micro-go,
// 2026-11-04): the same work and output; the first argument picks one
// workload (all with none).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Rec struct {
	ID    int      `json:"id"`
	Name  string   `json:"name"`
	Score float64  `json:"score"`
	Tags  []string `json:"tags"`
}

func fib(n int) int {
	if n < 2 {
		return n
	}
	return fib(n-1) + fib(n-2)
}

func report(name string, t0 time.Time, sum int) {
	fmt.Println(name, sum, time.Since(t0).Milliseconds())
}

func main() {
	only := ""
	if len(os.Args) > 1 {
		only = os.Args[1]
	}
	on := func(name string) bool { return only == "" || only == name }
	t0 := time.Now()
	if on("fib32") {
		report("fib32", t0, fib(32))
	}

	if on("map1M") {
		t0 = time.Now()
		m := map[int]int{}
		for i := 0; i < 1_000_000; i++ {
			m[i*7] = i
		}
		msum := 0
		for i := 0; i < 1_000_000; i++ {
			msum += m[i*7]
		}
		report("map1M", t0, msum)
	}

	if on("strbuild1M") {
		t0 = time.Now()
		var sb strings.Builder
		for i := 0; i < 1_000_000; i++ {
			sb.WriteString(strconv.Itoa(i))
			sb.WriteString(",")
		}
		report("strbuild1M", t0, sb.Len())
	}

	if on("sort1M") {
		t0 = time.Now()
		xs := make([]int, 0, 1_000_000)
		x := 12345
		for i := 0; i < 1_000_000; i++ {
			x = (x*1103515245 + 12345) % 2147483648
			xs = append(xs, x)
		}
		slices.Sort(xs)
		report("sort1M", t0, xs[500_000])
	}

	// sort1M_copy: the same work as sort1M; the Xo twin's row runs its
	// pre 2026-11-04 form (a sorted copy) for one release-vs-Go report
	if on("sort1M_copy") {
		t0 = time.Now()
		xs := make([]int, 0, 1_000_000)
		x := 12345
		for i := 0; i < 1_000_000; i++ {
			x = (x*1103515245 + 12345) % 2147483648
			xs = append(xs, x)
		}
		slices.Sort(xs)
		report("sort1M_copy", t0, xs[500_000])
	}

	if on("json10kx20") {
		t0 = time.Now()
		recs := make([]Rec, 10_000)
		for i := range recs {
			recs[i] = Rec{i, "rec " + strconv.Itoa(i), float64(i) / 3.0, []string{"a", "b"}}
		}
		jtotal := 0
		for k := 0; k < 20; k++ {
			b, _ := json.Marshal(recs)
			var back []Rec
			if err := json.Unmarshal(b, &back); err != nil {
				panic(err)
			}
			jtotal += len(back) + len(b)
		}
		report("json10kx20", t0, jtotal)
	}

	if on("spawn100k") {
		t0 = time.Now()
		var total atomic.Int64
		var wg sync.WaitGroup
		for i := 0; i < 100_000; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); total.Add(int64(i % 7)) }()
		}
		wg.Wait()
		report("spawn100k", t0, int(total.Load()))
	}
}
