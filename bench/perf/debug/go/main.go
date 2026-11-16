// Go twin of bench/perf/debug: std/debug's routes as a Go server would
// mount them (an auth check, then text counters from runtime/metrics, the
// open descriptors counted from the fd directory), called through
// net/http/httptest; mem asks the system for the physical memory as
// std/debug does (physmem_*.go) and loads a high-water mark.
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"runtime/metrics"
	"strings"
	"sync/atomic"
)

// peak stands in for std/debug's high-water mark: one atomic load.
var peak atomic.Int64

func fdsOpen() int {
	dir := "/proc/self/fd"
	if runtime.GOOS == "darwin" {
		dir = "/dev/fd"
	}
	f, err := os.Open(dir)
	if err != nil {
		return -1
	}
	names, err := f.Readdirnames(-1)
	f.Close()
	if err != nil || len(names) == 0 {
		return -1
	}
	return len(names) - 1
}

func router(auth func(*http.Request) bool) *http.ServeMux {
	all := metrics.All()
	samples := make([]metrics.Sample, 0, len(all))
	for _, d := range all {
		if d.Kind == metrics.KindUint64 {
			samples = append(samples, metrics.Sample{Name: d.Name})
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/sched", func(w http.ResponseWriter, r *http.Request) {
		if !auth(r) {
			http.Error(w, "forbidden", 403)
			return
		}
		metrics.Read(samples)
		var b strings.Builder
		for _, s := range samples {
			fmt.Fprintf(&b, "%s %d\n", s.Name, s.Value.Uint64())
		}
		fmt.Fprintf(&b, "fds_open %d\n", fdsOpen())
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte(b.String()))
	})
	return mux
}

func call(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func main() {
	w := "none"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	sum := 0
	switch w {
	case "refused":
		no := router(func(*http.Request) bool { return false })
		for i := 0; i < 200000; i++ {
			sum += call(no, "/sched").Code
		}
	case "sched":
		ok := router(func(*http.Request) bool { return true })
		for i := 0; i < 20000; i++ {
			r := call(ok, "/sched")
			sum += r.Code
			if r.Body.Len() > 100 {
				sum++
			}
		}
	case "mem":
		for i := 0; i < 1000000; i++ {
			if physMem() > 0 {
				sum++
			}
			if peak.Load() >= 0 {
				sum++
			}
		}
	}
	fmt.Printf("%s %d\n", w, sum)
}
