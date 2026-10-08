package benchtask

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newFakeUpstream serves GET /items/{id}: "missing" is 404, "broken" is 500,
// anything else is an item. It stands in for the ~200ms upstream without
// sleeping.
func newFakeUpstream(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		id := r.PathValue("id")
		switch id {
		case "missing":
			http.NotFound(w, r)
		case "broken":
			http.Error(w, "boom", http.StatusInternalServerError)
		default:
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(Item{ID: id, Name: "item " + id}); err != nil {
				t.Errorf("encode: %v", err)
			}
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &hits
}

func newFrontend(t *testing.T, up Upstream, clock *fakeClock) *httptest.Server {
	t.Helper()
	s, err := NewServer(up, Options{
		MaxEntries: 100, TTL: time.Minute, StaleTTL: time.Minute,
		NegativeTTL: 30 * time.Second, LoadTimeout: 5 * time.Second, Now: clock.Now,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv
}

func doRequest(t *testing.T, method, url string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	if resp == nil {
		t.Fatalf("%s %s: nil response", method, url)
	}
	body, err := io.ReadAll(resp.Body)
	if cerr := resp.Body.Close(); cerr != nil {
		t.Fatalf("close body: %v", cerr)
	}
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, body
}

func TestServiceGetCachesItems(t *testing.T) {
	up, hits := newFakeUpstream(t)
	front := newFrontend(t, &HTTPUpstream{BaseURL: up.URL, Client: up.Client()}, newFakeClock())

	for range 3 {
		status, body := doRequest(t, http.MethodGet, front.URL+"/items/1")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", status, body)
		}
		var item Item
		if err := json.Unmarshal(body, &item); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if item != (Item{ID: "1", Name: "item 1"}) {
			t.Fatalf("item = %+v", item)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("upstream hits = %d, want 1", got)
	}
}

func TestServiceNotFoundIsNegativelyCached(t *testing.T) {
	up, hits := newFakeUpstream(t)
	front := newFrontend(t, &HTTPUpstream{BaseURL: up.URL, Client: up.Client()}, newFakeClock())
	for range 2 {
		if status, _ := doRequest(t, http.MethodGet, front.URL+"/items/missing"); status != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", status)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("upstream hits = %d, want 1", got)
	}
}

func TestServiceUpstreamFailureIs502(t *testing.T) {
	up, _ := newFakeUpstream(t)
	front := newFrontend(t, &HTTPUpstream{BaseURL: up.URL, Client: up.Client()}, newFakeClock())
	if status, _ := doRequest(t, http.MethodGet, front.URL+"/items/broken"); status != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", status)
	}
}

func TestServiceDeleteInvalidates(t *testing.T) {
	up, hits := newFakeUpstream(t)
	front := newFrontend(t, &HTTPUpstream{BaseURL: up.URL, Client: up.Client()}, newFakeClock())
	doRequest(t, http.MethodGet, front.URL+"/items/7")
	if status, _ := doRequest(t, http.MethodDelete, front.URL+"/items/7/cache"); status != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", status)
	}
	if status, _ := doRequest(t, http.MethodGet, front.URL+"/items/7"); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("upstream hits = %d, want 2", got)
	}
}

// fetchStatus is safe to call from goroutines other than the test's.
func fetchStatus(ctx context.Context, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	if resp == nil {
		return 0, errors.New("nil response")
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return 0, errors.Join(err, resp.Body.Close())
	}
	return resp.StatusCode, resp.Body.Close()
}

// gatedUpstream simulates a slow upstream: each call blocks until released.
type gatedUpstream struct {
	calls   atomic.Int32
	release chan struct{}
}

func (g *gatedUpstream) FetchItem(ctx context.Context, id string) (Item, error) {
	g.calls.Add(1)
	select {
	case <-g.release:
		return Item{ID: id, Name: "slow " + id}, nil
	case <-ctx.Done():
		return Item{}, ctx.Err()
	}
}

func TestServiceConcurrentRequestsShareOneUpstreamCall(t *testing.T) {
	up := &gatedUpstream{release: make(chan struct{})}
	front := newFrontend(t, up, newFakeClock())
	const n = 20
	statuses := make([]int, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			statuses[i], errs[i] = fetchStatus(t.Context(), front.URL+"/items/9")
		})
	}
	// Late arrivals that miss the in flight load hit the stored value, so
	// waiting only for the first upstream call is enough.
	waitFor(t, "upstream call", func() bool { return up.calls.Load() >= 1 })
	close(up.release)
	wg.Wait()
	for i, s := range statuses {
		if errs[i] != nil || s != http.StatusOK {
			t.Fatalf("request %d: status %d err %v, want 200", i, s, errs[i])
		}
	}
	if got := up.calls.Load(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}
}
