package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"slices"
	"sync"
	"syscall"
	"time"
)

// Store lock statistics, off by default: with HOLDSTATS=<file> every store
// operation records how long it waited for s.mu and how long it held it,
// and SIGTERM writes count, p50, p99, and mean per operation to the file
// before exiting (the same table as XO_LOCKSTATS of the Xo runtime).

const (
	opCreate = iota
	opGet
	opList
	opUpdate
	opDelete
	nOps
)

var opNames = [nOps]string{"create", "get", "list", "update", "delete"}

var (
	statsOn  bool
	statsMu  sync.Mutex
	statWait [nOps][]time.Duration
	statHold [nOps][]time.Duration
)

func init() {
	hold := os.Getenv("HOLDSTATS")
	mem := os.Getenv("MEMSTATS")
	if hold == "" && mem == "" {
		return
	}
	statsOn = hold != ""
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-c
		if hold != "" {
			writeStats(hold)
		}
		if mem != "" {
			writeMemStats(mem)
		}
		os.Exit(0)
	}()
}

// writeMemStats writes the allocation counters of the whole run, with
// MEMSTATS=<file> (the same format as XO_MEMSTATS of the Xo runtime):
// bench/perf/http/allocs.sh divides their growth by the request count.
func writeMemStats(path string) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	os.WriteFile(path, []byte(fmt.Sprintf("mallocs %d\ntotal_alloc %d\nnum_gc %d\n", ms.Mallocs, ms.TotalAlloc, ms.NumGC)), 0o644)
}

func statNow() time.Time {
	if !statsOn {
		return time.Time{}
	}
	return time.Now()
}

func statRecord(op int, t0, t1, t2 time.Time) {
	if !statsOn {
		return
	}
	statsMu.Lock()
	statWait[op] = append(statWait[op], t1.Sub(t0))
	statHold[op] = append(statHold[op], t2.Sub(t1))
	statsMu.Unlock()
}

func writeStats(path string) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	statsMu.Lock()
	defer statsMu.Unlock()
	us := func(d time.Duration) string { return fmt.Sprintf("%.2f", float64(d.Nanoseconds())/1e3) }
	stat := func(xs []time.Duration) (p50, p99, mean time.Duration) {
		if len(xs) == 0 {
			return
		}
		slices.Sort(xs)
		var sum time.Duration
		for _, x := range xs {
			sum += x
		}
		return xs[(len(xs)-1)/2], xs[(len(xs)-1)*99/100], sum / time.Duration(len(xs))
	}
	fmt.Fprintln(f, "| Site | Count | Wait p50 us | Wait p99 us | Wait mean us | Hold p50 us | Hold p99 us | Hold mean us |")
	fmt.Fprintln(f, "|---|---|---|---|---|---|---|---|")
	for op := range nOps {
		w50, w99, wm := stat(statWait[op])
		h50, h99, hm := stat(statHold[op])
		fmt.Fprintf(f, "| %s | %d | %s | %s | %s | %s | %s | %s |\n", opNames[op], len(statHold[op]), us(w50), us(w99), us(wm), us(h50), us(h99), us(hm))
	}
}
