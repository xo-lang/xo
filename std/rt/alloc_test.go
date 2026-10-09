package rt

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// Allocation budgets of the per request paths of an HTTP server
// (bench/perf/http/allocs.sh measures the whole request). Each test pins
// the count so a change that adds an allocation to a hot path fails here.

var sinkStrMap Map[string, string]

// A small map is its table and its log array: the first index and the
// first array header live in the table.
func TestAllocsSmallMap(t *testing.T) {
	n := testing.AllocsPerRun(100, func() {
		var m Map[string, string]
		Map_put(nil, &m, "Content-Type", "application/json")
		Map_put(nil, &m, "Accept", "*/*")
		Map_put(nil, &m, "User-Agent", "x")
		if Map_get(nil, m, "Accept").V != "*/*" {
			t.Fatal("lookup")
		}
		sinkStrMap = m
	})
	if n > 2 {
		t.Errorf("small Map[Str, Str]: %v allocations, want at most 2", n)
	}
}

// Dispatch splits the path once and builds the parameter table of the
// chosen route only: routes whose path matches but whose method does not
// cost nothing.
func TestAllocsDispatch(t *testing.T) {
	var r Http_Router
	h := func(tk *Task, req Http_Request) Result[Http_Response, Never] {
		return Ok[Http_Response, Never](Http_empty(tk, req.M_param_int(tk, "id")))
	}
	for _, p := range []string{"POST /users", "GET /users", "GET /users/{id:Int}", "PATCH /users/{id:Int}", "DELETE /users/{id:Int}"} {
		Http_Router_handle[Never](nil, &r, p, h)
	}
	tk := &Task{node: newNode(nil, nil)}
	for _, m := range []string{"GET", "PATCH", "DELETE"} {
		req := Http_new_request(nil, m, "/users/204", Map[string, string]{}, "")
		n := testing.AllocsPerRun(100, func() {
			if resp := r.dispatch(tk, req); resp.F_status != 204 {
				t.Fatal("status", resp.F_status)
			}
		})
		if n > 2 {
			t.Errorf("%s /users/204: %v allocations, want at most 2 (the parameter table)", m, n)
		}
	}
	// precedence and errors are unchanged
	for _, c := range []struct {
		method, path string
		status       int64
	}{{"PUT", "/users/1", 405}, {"GET", "/nope", 404}, {"GET", "/users/x", 400}, {"GET", "/users/7/", 7}, {"get", "/users/9", 9}} {
		if got := r.dispatch(tk, Http_new_request(nil, c.method, c.path, Map[string, string]{}, "")).F_status; got != c.status {
			t.Errorf("%s %s: status %d, want %d", c.method, c.path, got, c.status)
		}
	}
}

// The server reads a body with a declared length into one exact buffer,
// and an empty body not at all.
func TestReadBody(t *testing.T) {
	for _, c := range []struct {
		body    string
		length  int64
		allocs  float64
		chunked bool
	}{{"", 0, 0, false}, {`{"a": 1}`, 8, 1, false}, {`{"a": 1}`, -1, 8, true}} {
		n := testing.AllocsPerRun(20, func() {
			hr := &http.Request{ContentLength: c.length, Body: io.NopCloser(strings.NewReader(c.body))}
			if c.length == 0 {
				hr.Body = http.NoBody
			}
			if got := readBody(hr); string(got) != c.body {
				t.Fatalf("body %q, want %q", got, c.body)
			}
		})
		if !c.chunked && n > c.allocs+2 { // +2: the request and reader of the test itself
			t.Errorf("length %d: %v allocations", c.length, n)
		}
	}
}

type allocState struct {
	users Map[int64, string]
	next  int64
}

// Mutex.with_read allocates nothing for a value without lists, once the
// task has its held set (a request task's comes with its reqBlock), and
// Own copies such a value without moving it to the heap.
func TestAllocsMutexRead(t *testing.T) {
	var users Map[int64, string]
	Map_put(nil, &users, 1, "ada")
	m := Mutex_new(nil, allocState{users: users, next: 2})
	tk := &Task{node: newNode(nil, nil), th: newRootThread("test"), held: new(heldSet)}
	n := testing.AllocsPerRun(100, func() {
		r := Mutex_with_read(tk, m, func(_ *Task, st allocState) Result[Option[string], Never] {
			return Ok[Option[string], Never](Map_get(nil, st.users, 1))
		})
		if r.V.V != "ada" {
			t.Fatal("read")
		}
	})
	if n != 0 {
		t.Errorf("Mutex.with_read: %v allocations, want 0", n)
	}
	if _, ok := tk.held.get(m); ok {
		t.Error("mutex still in the held set")
	}
	n = testing.AllocsPerRun(100, func() {
		if Own(allocState{next: 3}).next != 3 {
			t.Fatal("own")
		}
	})
	if n != 0 {
		t.Errorf("Own of a value without lists: %v allocations, want 0", n)
	}
}

// The held set keeps one mutex inline and the rest in a map.
func TestHeldSet(t *testing.T) {
	var h heldSet
	ms := []any{new(int), new(int), new(int), new(int)}
	for i, m := range ms {
		h.put(m, heldSite{n: i})
	}
	for i, m := range ms {
		if s, ok := h.get(m); !ok || s.n != i {
			t.Fatalf("get %d: %v %v", i, s.n, ok)
		}
	}
	h.del(ms[0])
	h.del(ms[3])
	if _, ok := h.get(ms[0]); ok {
		t.Fatal("deleted inline entry found")
	}
	if _, ok := h.get(ms[3]); ok {
		t.Fatal("deleted map entry found")
	}
	if s, ok := h.get(ms[2]); !ok || s.n != 2 {
		t.Fatal("map entry lost")
	}
	var nilSet *heldSet
	if _, ok := nilSet.get(ms[1]); ok {
		t.Fatal("nil set holds a mutex")
	}
	nilSet.del(ms[1])
}

// allocRec stands for a record with `derive Json`, encoded the way
// internal/gen emits it (direct calls for Option fields).
type allocRec struct {
	id   int64
	name string
	nick Option[string]
}

func (v allocRec) XoJsonEnc(tk *Task, w *JsonW) {
	w.Raw(`{"id":`)
	w.Int(v.id)
	w.Raw(`,"name":`)
	w.Str(v.name)
	w.Raw(`,"nick":`)
	v.nick.XoJsonEnc(tk, w)
	w.Raw("}")
}

// json.encode copies the text out of a pooled buffer once; list elements
// and Str, Int, and Bool values are encoded without copies to the heap.
func TestAllocsJsonEncode(t *testing.T) {
	recs := make([]allocRec, 64)
	for i := range recs {
		recs[i] = allocRec{id: int64(i), name: "user", nick: Some("n")}
	}
	l := ListOf(recs...)
	Json_encode(nil, l) // fill the pool
	for _, c := range []struct {
		name string
		max  float64
		f    func() Bytes
	}{
		{"Str", 1, func() Bytes { return Json_encode(nil, "x") }},
		{"Int", 1, func() Bytes { return Json_encode(nil, int64(7)) }},
		{"record", 2, func() Bytes { return Json_encode(nil, recs[0]) }},
		{"List of 64 records", 2, func() Bytes { return Json_encode(nil, l) }},
	} {
		n := testing.AllocsPerRun(50, func() { c.f() })
		if n > c.max && !raceOn {
			t.Errorf("json.encode(%s): %v allocations, want at most %v", c.name, n, c.max)
		}
	}
	got := string(Json_encode(nil, ListOf(recs[:2]...)))
	want := `[{"id":0,"name":"user","nick":"n"},{"id":1,"name":"user","nick":"n"}]`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if got := string(Json_encode(nil, ListOf(Some(true), None[bool]()))); got != "[true,null]" {
		t.Errorf("got %s", got)
	}
}

// Ranging over m.All() allocates nothing.
func TestAllocsMapAll(t *testing.T) {
	var m Map[string, string]
	Map_put(nil, &m, "a", "1")
	Map_put(nil, &m, "b", "2")
	Map_put(nil, &m, "a", "3") // an overwrite: the eachRoot path
	var mm Map[string, string]
	Map_put(nil, &mm, "a", "1")
	for _, x := range []Map[string, string]{m, mm} {
		n := testing.AllocsPerRun(100, func() {
			c := 0
			for k, v := range Map_All(x) {
				c += len(k) + len(v)
			}
			if c != 4 && c != 2 {
				t.Fatal("count", c)
			}
		})
		if n != 0 {
			t.Errorf("range over Map.All: %v allocations, want 0", n)
		}
	}
	// m.values() in a loop (`m.values().any(f)`), as generated code ranges
	var users Map[int64, allocRec]
	for i := range int64(8) {
		Map_put(nil, &users, i, allocRec{id: i, name: "u"})
	}
	n := testing.AllocsPerRun(100, func() {
		found := false
		for u := range Map_values_seq(users) {
			if u.id == 7 {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("not found")
		}
	})
	if n != 0 {
		t.Errorf("range over Map_values_seq: %v allocations, want 0", n)
	}
}
