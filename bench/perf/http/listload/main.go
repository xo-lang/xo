// listload: preloads users, then c keep-alive connections loop GET /users for d; prints requests.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	base := flag.String("url", "http://127.0.0.1:18080", "")
	c := flag.Int("c", 1024, "")
	d := flag.Duration("d", 8*time.Second, "")
	pre := flag.Int("preload", 1032, "")
	flag.Parse()
	tr := &http.Transport{MaxIdleConnsPerHost: *c + 8, MaxConnsPerHost: *c + 8}
	cl := &http.Client{Transport: tr}
	for i := 0; i < *pre; i++ {
		b := fmt.Sprintf(`{"name": "User %d", "email": "u%d@example.com", "nickname": "n%d"}`, i, i, i)
		r, err := cl.Post(*base+"/users", "application/json", bytes.NewBufferString(b))
		if err != nil {
			panic(err)
		}
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
	}
	var n atomic.Int64
	var mu sync.Mutex
	var lats []time.Duration
	var wg sync.WaitGroup
	stop := time.Now().Add(*d)
	for w := 0; w < *c; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var mine []time.Duration
			defer func() { mu.Lock(); lats = append(lats, mine...); mu.Unlock() }()
			for time.Now().Before(stop) {
				t0 := time.Now()
				r, err := cl.Get(*base + "/users")
				if err != nil {
					return
				}
				io.Copy(io.Discard, r.Body)
				r.Body.Close()
				mine = append(mine, time.Since(t0))
				n.Add(1)
			}
		}()
	}
	wg.Wait()
	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
	q := func(f float64) time.Duration {
		if len(lats) == 0 {
			return 0
		}
		return lats[int(f*float64(len(lats)-1))]
	}
	fmt.Printf("%d p50=%v p99=%v p999=%v max=%v\n", n.Load(), q(0.5).Round(time.Millisecond), q(0.99).Round(time.Millisecond), q(0.999).Round(time.Millisecond), q(1).Round(time.Millisecond))
}
