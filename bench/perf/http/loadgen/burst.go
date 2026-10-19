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
	var all, dials, serves []time.Duration
	// dial to response by the order dials completed in each round: the
	// first third, the middle, the last (a server that runs the newest
	// connection first serves the first third last)
	var thirds [3][]time.Duration
	var fails, total int
	var walls []time.Duration
	for r := 0; r < reps; r++ {
		lats := make([]time.Duration, n)
		dl := make([]time.Duration, n)
		done := make([]time.Time, n)
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
				dl[i] = time.Since(t0)
				done[i] = time.Now()
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
			dials = append(dials, dl[i])
			serves = append(serves, lats[i]-dl[i])
		}
		var ok []int
		for i := 0; i < n; i++ {
			if errs[i] == "" {
				ok = append(ok, i)
			}
		}
		sort.Slice(ok, func(a, b int) bool { return done[ok[a]].Before(done[ok[b]]) })
		for k, i := range ok {
			thirds[k*3/len(ok)] = append(thirds[k*3/len(ok)], lats[i]-dl[i])
		}
		fails += f
		total += n
		fmt.Printf("round %d: %d of %d answered, wall %v, failures %v\n", r, n-f, n, wall.Round(time.Microsecond), kinds)
		time.Sleep(200 * time.Millisecond)
	}
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	sort.Slice(dials, func(i, j int) bool { return dials[i] < dials[j] })
	sort.Slice(serves, func(i, j int) bool { return serves[i] < serves[j] })
	sort.Slice(walls, func(i, j int) bool { return walls[i] < walls[j] })
	pct := func(xs []time.Duration, p float64) time.Duration {
		if len(xs) == 0 {
			return 0
		}
		return xs[int(float64(len(xs)-1)*p)].Round(time.Microsecond)
	}
	fmt.Printf("burst %d x %d: failed %d of %d; wall p50 %v max %v; latency p50 %v p99 %v max %v\n", n, reps, fails, total, pct(walls, 0.5), pct(walls, 1), pct(all, 0.5), pct(all, 0.99), pct(all, 1))
	// where a connection's latency goes: the dial (the server's accept
	// queue, until the kernel completes the handshake) and from the dial
	// to the response's first line (accept, first read, the handler)
	for k := range thirds {
		sort.Slice(thirds[k], func(i, j int) bool { return thirds[k][i] < thirds[k][j] })
	}
	fmt.Printf("burst %d x %d: dial to response by dial order: first third p50 %v p99 %v; middle p50 %v p99 %v; last p50 %v p99 %v\n", n, reps, pct(thirds[0], 0.5), pct(thirds[0], 0.99), pct(thirds[1], 0.5), pct(thirds[1], 0.99), pct(thirds[2], 0.5), pct(thirds[2], 0.99))
	fmt.Printf("burst %d x %d: dial p50 %v p99 %v max %v; dial to response p50 %v p99 %v max %v\n", n, reps, pct(dials, 0.5), pct(dials, 0.99), pct(dials, 1), pct(serves, 0.5), pct(serves, 0.99), pct(serves, 1))
	if fails > 0 {
		return 1
	}
	return 0
}
