package rt

// Benchmarks for the 2026-10-09 spec audit (Appendix B questions 2 and 3,
// decisions 0091 and 0092): what insertion ordered Maps cost against
// unordered and sorted iteration, and what error frames cost on the error
// and success paths. Run with
//
//	go test ./std/rt -run '^$' -bench 'Order|Frames' -benchmem

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"testing"
	"unsafe"
)

// ---------------------------------------------------------------------------
// Map iteration order (Appendix B question 2)

const orderN = 10000

func orderKeys() []string {
	keys := make([]string, orderN)
	for i := range keys {
		// not inserted in sorted order
		keys[i] = fmt.Sprintf("user-%05d", (i*7919)%orderN)
	}
	return keys
}

func orderMap(churn bool) Map[string, int64] {
	var m Map[string, int64]
	keys := orderKeys()
	for i, k := range keys {
		m.M_put(nil, k, int64(i))
	}
	if churn {
		// overwrites and removals leave the log with garbage, so
		// iteration takes the slower path that restores insertion order
		for i := 0; i < orderN; i += 2 {
			m.M_put(nil, keys[i], int64(-i))
		}
		for i := 1; i < orderN; i += 10 {
			m.M_remove(nil, keys[i])
		}
	}
	return m
}

func BenchmarkOrderPutInsertionMap(b *testing.B) {
	keys := orderKeys()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m Map[string, int64]
		for j, k := range keys {
			m.M_put(nil, k, int64(j))
		}
	}
}

func BenchmarkOrderPutGoMap(b *testing.B) {
	keys := orderKeys()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := map[string]int64{}
		for j, k := range keys {
			m[k] = int64(j)
		}
	}
}

func BenchmarkOrderIterInsertion(b *testing.B) {
	for _, churn := range []bool{false, true} {
		m := orderMap(churn)
		b.Run(fmt.Sprintf("churn=%v", churn), func(b *testing.B) {
			var s int64
			for i := 0; i < b.N; i++ {
				for _, v := range m.All() {
					s += v
				}
			}
			_ = s
		})
	}
}

// BenchmarkOrderIterSorted is sorted iteration built on the current map:
// collect the keys, sort them, look each value up.
func BenchmarkOrderIterSorted(b *testing.B) {
	for _, churn := range []bool{false, true} {
		m := orderMap(churn)
		b.Run(fmt.Sprintf("churn=%v", churn), func(b *testing.B) {
			var s int64
			for i := 0; i < b.N; i++ {
				keys := make([]string, 0, m.Len())
				for k := range m.All() {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					s += m.M_get(nil, k).V
				}
			}
			_ = s
		})
	}
}

// BenchmarkOrderIterGoMap is Go's unordered iteration, the floor.
func BenchmarkOrderIterGoMap(b *testing.B) {
	m := map[string]int64{}
	for i, k := range orderKeys() {
		m[k] = int64(i)
	}
	b.ResetTimer()
	var s int64
	for i := 0; i < b.N; i++ {
		for _, v := range m {
			s += v
		}
	}
	_ = s
}

func BenchmarkOrderIterGoMapSorted(b *testing.B) {
	m := map[string]int64{}
	for i, k := range orderKeys() {
		m[k] = int64(i)
	}
	b.ResetTimer()
	var s int64
	for i := 0; i < b.N; i++ {
		for _, k := range slices.Sorted(maps.Keys(m)) {
			s += m[k]
		}
	}
	_ = s
}

// TestOrderEntrySize records the per entry cost of insertion order: the
// org field of a log entry.
func TestOrderEntrySize(t *testing.T) {
	type noOrg struct {
		h    uint64
		prev int32
		k    string
		v    int64
	}
	with := unsafe.Sizeof(mentry[string, int64]{})
	without := unsafe.Sizeof(noOrg{})
	t.Logf("log entry for Map[Str, Int]: %d bytes with org, %d without", with, without)
	if with < without {
		t.Fatal("impossible sizes")
	}
}

// ---------------------------------------------------------------------------
// Error frames (Appendix B question 3)

type frameErr struct {
	code int64
	msg  string
}

const frameDepth = 5

// framesNone propagates an error through frameDepth calls as `return
// Err(e)` does: no frame.
//
//go:noinline
func framesNone(d int, id int64, name string) Result[int64, frameErr] {
	if d == 0 {
		if id >= 0 {
			return Err[int64](frameErr{code: 404, msg: "not found"})
		}
		return Ok[int64, frameErr](id)
	}
	r := framesNone(d-1, id, name)
	if !r.Ok {
		return ErrTr[int64](r.E, r.Tr)
	}
	return Ok[int64, frameErr](r.V + 1)
}

// framesEager is what the Go backend emits for `?` today (core 11.1):
// a frame with the function, location, and the Debug text of every
// parameter, built when the error propagates.
//
//go:noinline
func framesEager(d int, id int64, name string) Result[int64, frameErr] {
	if d == 0 {
		if id >= 0 {
			return Err[int64](frameErr{code: 404, msg: "not found"})
		}
		return Ok[int64, frameErr](id)
	}
	r := framesEager(d-1, id, name)
	if !r.Ok {
		return ErrTr[int64](r.E, r.Tr.Frame(FrameLabel("users.fetch_user", "id", id, "name", name), "users/fetch.xo:18"))
	}
	return Ok[int64, frameErr](r.V + 1)
}

// framesLocOnly records the function and location only (a release build
// without --error-args).
//
//go:noinline
func framesLocOnly(d int, id int64, name string) Result[int64, frameErr] {
	if d == 0 {
		if id >= 0 {
			return Err[int64](frameErr{code: 404, msg: "not found"})
		}
		return Ok[int64, frameErr](id)
	}
	r := framesLocOnly(d-1, id, name)
	if !r.Ok {
		return ErrTr[int64](r.E, r.Tr.Frame("users.fetch_user", "users/fetch.xo:18"))
	}
	return Ok[int64, frameErr](r.V + 1)
}

// lazyTrace keeps the parameter values and formats them only when the
// trace is printed.
type lazyTrace struct {
	prev *lazyTrace
	fn   string
	loc  string
	args []any
}

type lazyResult struct {
	ok bool
	v  int64
	e  frameErr
	tr *lazyTrace
}

//go:noinline
func framesLazy(d int, id int64, name string) lazyResult {
	if d == 0 {
		if id >= 0 {
			return lazyResult{e: frameErr{code: 404, msg: "not found"}}
		}
		return lazyResult{ok: true, v: id}
	}
	r := framesLazy(d-1, id, name)
	if !r.ok {
		return lazyResult{e: r.e, tr: &lazyTrace{prev: r.tr, fn: "users.fetch_user", loc: "users/fetch.xo:18", args: []any{"id", id, "name", name}}}
	}
	return lazyResult{ok: true, v: r.v + 1}
}

// Debug text of a long parameter is truncated to 64 bytes (core 11.1);
// both a short and a long Str parameter are measured.
var frameNames = map[string]string{"short": "ada", "long": "a much longer parameter value such as a request body or a JSON document that is truncated"}

func BenchmarkFramesError(b *testing.B) {
	for _, nm := range []string{"short", "long"} {
		name := frameNames[nm]
		b.Run("none/"+nm, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if framesNone(frameDepth, int64(i&1023), name).Ok {
					b.Fatal("want an error")
				}
			}
		})
		b.Run("eager/"+nm, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if framesEager(frameDepth, int64(i&1023), name).Ok {
					b.Fatal("want an error")
				}
			}
		})
		b.Run("loc_only/"+nm, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if framesLocOnly(frameDepth, int64(i&1023), name).Ok {
					b.Fatal("want an error")
				}
			}
		})
		b.Run("lazy/"+nm, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if framesLazy(frameDepth, int64(i&1023), name).ok {
					b.Fatal("want an error")
				}
			}
		})
	}
}

// BenchmarkFramesSuccess is the success path: the same calls with no
// error, where frames cost only the Tr pointer every Result carries.
func BenchmarkFramesSuccess(b *testing.B) {
	b.Run("eager", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if !framesEager(frameDepth, -1-int64(i&1023), "ada").Ok {
				b.Fatal("want success")
			}
		}
	})
	b.Run("lazy_no_tr_field_in_ok", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if !framesLazy(frameDepth, -1-int64(i&1023), "ada").ok {
				b.Fatal("want success")
			}
		}
	})
}
