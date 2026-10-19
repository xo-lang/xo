package main

// -burst N: how a server's accept loop drains a burst of connections
// (the native accept drain rate against Go's, the lead's queue item from
// gotchas/h2diff-go-reference-streams-under-load.md). Every round dials
// N connections at once (each goroutine waits on one start signal), sends
// one request on each, and reads the response; it reports the
// connections that failed (refused, reset, timed out: a listen queue that
// overflowed, or an accept loop too slow for the clients' timeouts), the
// time until the last response, and the per connection latency.

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

func runBurst(base string, n, reps int) int {
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" {
		fmt.Fprintln(os.Stderr, "loadgen: -burst needs an http:// URL")
		return 2
	}
	addr := u.Host
	req := "GET /users/1 HTTP/1.1\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n"
	var all []time.Duration
	var fails, total int
	var walls []time.Duration
	for r := 0; r < reps; r++ {
		lats := make([]time.Duration, n)
		errs := make([]string, n)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				t0 := time.Now()
				c, err := net.DialTimeout("tcp", addr, 5*time.Second)
				if err != nil {
					errs[i] = "dial: " + err.Error()
					return
				}
				defer c.Close()
				c.SetDeadline(time.Now().Add(10 * time.Second))
				if _, err := c.Write([]byte(req)); err != nil {
					errs[i] = "write: " + err.Error()
					return
				}
				line, err := bufio.NewReader(c).ReadString('\n')
				if err != nil {
					errs[i] = "read: " + err.Error()
					return
				}
				if !strings.HasPrefix(line, "HTTP/1.1 200") {
					errs[i] = "status: " + strings.TrimSpace(line)
					return
				}
				lats[i] = time.Since(t0)
			}(i)
		}
		t0 := time.Now()
		close(start)
		wg.Wait()
		wall := time.Since(t0)
		walls = append(walls, wall)
		f := 0
		kinds := map[string]int{}
		for i := 0; i < n; i++ {
			if errs[i] != "" {
				f++
				k := errs[i]
				if j := strings.LastIndex(k, ": "); j >= 0 {
					k = k[:strings.Index(k, ":")] + ":" + k[j+1:]
				}
				kinds[k]++
				continue
			}
			all = append(all, lats[i])
		}
		fails += f
		total += n
		fmt.Printf("round %d: %d of %d answered, wall %v, failures %v\n", r, n-f, n, wall.Round(time.Microsecond), kinds)
		time.Sleep(200 * time.Millisecond)
	}
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	sort.Slice(walls, func(i, j int) bool { return walls[i] < walls[j] })
	pct := func(xs []time.Duration, p float64) time.Duration {
		if len(xs) == 0 {
			return 0
		}
		return xs[int(float64(len(xs)-1)*p)].Round(time.Microsecond)
	}
	fmt.Printf("burst %d x %d: failed %d of %d; wall p50 %v max %v; latency p50 %v p99 %v max %v\n", n, reps, fails, total, pct(walls, 0.5), pct(walls, 1), pct(all, 0.5), pct(all, 0.99), pct(all, 1))
	if fails > 0 {
		return 1
	}
	return 0
}
