package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"benchtask/cache"
)

type fakeUpstream struct {
	mu      sync.Mutex
	calls   map[string]int
	items   map[string]Item
	failing map[string]bool
	gate    chan struct{}
}

func newFakeUpstream() *fakeUpstream {
	return &fakeUpstream{
		calls:   map[string]int{},
		items:   map[string]Item{"1": {ID: "1", Name: "widget"}},
		failing: map[string]bool{},
	}
}

func (f *fakeUpstream) GetItem(ctx context.Context, id string) (Item, error) {
	f.mu.Lock()
	f.calls[id]++
	gate := f.gate
	item, ok := f.items[id]
	fail := f.failing[id]
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	if fail {
		return Item{}, errors.New("upstream exploded")
	}
	if !ok {
		return Item{}, ErrNotFound
	}
	return item, nil
}

func (f *fakeUpstream) callCount(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[id]
}

func testOpts() cache.Options {
	return cache.Options{
		MaxEntries:  100,
		TTL:         time.Minute,
		StaleTTL:    time.Minute,
		NegativeTTL: time.Minute,
		LoadTimeout: 5 * time.Second,
	}
}

func newServer(t *testing.T, up Upstream) *Server {
	t.Helper()
	s, err := New(up, testOpts(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func do(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestGetItemIsCached(t *testing.T) {
	up := newFakeUpstream()
	s := newServer(t, up)
	for range 2 {
		rec := do(t, s, http.MethodGet, "/items/1")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		var got Item
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got != (Item{ID: "1", Name: "widget"}) {
			t.Fatalf("item = %+v", got)
		}
	}
	if n := up.callCount("1"); n != 1 {
		t.Fatalf("upstream calls = %d, want 1", n)
	}
}

func TestNotFoundIsNegativelyCached(t *testing.T) {
	up := newFakeUpstream()
	s := newServer(t, up)
	for range 3 {
		if rec := do(t, s, http.MethodGet, "/items/missing"); rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	}
	if n := up.callCount("missing"); n != 1 {
		t.Fatalf("upstream calls = %d, want 1", n)
	}
}

func TestUpstreamFailureIs502AndNotCached(t *testing.T) {
	up := newFakeUpstream()
	up.failing["1"] = true
	s := newServer(t, up)
	for range 2 {
		if rec := do(t, s, http.MethodGet, "/items/1"); rec.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502", rec.Code)
		}
	}
	if n := up.callCount("1"); n != 2 {
		t.Fatalf("upstream calls = %d, want 2", n)
	}
	up.mu.Lock()
	up.failing["1"] = false
	up.mu.Unlock()
	if rec := do(t, s, http.MethodGet, "/items/1"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d after recovery, want 200", rec.Code)
	}
}

func TestDeleteInvalidates(t *testing.T) {
	up := newFakeUpstream()
	s := newServer(t, up)
	do(t, s, http.MethodGet, "/items/1")
	if rec := do(t, s, http.MethodDelete, "/items/1/cache"); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", rec.Code)
	}
	if rec := do(t, s, http.MethodGet, "/items/1"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if n := up.callCount("1"); n != 2 {
		t.Fatalf("upstream calls = %d, want 2", n)
	}
}

func TestConcurrentRequestsShareOneUpstreamCall(t *testing.T) {
	up := newFakeUpstream()
	up.gate = make(chan struct{})
	s := newServer(t, up)

	const n = 20
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/items/1", nil))
			codes[i] = rec.Code
		})
	}
	deadline := time.Now().Add(5 * time.Second)
	for s.Cache().Metrics().Misses != n {
		if time.Now().After(deadline) {
			t.Fatal("requests never joined the load")
		}
		time.Sleep(time.Millisecond)
	}
	close(up.gate)
	wg.Wait()
	for i, c := range codes {
		if c != http.StatusOK {
			t.Fatalf("request %d status = %d", i, c)
		}
	}
	if got := up.callCount("1"); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	s := newServer(t, newFakeUpstream())
	if rec := do(t, s, http.MethodPost, "/items/1"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestHTTPUpstream(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/items/1":
			_ = json.NewEncoder(w).Encode(Item{ID: "1", Name: "widget"})
		case "/items/2":
			http.NotFound(w, r)
		default:
			http.Error(w, "boom", http.StatusInternalServerError)
		}
	}))
	defer backend.Close()
	up := &HTTPUpstream{BaseURL: backend.URL, Client: backend.Client()}

	item, err := up.GetItem(context.Background(), "1")
	if err != nil || item.Name != "widget" {
		t.Fatalf("GetItem(1) = %+v, %v", item, err)
	}
	if _, err := up.GetItem(context.Background(), "2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetItem(2) err = %v, want ErrNotFound", err)
	}
	if _, err := up.GetItem(context.Background(), "3"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("GetItem(3) err = %v, want generic error", err)
	}

	// End to end through the service.
	s := newServer(t, up)
	if rec := do(t, s, http.MethodGet, "/items/1"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec := do(t, s, http.MethodGet, "/items/2"); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec := do(t, s, http.MethodGet, "/items/3"); rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d", rec.Code)
	}
}
