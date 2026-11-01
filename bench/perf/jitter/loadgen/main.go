// The jitter bench's load generator (bench/perf/jitter): -c workers, each on
// its own keep-alive connection, send GET -path back to back for -warmup
// then -d, and every measured request's latency lands in a preallocated
// per-worker array. The client speaks raw HTTP/1.1 (a fixed request, the
// response read by Content-Length) and runs with the GC off, so its own
// allocations and pauses do not add to the tail it measures. Prints one
// JSON line: requests, rps, p50/p99/p99.9/max latency in microseconds, the
// coefficient of variation of the per-second request counts, and with -pid
// the server's CPU (user + system from /proc, Linux) per request.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type result struct {
	Requests  int64   `json:"requests"`
	Errors    int64   `json:"errors"`
	RPS       float64 `json:"rps"`
	P50us     float64 `json:"p50_us"`
	P99us     float64 `json:"p99_us"`
	P999us    float64 `json:"p999_us"`
	MaxUs     float64 `json:"max_us"`
	SecCV     float64 `json:"sec_cv"` // stddev/mean of the per-second request counts
	Rate      float64 `json:"rate"`   // the open loop's target, 0 closed loop
	CPUusReq  float64 `json:"cpu_us_per_req"`
	BodyBytes int     `json:"body_bytes"`
	// The open loop's two parts of the latency above: how late this generator
	// sent (its own timers) and the send to the response's end (the server).
	LateP50us float64 `json:"late_p50_us,omitempty"`
	LateP99us float64 `json:"late_p99_us,omitempty"`
	SvcP50us  float64 `json:"svc_p50_us,omitempty"`
	SvcP99us  float64 `json:"svc_p99_us,omitempty"`
	SvcP999us float64 `json:"svc_p999_us,omitempty"`
}

// cpuTicks is the user + system time of pid in clock ticks (Linux), or -1.
func cpuTicks(pid int) int64 {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return -1
	}
	s := string(b)
	f := strings.Fields(s[strings.LastIndexByte(s, ')')+2:])
	u, _ := strconv.ParseInt(f[11], 10, 64)
	k, _ := strconv.ParseInt(f[12], 10, 64)
	return u + k
}

func main() {
	addr := flag.String("addr", "127.0.0.1:18080", "server address")
	path := flag.String("path", "/hello", "request path")
	conc := flag.Int("c", 32, "concurrent connections")
	dur := flag.Duration("d", 10*time.Second, "measured duration")
	warm := flag.Duration("warmup", 2*time.Second, "warmup before measuring")
	pid := flag.Int("pid", 0, "server pid for CPU per request")
	rate := flag.Float64("rate", 0, "open loop: requests a second in all, each connection sending on its own fixed schedule, latency counted from the scheduled send (no coordinated omission); 0 sends back to back")
	flag.Parse()
	debug.SetGCPercent(-1)

	req := []byte("GET " + *path + " HTTP/1.1\r\nHost: " + *addr + "\r\n\r\n")
	// samples a worker keeps: 8M in all, at least 64k each
	capPer := max(1<<16, (8<<20) / *conc)
	lat := make([][]int32, *conc)
	late := make([][]int32, *conc)
	var measuring atomic.Bool
	var stop atomic.Bool
	var errs atomic.Int64
	var bodyLen atomic.Int64
	secs := int(dur.Seconds()) + 2
	perSec := make([]atomic.Int64, secs)
	// start is written before measuring is set and read after it is seen
	var start time.Time
	var wg sync.WaitGroup
	// open loop: connection w sends at base + w*iv/conc + k*iv
	iv := time.Duration(0)
	if *rate > 0 {
		iv = time.Duration(float64(*conc) / *rate * 1e9)
	}
	base := time.Now().Add(100 * time.Millisecond)
	for w := 0; w < *conc; w++ {
		lat[w] = make([]int32, 0, capPer)
		if *rate > 0 {
			late[w] = make([]int32, 0, capPer)
		}
		c, err := net.Dial("tcp", *addr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "dial:", err)
			os.Exit(1)
		}
		wg.Add(1)
		go func(w int, c net.Conn) {
			defer wg.Done()
			defer c.Close()
			r := bufio.NewReaderSize(c, 64<<10)
			body := make([]byte, 1<<20)
			next := base.Add(iv * time.Duration(w) / time.Duration(*conc))
			for !stop.Load() {
				t0 := time.Now()
				sent := t0
				if iv > 0 {
					if d := time.Until(next); d > 0 {
						time.Sleep(d)
					}
					sent = time.Now()
					t0 = next
					next = next.Add(iv)
				}
				if _, err := c.Write(req); err != nil {
					errs.Add(1)
					return
				}
				n := -1
				for {
					line, err := r.ReadSlice('\n')
					if err != nil {
						errs.Add(1)
						return
					}
					if len(line) <= 2 {
						break
					}
					if len(line) > 15 && bytes.EqualFold(line[:15], []byte("content-length:")) {
						n, _ = strconv.Atoi(string(bytes.TrimSpace(line[15:])))
					}
				}
				if n < 0 || n > len(body) {
					fmt.Fprintln(os.Stderr, "response without a usable Content-Length")
					errs.Add(1)
					return
				}
				if _, err := ioReadFull(r, body[:n]); err != nil {
					errs.Add(1)
					return
				}
				d := time.Since(t0)
				if measuring.Load() {
					if len(lat[w]) < capPer {
						lat[w] = append(lat[w], int32(d.Microseconds()))
						if iv > 0 {
							late[w] = append(late[w], int32(sent.Sub(t0).Microseconds()))
						}
					}
					s := int(time.Since(start) / time.Second)
					if s < secs {
						perSec[s].Add(1)
					}
				}
				bodyLen.Store(int64(n))
			}
		}(w, c)
	}
	time.Sleep(*warm)
	t0 := cpuTicks(*pid)
	start = time.Now()
	measuring.Store(true)
	time.Sleep(*dur)
	measuring.Store(false)
	elapsed := time.Since(start)
	t1 := cpuTicks(*pid)
	stop.Store(true)
	wg.Wait()

	var all []int32
	for _, l := range lat {
		all = append(all, l...)
	}
	slices.Sort(all)
	q := func(p float64) float64 {
		if len(all) == 0 {
			return 0
		}
		i := int(math.Ceil(p*float64(len(all)))) - 1
		if i < 0 {
			i = 0
		}
		return float64(all[i])
	}
	full := int(dur.Seconds())
	var sum, sq float64
	for s := 0; s < full; s++ {
		v := float64(perSec[s].Load())
		sum += v
		sq += v * v
	}
	cv := 0.0
	if full > 1 && sum > 0 {
		m := sum / float64(full)
		cv = math.Sqrt(math.Max(sq/float64(full)-m*m, 0)) / m
	}
	res := result{Requests: int64(len(all)), Errors: errs.Load(), RPS: float64(len(all)) / elapsed.Seconds(),
		P50us: q(0.50), P99us: q(0.99), P999us: q(0.999), SecCV: cv, Rate: *rate, BodyBytes: int(bodyLen.Load())}
	if len(all) > 0 {
		res.MaxUs = float64(all[len(all)-1])
	}
	if t0 >= 0 && t1 >= 0 && len(all) > 0 {
		// clock ticks are 100 a second on Linux
		res.CPUusReq = float64(t1-t0) * 1e4 / float64(len(all))
	}
	if *rate > 0 {
		var ls, ss []int32
		for w, l := range lat {
			for i, v := range l {
				ls = append(ls, late[w][i])
				ss = append(ss, v-late[w][i])
			}
		}
		slices.Sort(ls)
		slices.Sort(ss)
		pick := func(a []int32, p float64) float64 {
			if len(a) == 0 {
				return 0
			}
			i := int(math.Ceil(p*float64(len(a)))) - 1
			if i < 0 {
				i = 0
			}
			return float64(a[i])
		}
		res.LateP50us, res.LateP99us = pick(ls, 0.50), pick(ls, 0.99)
		res.SvcP50us, res.SvcP99us, res.SvcP999us = pick(ss, 0.50), pick(ss, 0.99), pick(ss, 0.999)
	}
	out, _ := json.Marshal(res)
	fmt.Println(string(out))
}

func ioReadFull(r *bufio.Reader, b []byte) (int, error) {
	n := 0
	for n < len(b) {
		m, err := r.Read(b[n:])
		n += m
		if err != nil {
			return n, err
		}
	}
	return n, nil
}
