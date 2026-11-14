// The Go twin of ../xo (decision 0616 section 2a): ROUNDS rounds of PER
// trivial goroutines, started and waited for 1000 at a time, under -race;
// prints each round's resident set (Linux VmRSS, KiB).
//
//	main ROUNDS PER
package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

func rssKiB() int {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "VmRSS:"); ok {
			n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "kB")))
			return n
		}
	}
	return 0
}

func main() {
	rounds, per := 50, 200000
	if len(os.Args) > 1 {
		rounds, _ = strconv.Atoi(os.Args[1])
	}
	if len(os.Args) > 2 {
		per, _ = strconv.Atoi(os.Args[2])
	}
	for r := 0; r < rounds; r++ {
		var total int64
		var mu sync.Mutex
		for i := 0; i < per; i += 1000 {
			var wg sync.WaitGroup
			for j := i; j < i+1000 && j < per; j++ {
				wg.Add(1)
				go func(v int) {
					mu.Lock()
					total += int64(v + 1)
					mu.Unlock()
					wg.Done()
				}(j)
			}
			wg.Wait()
		}
		if total != int64(per)*int64(per+1)/2 {
			fmt.Fprintf(os.Stderr, "round %d: sum %d\n", r, total)
			os.Exit(1)
		}
		fmt.Printf("round %d rss %d\n", r, rssKiB())
	}
}
