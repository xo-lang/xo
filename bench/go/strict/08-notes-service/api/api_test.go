package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"benchtask/domain"
	"benchtask/store"
)

// fakeClock advances one second on every reading, so creation order is
// strictly reflected in timestamps without any real sleeping.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(time.Second)
	return c.t
}

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	clock := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	return New(st, Options{Now: clock.Now, Logger: slog.New(slog.DiscardHandler)}).Handler()
}

// do performs a request. It never calls t.Fatal so it is safe in goroutines.
func do(t *testing.T, h http.Handler, method, target, body string, header ...string) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rd)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func createNote(t *testing.T, h http.Handler, title, body string, tags ...string) domain.Note {
	t.Helper()
	rec := do(t, h, http.MethodPost, "/notes", mustJSON(t, map[string]any{"title": title, "body": body, "tags": tags}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status %d body %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("ETag"); got != `"1"` {
		t.Fatalf("create: ETag %q, want \"1\"", got)
	}
	n := decode[domain.Note](t, rec)
	if n.Version != 1 || n.ID == "" {
		t.Fatalf("create: unexpected note %+v", n)
	}
	if loc := rec.Header().Get("Location"); loc != "/notes/"+n.ID {
		t.Fatalf("create: Location %q", loc)
	}
	return n
}

func TestCreateAndGet(t *testing.T) {
	h := newTestHandler(t)
	n := createNote(t, h, "Hello", "world", "greeting")
	rec := do(t, h, http.MethodGet, "/notes/"+n.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d", rec.Code)
	}
	if got := rec.Header().Get("ETag"); got != `"1"` {
		t.Fatalf("get: ETag %q", got)
	}
	got := decode[domain.Note](t, rec)
	if got.Title != "Hello" || got.Body != "world" || !slices.Equal(got.Tags, []string{"greeting"}) {
		t.Fatalf("get: %+v", got)
	}
	if !domain.ValidID(got.ID) {
		t.Fatalf("generated id %q is not a safe file name", got.ID)
	}

	if rec := do(t, h, http.MethodGet, "/notes/doesnotexist", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing note: %d", rec.Code)
	}
	for _, bad := range []string{"CON", "a.b", "nul"} {
		if rec := do(t, h, http.MethodGet, "/notes/"+bad, ""); rec.Code != http.StatusNotFound {
			t.Fatalf("unsafe id %q: %d", bad, rec.Code)
		}
	}
}

func TestCreateValidation(t *testing.T) {
	h := newTestHandler(t)
	bodies := []string{
		`{"title":""}`,
		mustJSON(t, map[string]any{"title": strings.Repeat("x", 201)}),
		`{"title":"t","tags":["UPPER"]}`,
		mustJSON(t, map[string]any{"title": "t", "tags": []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}}),
		`not json`,
		`{"title":"a"} {"title":"b"}`,
	}
	for _, b := range bodies {
		if rec := do(t, h, http.MethodPost, "/notes", b); rec.Code != http.StatusBadRequest {
			t.Errorf("body %.40q: status %d, want 400", b, rec.Code)
		}
	}
	big := `{"title":"t","body":"` + strings.Repeat("x", maxBodyBytes) + `"}`
	if rec := do(t, h, http.MethodPost, "/notes", big); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body: status %d, want 413", rec.Code)
	}
	// Tags omitted serialize as an empty list, not null.
	n := createNote(t, h, "no tags", "")
	if n.Tags == nil {
		t.Fatal("tags should be an empty list")
	}
}

func TestUpdatePreconditions(t *testing.T) {
	h := newTestHandler(t)
	n := createNote(t, h, "v1", "")
	path := "/notes/" + n.ID
	body := `{"title":"v2","body":"b","tags":["x"]}`

	if rec := do(t, h, http.MethodPut, path, body); rec.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing If-Match: %d, want 428", rec.Code)
	}
	for _, bad := range []string{"*", `W/"1"`, `"1", "2"`, `"abc"`, `"0"`} {
		if rec := do(t, h, http.MethodPut, path, body, "If-Match", bad); rec.Code != http.StatusBadRequest {
			t.Fatalf("If-Match %q: %d, want 400", bad, rec.Code)
		}
	}
	if rec := do(t, h, http.MethodPut, path, body, "If-Match", `"7"`); rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale If-Match: %d, want 412", rec.Code)
	}
	rec := do(t, h, http.MethodPut, path, body, "If-Match", `"1"`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("ETag"); got != `"2"` {
		t.Fatalf("update ETag %q", got)
	}
	up := decode[domain.Note](t, rec)
	if up.Version != 2 || up.Title != "v2" || !up.Created.Equal(n.Created) || !up.Updated.After(n.Updated) {
		t.Fatalf("update result %+v (orig %+v)", up, n)
	}
	if rec := do(t, h, http.MethodPut, path, body, "If-Match", `"1"`); rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("reused old version: %d, want 412", rec.Code)
	}
	if rec := do(t, h, http.MethodPut, path, `{"title":""}`, "If-Match", `"2"`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid update body: %d, want 400", rec.Code)
	}
	if rec := do(t, h, http.MethodPut, "/notes/nosuchnote", body, "If-Match", `"1"`); rec.Code != http.StatusNotFound {
		t.Fatalf("update missing: %d, want 404", rec.Code)
	}
}

func TestDeleteRequiresIfMatch(t *testing.T) {
	h := newTestHandler(t)
	n := createNote(t, h, "doomed", "")
	path := "/notes/" + n.ID
	if rec := do(t, h, http.MethodDelete, path, ""); rec.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing If-Match: %d, want 428", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, path, "", "If-Match", `"2"`); rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale delete: %d, want 412", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, path, "", "If-Match", `"1"`); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d, want 204", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, path, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, path, "", "If-Match", `"1"`); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete: %d, want 404", rec.Code)
	}
}

func TestConcurrentPutsExactlyOneSucceeds(t *testing.T) {
	h := newTestHandler(t)
	n := createNote(t, h, "race", "")
	const rounds = 20
	for round := range rounds {
		version := int64(round + 1)
		const writers = 2
		codes := make([]int, writers)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range writers {
			wg.Go(func() {
				<-start
				body := fmt.Sprintf(`{"title":"writer %d round %d"}`, i, round)
				codes[i] = do(t, h, http.MethodPut, "/notes/"+n.ID, body, "If-Match", etag(version)).Code
			})
		}
		close(start)
		wg.Wait()
		ok, failed := 0, 0
		for _, c := range codes {
			switch c {
			case http.StatusOK:
				ok++
			case http.StatusPreconditionFailed:
				failed++
			default:
				t.Fatalf("round %d: unexpected status %d", round, c)
			}
		}
		if ok != 1 || failed != writers-1 {
			t.Fatalf("round %d: %d succeeded and %d got 412, want exactly one success", round, ok, failed)
		}
	}
	got := decode[domain.Note](t, do(t, h, http.MethodGet, "/notes/"+n.ID, ""))
	if got.Version != rounds+1 {
		t.Fatalf("final version %d, want %d", got.Version, rounds+1)
	}
}

func listPage(t *testing.T, h http.Handler, query url.Values) listResponse {
	t.Helper()
	rec := do(t, h, http.MethodGet, "/notes?"+query.Encode(), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list %s: %d %s", query.Encode(), rec.Code, rec.Body.String())
	}
	return decode[listResponse](t, rec)
}

func TestPaginationWithCreateMidway(t *testing.T) {
	h := newTestHandler(t)
	created := make([]string, 0, 25)
	for i := range 25 {
		created = append(created, createNote(t, h, fmt.Sprintf("note %02d", i), "").ID)
	}

	var got []string
	var sizes []int
	var inserted string
	cursor := ""
	for page := 1; ; page++ {
		if page > 10 {
			t.Fatal("pagination did not terminate")
		}
		q := url.Values{"limit": {"10"}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		resp := listPage(t, h, q)
		sizes = append(sizes, len(resp.Notes))
		for _, n := range resp.Notes {
			got = append(got, n.ID)
		}
		if page == 1 {
			inserted = createNote(t, h, "created mid way", "").ID
		}
		if resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}

	if !slices.Equal(sizes, []int{10, 10, 5}) {
		t.Fatalf("page sizes %v, want [10 10 5]", sizes)
	}
	want := slices.Clone(created)
	slices.Reverse(want)
	if !slices.Equal(got, want) {
		t.Fatalf("pages are not the 25 notes newest first with no gaps or duplicates:\n got %v\nwant %v", got, want)
	}
	if slices.Contains(got, inserted) {
		t.Fatal("note created mid way sorts before the cursor and must not appear on later pages")
	}
	first := listPage(t, h, url.Values{"limit": {"1"}})
	if len(first.Notes) != 1 || first.Notes[0].ID != inserted {
		t.Fatalf("a fresh listing should start with the new note, got %+v", first.Notes)
	}
}

func TestListFilters(t *testing.T) {
	h := newTestHandler(t)
	a := createNote(t, h, "Groceries", "buy MILK", "home")
	b := createNote(t, h, "Standup", "discuss milk budget", "work")
	createNote(t, h, "Other", "nothing", "home")

	ids := func(r listResponse) []string {
		out := []string{}
		for _, n := range r.Notes {
			out = append(out, n.ID)
		}
		return out
	}
	if got := ids(listPage(t, h, url.Values{"q": {"Milk"}})); !slices.Equal(got, []string{b.ID, a.ID}) {
		t.Fatalf("q=Milk: %v", got)
	}
	if got := ids(listPage(t, h, url.Values{"q": {"milk"}, "tag": {"home"}})); !slices.Equal(got, []string{a.ID}) {
		t.Fatalf("q=milk tag=home: %v", got)
	}
	if got := ids(listPage(t, h, url.Values{"q": {"groceries"}})); !slices.Equal(got, []string{a.ID}) {
		t.Fatalf("title match: %v", got)
	}
	empty := listPage(t, h, url.Values{"tag": {"none"}})
	if empty.Notes == nil || len(empty.Notes) != 0 || empty.NextCursor != "" {
		t.Fatalf("empty result: %+v", empty)
	}
	all := listPage(t, h, url.Values{})
	if len(all.Notes) != 3 {
		t.Fatalf("default list: %d notes", len(all.Notes))
	}
}

func TestListBadParams(t *testing.T) {
	h := newTestHandler(t)
	for _, q := range []string{"limit=0", "limit=101", "limit=abc", "cursor=%21%21", "cursor=bm9kb3Q"} {
		if rec := do(t, h, http.MethodGet, "/notes?"+q, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", q, rec.Code)
		}
	}
}

func TestExportStreamsJSONLines(t *testing.T) {
	h := newTestHandler(t)
	want := map[string]bool{}
	for i := range 3 {
		want[createNote(t, h, fmt.Sprintf("n%d", i), "line\nbreak").ID] = true
	}
	rec := do(t, h, http.MethodGet, "/export", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("export: %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/x-ndjson" {
		t.Fatalf("content type %q", ct)
	}
	if !rec.Flushed {
		t.Fatal("export should flush as it streams")
	}
	sc := bufio.NewScanner(strings.NewReader(rec.Body.String()))
	got := map[string]bool{}
	for sc.Scan() {
		var n domain.Note
		if err := json.Unmarshal(sc.Bytes(), &n); err != nil {
			t.Fatalf("line %q: %v", sc.Text(), err)
		}
		got[n.ID] = true
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("exported %d notes, want %d", len(got), len(want))
	}
	for id := range want {
		if !got[id] {
			t.Fatalf("missing %s from export", id)
		}
	}
}
