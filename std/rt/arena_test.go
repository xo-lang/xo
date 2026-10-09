package rt

import (
	"strings"
	"testing"
	"unsafe"
)

func overlaps[T any](a, b []T) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	var z T
	sz := unsafe.Sizeof(z)
	a0, b0 := uintptr(unsafe.Pointer(&a[0])), uintptr(unsafe.Pointer(&b[0]))
	return a0 < b0+uintptr(len(b))*sz && b0 < a0+uintptr(len(a))*sz
}

// TestArenaSliceNoReuseBeforeReset: within one request the bump allocator
// never hands out memory twice, also across slab growth.
func TestArenaSliceNoReuseBeforeReset(t *testing.T) {
	a := &Arena{slabs: map[any]arenaSlab{}}
	var got [][]int64
	for i := 0; i < 200; i++ {
		s := arenaSlice[int64](a, i%50+1)
		for j := range s {
			s[j] = int64(i)
		}
		got = append(got, s)
	}
	for i, s := range got {
		for _, v := range s {
			if v != int64(i) {
				t.Fatalf("slice %d was overwritten: %v", i, s)
			}
		}
		for j := i + 1; j < len(got); j++ {
			if overlaps(s, got[j]) {
				t.Fatalf("slices %d and %d overlap", i, j)
			}
		}
	}
	if arenaSlice[int64](a, arenaKeep+1) != nil {
		t.Fatal("an oversized request must fall back to the heap")
	}
}

// TestArenaReuseAfterRelease: after release the same tables and buffers
// serve the next request, emptied (no stale keys, values cleared).
func TestArenaReuseAfterRelease(t *testing.T) {
	for len(arenaFree) > 0 {
		<-arenaFree
	}
	a := getArena()
	m := a.strMap()
	for i := 0; i < 20; i++ {
		Map_put(nil, &m, strings.Repeat("k", i+1), "v")
	}
	tab := m.t
	vals := Map_values_arena(&Task{arena: a}, m)
	if List_len(nil, vals) != 20 {
		t.Fatalf("values len %d", List_len(nil, vals))
	}
	a.release()
	b := getArena()
	if b != a {
		t.Fatal("released arena was not reused")
	}
	m2 := b.strMap()
	if m2.t != tab {
		t.Fatal("table was not reused")
	}
	if Map_len(nil, m2) != 0 || Map_get(nil, m2, "k").Some {
		t.Fatal("reused table is not empty")
	}
	Map_put(nil, &m2, "a", "1")
	Map_put(nil, &m2, "b", "2")
	if got := Map_get(nil, m2, "b"); !got.Some || got.V != "2" {
		t.Fatalf("get b = %v", got)
	}
	if arr := m2.t.arr.Load(); arr != nil {
		for _, e := range (*arr)[2:] {
			if e.k != "" || e.v != "" {
				t.Fatal("stale entries kept alive after reset")
			}
		}
	}
	b.release()
}

// TestArenaHeapCopy: a request copied out of the arena shares no memory
// with it, so resetting the arena cannot change it.
func TestArenaHeapCopy(t *testing.T) {
	a := &Arena{slabs: map[any]arenaSlab{}}
	h := a.strMap()
	Map_put(nil, &h, "X-A", "1")
	body := a.readBody(strings.NewReader("hello body"))
	req := Http_Request{F_headers: h, F_body: body}
	req.XParams = a.strMap()
	Map_put(nil, &req.XParams, "id", "7")
	cp := req.heapCopy()
	if cp.F_headers.t == h.t || cp.XParams.t == req.XParams.t {
		t.Fatal("heap copy shares a table with the arena")
	}
	if unsafe.StringData(string(cp.F_body)) == unsafe.StringData(string(body)) {
		t.Fatal("heap copy shares the body with the arena")
	}
	a.release()
	a.readBody(strings.NewReader("XXXXXXXXXX"))
	if cp.F_body != "hello body" || Map_get(nil, cp.F_headers, "X-A").V != "1" || Map_get(nil, cp.XParams, "id").V != "7" {
		t.Fatalf("heap copy changed after the arena was reused: %+v", cp)
	}
}

// TestArenaResponseOnlyInServerDispatch: arena responses are built only
// while the server's own dispatch runs (Arena.resp), at most once per
// request; otherwise the heap forms are used.
func TestArenaResponseOnlyInServerDispatch(t *testing.T) {
	a := &Arena{slabs: map[any]arenaSlab{}}
	tk := &Task{arena: a}
	r := Http_json_arena(tk, 200, "x")
	if a.respUsed || r.F_body != `"x"` {
		t.Fatal("arena response built outside server dispatch")
	}
	a.resp = true
	r1 := Http_json_arena(tk, 200, "first")
	r2 := Http_json_arena(tk, 200, "second")
	if r1.F_body != `"first"` || r2.F_body != `"second"` {
		t.Fatalf("bodies %q %q", r1.F_body, r2.F_body)
	}
	if r1.F_headers.t == r2.F_headers.t {
		t.Fatal("second response reused the first one's arena table")
	}
	var router Http_Router
	Http_Router_handle[Never](nil, &router, "GET /x", func(tk *Task, _ Http_Request) Result[Http_Response, Never] {
		if tk.arena.resp {
			t.Error("Arena.resp set during in process dispatch")
		}
		return Ok[Http_Response, Never](Http_json_arena(tk, 200, 1))
	})
	b := &Arena{slabs: map[any]arenaSlab{}, resp: true}
	router.M_call(&Task{node: newNode(nil, nil), arena: b}, Http_new_request(nil, "GET", "/x", Map[string, string]{}, ""))
	if !b.resp || b.respUsed {
		t.Fatal("in process dispatch used or did not restore Arena.resp")
	}
}
