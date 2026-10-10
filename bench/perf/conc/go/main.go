// Command concperf runs the idiomatic Go versions of the Xo programs in
// ../xo: `concperf <name>` prints the same line as the Xo program.
package main

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

func spawn() {
	var count atomic.Int64
	for r := 0; r < 200; r++ {
		var wg sync.WaitGroup
		for i := 0; i < 1000; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				count.Add(int64(i))
			}()
		}
		wg.Wait()
	}
	fmt.Printf("spawn %d\n", count.Load())
}

func fanout() {
	total := 0
	for r := 0; r < 100; r++ {
		res := make([]chan int, 1000)
		for i := range res {
			ch := make(chan int, 1)
			res[i] = ch
			go func() { ch <- i*2 + r }()
		}
		acc := 0
		for _, ch := range res {
			acc += <-ch
		}
		total += acc
	}
	fmt.Printf("fanout %d\n", total)
}

func pingpong() {
	ping, pong := make(chan int), make(chan int)
	go func() {
		for v := range ping {
			pong <- v + 1
		}
		close(pong)
	}()
	v := 0
	for i := 0; i < 100000; i++ {
		ping <- v
		v = <-pong
	}
	close(ping)
	fmt.Printf("pingpong %d\n", v)
}

func mutex() {
	var mu sync.Mutex
	c := 0
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25000; j++ {
				mu.Lock()
				c++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	fmt.Printf("mutex %d\n", c)
}

// rwmutex: 64 goroutines, 4,000 accesses each, one in four a write that
// holds the lock for a Collatz step count, the others reads.
func rwmutex() {
	var mu sync.RWMutex
	c, odd := 0, 0
	var oddMu sync.Mutex
	var wg sync.WaitGroup
	for t := 0; t < 64; t++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 4000; i++ {
				if (i+t)%4 == 0 {
					mu.Lock()
					c += 1 + steps(27+c%2)/1000
					mu.Unlock()
				} else {
					mu.RLock()
					v := c
					mu.RUnlock()
					if v < 0 {
						oddMu.Lock()
						odd++
						oddMu.Unlock()
					}
				}
			}
		}()
	}
	wg.Wait()
	fmt.Printf("rwmutex %d %d\n", c, odd)
}

func steps(x int) int {
	n, c := x, 0
	for n > 1 {
		if n%2 == 0 {
			n /= 2
		} else {
			n = 3*n + 1
		}
		c++
	}
	return c
}

func pmap() {
	const size, chunks = 200000, 64
	total := 0
	for r := 0; r < 20; r++ {
		out := make([]int, chunks)
		var wg sync.WaitGroup
		for k := 0; k < chunks; k++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				acc := 0
				for i := k * size / chunks; i < (k+1)*size/chunks; i++ {
					acc += steps(i + r)
				}
				out[k] = acc
			}()
		}
		wg.Wait()
		for _, v := range out {
			total += v
		}
	}
	fmt.Printf("pmap %d\n", total)
}

func main() {
	runs := map[string]func(){"spawn": spawn, "fanout": fanout, "pingpong": pingpong, "mutex": mutex, "rwmutex": rwmutex, "pmap": pmap}
	if len(os.Args) != 2 || runs[os.Args[1]] == nil {
		fmt.Fprintln(os.Stderr, "usage: concperf spawn|fanout|pingpong|mutex|rwmutex|pmap")
		os.Exit(2)
	}
	runs[os.Args[1]]()
}
