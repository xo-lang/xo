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
		m.M_put(nil, "Content-Type", "application/json")
		m.M_put(nil, "Accept", "*/*")
		m.M_put(nil, "User-Agent", "x")
		if m.M_get(nil, "Accept").V != "*/*" {
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

// Ranging over m.All() allocates nothing.
func TestAllocsMapAll(t *testing.T) {
	var m Map[string, string]
	m.M_put(nil, "a", "1")
	m.M_put(nil, "b", "2")
	m.M_put(nil, "a", "3") // an overwrite: the eachRoot path
	var mm Map[string, string]
	mm.M_put(nil, "a", "1")
	for _, x := range []Map[string, string]{m, mm} {
		n := testing.AllocsPerRun(100, func() {
			c := 0
			for k, v := range x.All() {
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
}
