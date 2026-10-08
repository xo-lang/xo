package rt

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"strconv"
	"strings"
	"sync/atomic"
)

// Heap goal of Xo programs (decision 0065).
//
// Go's collector sets the heap goal to the marked heap plus GOGC percent of
// it (plus stacks and globals), and never below a minimum of 4 MB times
// GOGC/100. For a server with a small live heap on many Ps that minimum is
// the goal, and each cycle the pacer counts span slack of every P that
// allocated (knowledgebase/gotchas/small-heap-gc-per-p-spans.md): on 12 Ps
// half to three quarters of the 4 MB headroom, so the GC runs after a
// fraction of the bytes the goal suggests.
//
// RunMain raises the floor of the goal to a minimum heap (gcMinHeapDefault,
// `gc min_heap` in xo.mod, XO_GC_MIN_HEAP) and leaves the proportional part
// at GOGC=100: a program whose live heap is past half the floor collects
// exactly as Go would. It does so with a percent chosen after every cycle:
// the largest percent at or above the base whose goal (both Go's formula
// and its scaled minimum) stays within the floor. The environment always
// wins: with GOGC set nothing is tuned, and with GOMEMLIMIT set the limit
// from xo.mod is not applied.

// gcMinHeapDefault is the floor of the heap goal when neither xo.mod nor
// XO_GC_MIN_HEAP sets one: 2 MiB per P, at least 16 MiB. The slack the
// pacer counts per cycle grows with the number of Ps (about 0.1 to 0.15
// MB per P measured), so the floor does too.
func gcMinHeapDefault(procs int) int64 { return max(16<<20, int64(procs)*(2<<20)) }

// goHeapMinimum is Go's minimum heap goal at GOGC=100.
const goHeapMinimum = 4 << 20

// gcConf is the GC configuration from xo.mod, set by the generated main
// (GCConfig) before RunMain.
var gcConf struct {
	percent  int64 // < 0: not set
	minHeap  int64 // < 0: not set
	memLimit int64 // < 0: not set
	off      bool  // gc percent off
}

func init() { gcConf.percent, gcConf.minHeap, gcConf.memLimit = -1, -1, -1 }

// GCConfig records the `gc` lines of the main module's xo.mod: percent
// (-1 unset, -2 off), minimum heap and memory limit in bytes (-1 unset).
func GCConfig(percent, minHeap, memLimit int64) {
	gcConf.percent, gcConf.minHeap, gcConf.memLimit = percent, minHeap, memLimit
	if percent == -2 {
		gcConf.percent, gcConf.off = -1, true
	}
}

// gcTuner keeps the heap goal at or above floor.
type gcTuner struct {
	floor   uint64
	base    int64
	last    atomic.Int64 // the percent last set
	samples []metrics.Sample
}

// curGCTuner is the running tuner; a tuner that is no longer current
// stops at its next cycle.
var curGCTuner atomic.Pointer[gcTuner]

// gcSetup applies the heap goal settings at the start of RunMain.
func gcSetup() {
	if v, ok := os.LookupEnv("GOMEMLIMIT"); !ok || v == "" {
		if gcConf.memLimit > 0 {
			debug.SetMemoryLimit(gcConf.memLimit)
		}
	}
	curGCTuner.Store(nil)
	if v, ok := os.LookupEnv("GOGC"); ok && v != "" {
		return // the environment chose
	}
	if gcConf.off {
		debug.SetGCPercent(-1)
		return
	}
	base := int64(100)
	if gcConf.percent > 0 {
		base = gcConf.percent
	}
	floor := gcMinHeapDefault(runtime.GOMAXPROCS(0))
	if gcConf.minHeap >= 0 {
		floor = gcConf.minHeap
	}
	if v := os.Getenv("XO_GC_MIN_HEAP"); v != "" {
		if n, err := parseBytes(v); err == nil {
			floor = n
		} else {
			stderrWrite("xo: XO_GC_MIN_HEAP: " + err.Error() + "\n")
		}
	}
	if floor*100/base <= goHeapMinimum {
		// Go's own minimum is at least the floor: a plain percent
		if base != 100 {
			debug.SetGCPercent(int(base))
		}
		return
	}
	t := &gcTuner{floor: uint64(floor), base: base, samples: []metrics.Sample{
		{Name: "/gc/heap/live:bytes"},
		{Name: "/gc/scan/stack:bytes"},
		{Name: "/gc/scan/globals:bytes"},
	}}
	curGCTuner.Store(t)
	t.tune()
	t.arm()
}

// percentFor is the percent that puts the heap goal at the floor for a
// marked heap of live bytes and scan roots of roots bytes (stacks and
// globals), or base when the base goal is already past the floor.
func (t *gcTuner) percentFor(live, roots uint64) int64 {
	// Go: goal = max(live + (live+roots)*p/100, goHeapMinimum*p/100)
	p := int64(t.floor * 100 / goHeapMinimum) // the minimum alone reaches the floor
	if live < t.floor {
		if q := int64((t.floor - live) * 100 / max(live+roots, 1)); q < p {
			p = q
		}
	} else {
		p = 0
	}
	return max(p, t.base)
}

// tune sets the percent for the heap the last GC marked.
func (t *gcTuner) tune() {
	metrics.Read(t.samples)
	var v [3]uint64
	for i, s := range t.samples {
		if s.Value.Kind() == metrics.KindUint64 {
			v[i] = s.Value.Uint64()
		}
	}
	p := t.percentFor(v[0], v[1]+v[2])
	if t.last.Load() != p {
		t.last.Store(p)
		debug.SetGCPercent(int(p))
	}
}

// gcSentinel is garbage whose finalizer runs once after each GC cycle.
type gcSentinel struct{ _ [16]byte }

// arm schedules tune after the next GC.
func (t *gcTuner) arm() {
	s := new(gcSentinel)
	runtime.SetFinalizer(s, func(*gcSentinel) {
		if curGCTuner.Load() != t {
			return
		}
		t.tune()
		t.arm()
	})
}

// parseBytes parses a byte size (XO_GC_MIN_HEAP; internal/check parses xo.mod the same way): a number with an optional unit B, KiB,
// MiB, GiB, TiB (the GOMEMLIMIT spelling) or KB, MB, GB, TB (powers of 1000).
func parseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	units := []struct {
		suf string
		n   int64
	}{{"KiB", 1 << 10}, {"MiB", 1 << 20}, {"GiB", 1 << 30}, {"TiB", 1 << 40}, {"KB", 1e3}, {"MB", 1e6}, {"GB", 1e9}, {"TB", 1e12}, {"B", 1}}
	mul := int64(1)
	for _, u := range units {
		if strings.HasSuffix(s, u.suf) {
			s, mul = strings.TrimSuffix(s, u.suf), u.n
			break
		}
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n < 0 || n > (1<<62)/mul {
		return 0, fmt.Errorf("invalid byte size %q (want a number with B, KiB, MiB, GiB, or TiB)", s)
	}
	return n * mul, nil
}
