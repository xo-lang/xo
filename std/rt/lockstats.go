package rt

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

// Lock statistics for measuring (toolchain.md, "Profiling a built Xo
// program"): with XO_LOCKSTATS=<file> set, every Mutex.with and
// Mutex.with_read records how long it waited to acquire the mutex and how
// long it held it, keyed by the call site, and RunMain writes p50, p99,
// and mean per site to the file when it returns. Off by default; when off
// the cost is one load of lockStatsOn per acquisition.
var (
	lockStatsOn  bool
	lockStatsMu  sync.Mutex
	lockStatsBy  map[lockStatKey]*lockSamples
	lockStatsOut string
)

type lockStatKey struct {
	pc   uintptr
	read bool
}

type lockSamples struct{ wait, hold []time.Duration }

func init() {
	if p := os.Getenv("XO_LOCKSTATS"); p != "" {
		lockStatsOn, lockStatsOut = true, p
		lockStatsBy = map[lockStatKey]*lockSamples{}
	}
}

// lockStatRecord adds one acquisition at the call site pc.
func lockStatRecord(pc uintptr, read bool, wait, hold time.Duration) {
	lockStatsMu.Lock()
	k := lockStatKey{pc, read}
	s := lockStatsBy[k]
	if s == nil {
		s = &lockSamples{}
		lockStatsBy[k] = s
	}
	s.wait = append(s.wait, wait)
	s.hold = append(s.hold, hold)
	lockStatsMu.Unlock()
}

// lockStatsWrite writes the table to the XO_LOCKSTATS file.
func lockStatsWrite() {
	if !lockStatsOn {
		return
	}
	f, err := os.Create(lockStatsOut)
	if err != nil {
		return
	}
	defer f.Close()
	lockStatsMu.Lock()
	defer lockStatsMu.Unlock()
	WriteLockStats(f, lockStatsRows())
}

// LockStatRow is one call site (or operation) of a lock statistics table.
type LockStatRow struct {
	Name       string
	Wait, Hold []time.Duration
}

func lockStatsRows() []LockStatRow {
	var rows []LockStatRow
	for k, s := range lockStatsBy {
		name := "?"
		if fn := runtime.FuncForPC(k.pc); fn != nil {
			name = fn.Name()
			if i := strings.LastIndex(name, "/"); i >= 0 {
				name = name[i+1:]
			}
			file, line := fn.FileLine(k.pc - 1)
			if i := strings.LastIndex(file, "/"); i >= 0 {
				file = file[i+1:]
			}
			name = fmt.Sprintf("%s (%s:%d)", name, file, line)
		}
		if k.read {
			name += " read"
		}
		rows = append(rows, LockStatRow{Name: name, Wait: s.wait, Hold: s.hold})
	}
	slices.SortFunc(rows, func(a, b LockStatRow) int { return strings.Compare(a.Name, b.Name) })
	return rows
}

// WriteLockStats writes a Markdown table: per row the count, and p50, p99,
// and mean of the wait and hold times in microseconds. It sorts the
// samples in place. bench/perf/http/go has its own copy of this format.
func WriteLockStats(w io.Writer, rows []LockStatRow) {
	fmt.Fprintln(w, "| Site | Count | Wait p50 us | Wait p99 us | Wait mean us | Hold p50 us | Hold p99 us | Hold mean us |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|")
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
	for _, r := range rows {
		w50, w99, wm := stat(r.Wait)
		h50, h99, hm := stat(r.Hold)
		fmt.Fprintf(w, "| %s | %d | %s | %s | %s | %s | %s | %s |\n", r.Name, len(r.Hold), us(w50), us(w99), us(wm), us(h50), us(h99), us(hm))
	}
}
