package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// runPipeline is -pipeline mode: conc raw TCP connections, each writing
// depth GET /users/1 requests in one write and then reading the depth
// responses (HTTP/1.1 pipelining, as a server sees from a proxy or a
// pipelining client), for warm then dur. Prints the same JSON as the
// default mode (no latency percentiles) and returns the exit status.
func runPipeline(base string, conc, depth int, warm, dur time.Duration, pid int) int {
	addr := strings.TrimPrefix(base, "http://")
	req := strings.Repeat("GET /users/1 HTTP/1.1\r\nHost: "+addr+"\r\n\r\n", depth)
	var all, errs atomic.Int64
	var stop atomic.Bool
	var wg sync.WaitGroup
	for w := 0; w < conc; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := net.Dial("tcp", addr)
			if err != nil {
				errs.Add(1)
				return
			}
			defer c.Close()
			r := bufio.NewReaderSize(c, 64<<10)
			tp := textproto.NewReader(r)
			for !stop.Load() {
				if _, err := io.WriteString(c, req); err != nil {
					errs.Add(1)
					return
				}
				for i := 0; i < depth; i++ {
					line, err := tp.ReadLine()
					if err != nil || !strings.HasPrefix(line, "HTTP/1.1 200") {
						errs.Add(1)
						return
					}
					h, err := tp.ReadMIMEHeader()
					if err != nil {
						errs.Add(1)
						return
					}
					n, _ := strconv.Atoi(h.Get("Content-Length"))
					if _, err := r.Discard(n); err != nil {
						errs.Add(1)
						return
					}
					all.Add(1)
				}
			}
		}()
	}
	time.Sleep(warm)
	before := all.Load()
	cpu0 := cpuTime(pid)
	t0 := time.Now()
	time.Sleep(dur)
	el := time.Since(t0)
	after := all.Load()
	cpu1 := cpuTime(pid)
	stop.Store(true)
	wg.Wait()
	res := result{Requests: after - before, AllRequests: all.Load(), Errors: errs.Load(), Seconds: el.Seconds(), RPS: float64(after-before) / el.Seconds()}
	if pid > 0 && after > before {
		res.CPUusReq = (cpu1 - cpu0).Seconds() * 1e6 / float64(after-before)
	}
	out, _ := json.Marshal(res)
	fmt.Println(string(out))
	if res.Errors > 0 {
		fmt.Fprintf(os.Stderr, "loadgen: -pipeline: %d errors\n", res.Errors)
		return 1
	}
	return 0
}
