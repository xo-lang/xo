package api_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"benchtask/api"
	"benchtask/domain"
	"benchtask/store"
)

// fakeClock advances by step on every call so creation order is
// deterministic without real sleeps. step 0 freezes time.
type fakeClock struct {
	mu   sync.Mutex
	t    time.Time
	step time.Duration
}

func newFakeClock(step time.Duration) *fakeClock {
	return &fakeClock{t: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC), step: step}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(c.step)
	return c.t
}

type harness struct {
	t   *testing.T
	srv *httptest.Server
}

func newHarness(t *testing.T, now func() time.Time) *harness {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api.New(st, now))
	t.Cleanup(srv.Close)
	return &harness{t: t, srv: srv}
}

type result struct {
	status int
	header http.Header
	body   []byte
}

// send performs a request without touching t, so it is safe in goroutines.
func (h *harness) send(method, path string, body any, headers map[string]string) (result, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return result{}, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, h.srv.URL+path, rdr)
	if err != nil {
		return result{}, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := h.srv.Client().Do(req)
	if err != nil {
		return result{}, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return result{}, err
	}
	return result{status: resp.StatusCode, header: resp.Header, body: b}, nil
}

func (h *harness) do(method, path string, body any, headers map[string]string) result {
	h.t.Helper()
	res, err := h.send(method, path, body, headers)
	if err != nil {
		h.t.Fatal(err)
	}
	return res
}

func (h *harness) expect(res result, status int) {
	h.t.Helper()
	if res.status != status {
		h.t.Fatalf("status = %d, want %d; body: %s", res.status, status, res.body)
	}
}

func decode[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
	return v
}

func (h *harness) create(title, body string, tags []string) domain.Note {
	h.t.Helper()
	res := h.do("POST", "/notes", map[string]any{"title": title, "body": body, "tags": tags}, nil)
	h.expect(res, http.StatusCreated)
	return decode[domain.Note](h.t, res.body)
}

type listResp struct {
	Notes      []domain.Note `json:"notes"`
	NextCursor string        `json:"next_cursor"`
}

func TestCreateAndGet(t *testing.T) {
	h := newHarness(t, newFakeClock(time.Second).Now)
	res := h.do("POST", "/notes", map[string]any{"title": "Hello", "body": "world", "tags": []string{"greet"}}, nil)
	h.expect(res, http.StatusCreated)
	if got := res.header.Get("ETag"); got != `"1"` {
		t.Fatalf("ETag = %q, want \"1\"", got)
	}
	n := decode[domain.Note](t, res.body)
	if n.Version != 1 || n.Title != "Hello" || !domain.ValidID(n.ID) || n.Created.IsZero() || !n.Created.Equal(n.Updated) {
		t.Fatalf("unexpected note %+v", n)
	}
	if loc := res.header.Get("Location"); loc != "/notes/"+n.ID {
		t.Fatalf("Location = %q", loc)
	}

	res = h.do("GET", "/notes/"+n.ID, nil, nil)
	h.expect(res, http.StatusOK)
	if got := res.header.Get("ETag"); got != `"1"` {
		t.Fatalf("GET ETag = %q", got)
	}
	got := decode[domain.Note](t, res.body)
	if got.ID != n.ID || got.Body != "world" || len(got.Tags) != 1 || got.Tags[0] != "greet" {
		t.Fatalf("GET returned %+v", got)
	}

	h.expect(h.do("GET", "/notes/0123456789abcdef0123456789abcdef", nil, nil), http.StatusNotFound)
	h.expect(h.do("GET", "/notes/..%2fetc", nil, nil), http.StatusNotFound)
}

func TestCreateWithoutTagsReturnsEmptyArray(t *testing.T) {
	h := newHarness(t, nil)
	res := h.do("POST", "/notes", map[string]any{"title": "x"}, nil)
	h.expect(res, http.StatusCreated)
	if !bytes.Contains(res.body, []byte(`"tags":[]`)) {
		t.Fatalf("expected empty tags array, got %s", res.body)
	}
}

func TestValidationErrors(t *testing.T) {
	h := newHarness(t, nil)
	cases := []any{
		map[string]any{"title": ""},
		map[string]any{"title": strings.Repeat("x", 201)},
		map[string]any{"title": "ok", "tags": []string{"UPPER"}},
		map[string]any{"title": "ok", "tags": []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}},
	}
	for i, body := range cases {
		res := h.do("POST", "/notes", body, nil)
		if res.status != http.StatusBadRequest {
			t.Errorf("case %d: status %d, body %s", i, res.status, res.body)
		}
	}
	req, _ := http.NewRequest("POST", h.srv.URL+"/notes", strings.NewReader("{not json"))
	resp, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed JSON status %d", resp.StatusCode)
	}
}

func TestPreconditions(t *testing.T) {
	h := newHarness(t, newFakeClock(time.Second).Now)
	n := h.create("v1", "", nil)
	path := "/notes/" + n.ID
	upd := map[string]any{"title": "v2", "tags": []string{"x"}}

	h.expect(h.do("PUT", path, upd, nil), http.StatusPreconditionRequired)
	h.expect(h.do("PUT", path, upd, map[string]string{"If-Match": `"7"`}), http.StatusPreconditionFailed)
	h.expect(h.do("PUT", path, upd, map[string]string{"If-Match": `W/"1"`}), http.StatusPreconditionFailed)
	h.expect(h.do("PUT", path, map[string]any{"title": ""}, map[string]string{"If-Match": `"1"`}), http.StatusBadRequest)

	res := h.do("PUT", path, upd, map[string]string{"If-Match": `"1"`})
	h.expect(res, http.StatusOK)
	if got := res.header.Get("ETag"); got != `"2"` {
		t.Fatalf("ETag after update = %q", got)
	}
	updated := decode[domain.Note](t, res.body)
	if updated.Version != 2 || updated.Title != "v2" || !updated.Created.Equal(n.Created) || !updated.Updated.After(n.Updated) {
		t.Fatalf("unexpected updated note %+v (orig %+v)", updated, n)
	}
	// The old version is now stale.
	h.expect(h.do("PUT", path, upd, map[string]string{"If-Match": `"1"`}), http.StatusPreconditionFailed)

	h.expect(h.do("DELETE", path, nil, nil), http.StatusPreconditionRequired)
	h.expect(h.do("DELETE", path, nil, map[string]string{"If-Match": `"1"`}), http.StatusPreconditionFailed)
	h.expect(h.do("DELETE", path, nil, map[string]string{"If-Match": `"2"`}), http.StatusNoContent)
	h.expect(h.do("GET", path, nil, nil), http.StatusNotFound)
	h.expect(h.do("PUT", path, upd, map[string]string{"If-Match": `"2"`}), http.StatusNotFound)
}

func TestConcurrentPutsExactlyOneWins(t *testing.T) {
	h := newHarness(t, nil)
	for round := range 20 {
		n := h.create(fmt.Sprintf("round %d", round), "", nil)
		path := "/notes/" + n.ID
		start := make(chan struct{})
		var wg sync.WaitGroup
		statuses := make([]int, 2)
		errs := make([]error, 2)
		titles := []string{"writer A", "writer B"}
		for i := range 2 {
			wg.Go(func() {
				<-start
				res, err := h.send("PUT", path, map[string]any{"title": titles[i]}, map[string]string{"If-Match": `"1"`})
				statuses[i], errs[i] = res.status, err
			})
		}
		close(start)
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		winner := -1
		for i, s := range statuses {
			switch s {
			case http.StatusOK:
				if winner != -1 {
					t.Fatalf("round %d: both PUTs succeeded", round)
				}
				winner = i
			case http.StatusPreconditionFailed:
			default:
				t.Fatalf("round %d: unexpected status %d", round, s)
			}
		}
		if winner == -1 {
			t.Fatalf("round %d: no PUT succeeded: %v", round, statuses)
		}
		got := decode[domain.Note](t, h.do("GET", path, nil, nil).body)
		if got.Version != 2 || got.Title != titles[winner] {
			t.Fatalf("round %d: final note %+v, winner %q", round, got, titles[winner])
		}
	}
}

func TestPaginationStableUnderInserts(t *testing.T) {
	cases := []struct {
		name string
		step time.Duration
	}{
		{"distinct timestamps", time.Second},
		{"identical timestamps", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, newFakeClock(tc.step).Now)
			original := map[string]bool{}
			for i := range 25 {
				original[h.create(fmt.Sprintf("note %02d", i), "", nil).ID] = true
			}
			seen := map[string]int{}
			var all []domain.Note
			var inserted string
			cursor := ""
			pages := 0
			for {
				path := "/notes?limit=10"
				if cursor != "" {
					path += "&cursor=" + url.QueryEscape(cursor)
				}
				res := h.do("GET", path, nil, nil)
				h.expect(res, http.StatusOK)
				lr := decode[listResp](t, res.body)
				pages++
				if len(lr.Notes) > 10 {
					t.Fatalf("page %d has %d notes", pages, len(lr.Notes))
				}
				for _, n := range lr.Notes {
					seen[n.ID]++
					all = append(all, n)
				}
				if pages == 1 {
					inserted = h.create("inserted mid way", "", nil).ID
				}
				if lr.NextCursor == "" {
					break
				}
				cursor = lr.NextCursor
				if pages > 10 {
					t.Fatal("pagination did not terminate")
				}
			}
			for id, c := range seen {
				if c > 1 {
					t.Errorf("note %s returned %d times", id, c)
				}
			}
			for id := range original {
				if seen[id] != 1 {
					t.Errorf("original note %s seen %d times (gap)", id, seen[id])
				}
			}
			if pages != 3 {
				t.Errorf("pages = %d, want 3", pages)
			}
			for i := 1; i < len(all); i++ {
				a, b := all[i-1], all[i]
				if a.Created.Before(b.Created) || (a.Created.Equal(b.Created) && a.ID < b.ID) {
					t.Errorf("not newest first at %d", i)
				}
			}
			if tc.step > 0 {
				if seen[inserted] != 0 {
					t.Errorf("note created after page 1 should not appear in later pages")
				}
				if len(all) != 25 {
					t.Errorf("got %d notes, want 25", len(all))
				}
				// A fresh listing sees the inserted note first.
				lr := decode[listResp](t, h.do("GET", "/notes?limit=1", nil, nil).body)
				if len(lr.Notes) != 1 || lr.Notes[0].ID != inserted {
					t.Errorf("fresh first page does not start with inserted note")
				}
			}
		})
	}
}

func TestListFiltersAndValidation(t *testing.T) {
	h := newHarness(t, newFakeClock(time.Second).Now)
	h.create("Shopping", "buy MILK", []string{"home"})
	h.create("Standup", "discuss milk budget", []string{"work"})
	h.create("Taxes", "file them", []string{"home", "money"})

	count := func(query string) int {
		t.Helper()
		res := h.do("GET", "/notes?"+query, nil, nil)
		h.expect(res, http.StatusOK)
		return len(decode[listResp](t, res.body).Notes)
	}
	if n := count(""); n != 3 {
		t.Errorf("all: %d", n)
	}
	if n := count("tag=home"); n != 2 {
		t.Errorf("tag=home: %d", n)
	}
	if n := count("q=milk"); n != 2 {
		t.Errorf("q=milk: %d", n)
	}
	if n := count("q=SHOP"); n != 1 {
		t.Errorf("q=SHOP: %d", n)
	}
	if n := count("tag=home&q=milk"); n != 1 {
		t.Errorf("tag=home&q=milk: %d", n)
	}
	if n := count("tag=none"); n != 0 {
		t.Errorf("tag=none: %d", n)
	}
	res := h.do("GET", "/notes?tag=none", nil, nil)
	if !bytes.Contains(res.body, []byte(`"notes":[]`)) {
		t.Errorf("empty list should be an array: %s", res.body)
	}
	for _, q := range []string{"limit=0", "limit=101", "limit=abc", "cursor=%21%21", "cursor=abc"} {
		h.expect(h.do("GET", "/notes?"+q, nil, nil), http.StatusBadRequest)
	}
}

func TestExportStreamsJSONLines(t *testing.T) {
	h := newHarness(t, newFakeClock(time.Second).Now)
	want := map[string]bool{}
	for i := range 3 {
		want[h.create(fmt.Sprintf("n%d", i), "line\nbreak", nil).ID] = true
	}
	resp, err := h.srv.Client().Get(h.srv.URL + "/export")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/x-ndjson" {
		t.Fatalf("Content-Type %q", ct)
	}
	sc := bufio.NewScanner(resp.Body)
	lines := 0
	for sc.Scan() {
		lines++
		n := decode[domain.Note](t, sc.Bytes())
		if !want[n.ID] {
			t.Fatalf("unexpected note %s", n.ID)
		}
		delete(want, n.ID)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if lines != 3 || len(want) != 0 {
		t.Fatalf("lines=%d missing=%v", lines, want)
	}
}
