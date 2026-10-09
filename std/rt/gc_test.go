package rt

import (
	"math"
	"runtime"
	"runtime/debug"
	"testing"
)

// The percent puts Go's heap goal at the floor while the live heap is
// small, and is the base once the base goal passes the floor.
func TestGCTunerPercent(t *testing.T) {
	const mb = 1 << 20
	tu := &gcTuner{floor: 32 * mb, base: 100}
	goal := func(live, roots uint64, p int64) uint64 {
		g := live + (live+roots)*uint64(p)/100
		return max(g, goHeapMinimum*uint64(p)/100)
	}
	for _, c := range []struct {
		live, roots uint64
		want        int64
	}{
		{0, 0, 800},               // the scaled minimum alone reaches the floor
		{1 * mb, 0, 800},          // 1 + 8 = 9 MB < 32: the minimum decides
		{2 * mb, 0, 800},          // 2 + 16 = 18 MB
		{4 * mb, 0, 700},          // 4 + 28 = 32 MB
		{8 * mb, 1 * mb, 266},     // 8 + 9*2.66 = 32 MB
		{16 * mb, 0, 100},         // 16 + 16: the base goal is the floor
		{64 * mb, 0, 100},         // past the floor
		{31 * mb, 0, 100},         // the base goal is past the floor
		{1 * mb / 2, 4 * mb, 700}, // stacks count: 0.5 + 4.5*7 = 32 MB
	} {
		p := tu.percentFor(c.live, c.roots)
		if p != c.want {
			t.Errorf("live %d roots %d: percent %d, want %d", c.live, c.roots, p, c.want)
		}
		if g := goal(c.live, c.roots, p); c.want > tu.base && g > tu.floor {
			t.Errorf("live %d: goal %d past the floor %d", c.live, g, tu.floor)
		}
	}
	if p := (&gcTuner{floor: 32 * mb, base: 300}).percentFor(10*mb, 0); p != 300 {
		t.Errorf("base 300: percent %d", p)
	}
}

func TestGCMinHeapDefault(t *testing.T) {
	for _, c := range []struct {
		procs int
		want  int64
	}{{1, 16 << 20}, {8, 16 << 20}, {12, 24 << 20}, {64, 128 << 20}} {
		if got := gcMinHeapDefault(c.procs); got != c.want {
			t.Errorf("%d Ps: %d, want %d", c.procs, got, c.want)
		}
	}
}

func TestParseBytes(t *testing.T) {
	for in, want := range map[string]int64{"0": 0, "4096": 4096, "64MiB": 64 << 20, "1GiB": 1 << 30, "500MB": 500e6, "12B": 12, " 3KiB ": 3 << 10} {
		if got, err := parseBytes(in); err != nil || got != want {
			t.Errorf("%q: %d %v, want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "x", "-1", "1.5GiB", "3XB"} {
		if _, err := parseBytes(in); err == nil {
			t.Errorf("%q: no error", in)
		}
	}
}

// gcState saves the GC settings a test changes.
func gcState(t *testing.T) {
	t.Helper()
	runtime.GC() // a fresh live heap for the tuner to read
	conf := gcConf
	pct := debug.SetGCPercent(100)
	lim := debug.SetMemoryLimit(-1)
	t.Cleanup(func() {
		curGCTuner.Store(nil)
		gcConf = conf
		debug.SetGCPercent(pct)
		debug.SetMemoryLimit(lim)
	})
}

// The environment wins over xo.mod and the defaults.
func TestGCSetupPrecedence(t *testing.T) {
	t.Run("default floor", func(t *testing.T) {
		gcState(t)
		t.Setenv("GOGC", "")
		t.Setenv("GOMEMLIMIT", "")
		t.Setenv("XO_GC_MIN_HEAP", "")
		GCConfig(-1, -1, -1)
		gcSetup()
		tu := curGCTuner.Load()
		if tu == nil || tu.floor != uint64(gcMinHeapDefault(runtime.GOMAXPROCS(0))) || tu.base != 100 {
			t.Fatalf("tuner %+v", tu)
		}
		if p := debug.SetGCPercent(100); p <= 100 {
			t.Errorf("percent %d, want above 100 for a small heap", p)
		}
	})
	t.Run("GOGC env", func(t *testing.T) {
		gcState(t)
		t.Setenv("GOGC", "150")
		t.Setenv("GOMEMLIMIT", "")
		GCConfig(300, 64<<20, -1)
		debug.SetGCPercent(150) // what the Go runtime read from GOGC
		gcSetup()
		if curGCTuner.Load() != nil {
			t.Error("tuner running with GOGC set")
		}
		if p := debug.SetGCPercent(100); p != 150 {
			t.Errorf("percent %d, want 150 from GOGC", p)
		}
	})
	t.Run("xo.mod limit and env limit", func(t *testing.T) {
		gcState(t)
		t.Setenv("GOGC", "")
		t.Setenv("GOMEMLIMIT", "")
		GCConfig(-2, -1, 256<<20)
		gcSetup()
		if l := debug.SetMemoryLimit(-1); l != 256<<20 {
			t.Errorf("limit %d, want 256 MiB from xo.mod", l)
		}
		if p := debug.SetGCPercent(-1); p != -1 {
			t.Errorf("percent %d, want off from xo.mod", p)
		}
		debug.SetMemoryLimit(math.MaxInt64)
		t.Setenv("GOMEMLIMIT", "1GiB")
		gcSetup()
		if l := debug.SetMemoryLimit(-1); l != math.MaxInt64 {
			t.Errorf("limit %d, want the environment's (unchanged)", l)
		}
	})
	t.Run("XO_GC_MIN_HEAP over xo.mod", func(t *testing.T) {
		gcState(t)
		t.Setenv("GOGC", "")
		t.Setenv("GOMEMLIMIT", "")
		t.Setenv("XO_GC_MIN_HEAP", "48MiB")
		GCConfig(-1, 64<<20, -1)
		gcSetup()
		if tu := curGCTuner.Load(); tu == nil || tu.floor != 48<<20 {
			t.Fatalf("tuner %+v, want floor 48 MiB", tu)
		}
	})
	t.Run("floor at Go's minimum", func(t *testing.T) {
		gcState(t)
		t.Setenv("GOGC", "")
		t.Setenv("GOMEMLIMIT", "")
		t.Setenv("XO_GC_MIN_HEAP", "")
		GCConfig(200, 0, -1)
		gcSetup()
		if curGCTuner.Load() != nil {
			t.Error("tuner running with min_heap 0")
		}
		if p := debug.SetGCPercent(100); p != 200 {
			t.Errorf("percent %d, want 200", p)
		}
	})
}

// The tuner follows the heap across GC cycles.
func TestGCTunerFollowsCycles(t *testing.T) {
	gcState(t)
	t.Setenv("GOGC", "")
	t.Setenv("GOMEMLIMIT", "")
	t.Setenv("XO_GC_MIN_HEAP", "64MiB")
	GCConfig(-1, -1, -1)
	gcSetup()
	tu := curGCTuner.Load()
	if tu == nil || tu.last.Load() != 64*100/4 {
		t.Fatalf("first percent %v, want 1600", tu)
	}
	// a live heap past the floor brings the percent down to the base
	big := make([]byte, 80<<20)
	for i := 0; i < 6 && tu.last.Load() != 100; i++ {
		runtime.GC()
		runtime.Gosched()
	}
	runtime.KeepAlive(big)
	if p := tu.last.Load(); p != 100 {
		t.Errorf("percent %d with an 80 MiB live heap, want 100", p)
	}
}
