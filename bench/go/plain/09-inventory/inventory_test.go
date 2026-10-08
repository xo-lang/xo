package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func mustItem(t *testing.T, s *Store, id string, stock int) {
	t.Helper()
	if _, err := s.CreateItem(id, "widget", stock); err != nil {
		t.Fatalf("create item: %v", err)
	}
}

func checkItem(t *testing.T, s *Store, id string, stock, held, avail int) {
	t.Helper()
	v, err := s.GetItem(id)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if v.Stock != stock || v.Held != held || v.Available != avail {
		t.Fatalf("item %s: got stock=%d held=%d avail=%d, want %d/%d/%d",
			id, v.Stock, v.Held, v.Available, stock, held, avail)
	}
}

func TestConcurrentReservationsNeverOversell(t *testing.T) {
	s := NewStore(newFakeClock())
	mustItem(t, s, "a", 10)

	var ok, conflict atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _, err := s.Reserve("a", 1, "")
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrInsufficientStock):
				conflict.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if ok.Load() != 10 || conflict.Load() != 90 {
		t.Fatalf("ok=%d conflict=%d, want 10/90", ok.Load(), conflict.Load())
	}
	checkItem(t, s, "a", 10, 10, 0)
}

func TestConcurrentReservationsHTTP(t *testing.T) {
	s := NewStore(newFakeClock())
	srv := httptest.NewServer(NewHandler(s))
	defer srv.Close()

	resp := post(t, srv.URL+"/items", `{"id":"a","name":"widget","stock":10}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d", resp.StatusCode)
	}

	var created, conflict atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := http.Post(srv.URL+"/reservations", "application/json",
				bytes.NewBufferString(`{"item_id":"a","qty":1}`))
			if err != nil {
				t.Errorf("post: %v", err)
				return
			}
			r.Body.Close()
			switch r.StatusCode {
			case http.StatusCreated:
				created.Add(1)
			case http.StatusConflict:
				conflict.Add(1)
			default:
				t.Errorf("unexpected status %d", r.StatusCode)
			}
		}()
	}
	wg.Wait()
	if created.Load() != 10 || conflict.Load() != 90 {
		t.Fatalf("created=%d conflict=%d", created.Load(), conflict.Load())
	}
	var v ItemView
	getJSON(t, srv.URL+"/items/a", http.StatusOK, &v)
	if v.Available != 0 || v.Held != 10 || v.Stock != 10 {
		t.Fatalf("view %+v", v)
	}
}

func TestIdempotentReserve(t *testing.T) {
	s := NewStore(newFakeClock())
	mustItem(t, s, "a", 5)

	r1, created1, err := s.Reserve("a", 2, "key-1")
	if err != nil || !created1 {
		t.Fatalf("first reserve: %v created=%v", err, created1)
	}
	r2, created2, err := s.Reserve("a", 2, "key-1")
	if err != nil || created2 {
		t.Fatalf("second reserve: %v created=%v", err, created2)
	}
	if r1.ID != r2.ID {
		t.Fatalf("ids differ: %s vs %s", r1.ID, r2.ID)
	}
	checkItem(t, s, "a", 5, 2, 3)

	if _, _, err := s.Reserve("a", 3, "key-1"); !errors.Is(err, ErrIdempotencyReuse) {
		t.Fatalf("reuse with different qty: %v", err)
	}
	checkItem(t, s, "a", 5, 2, 3)
}

func TestIdempotentReserveConcurrent(t *testing.T) {
	s := NewStore(newFakeClock())
	mustItem(t, s, "a", 5)
	ids := make([]string, 50)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, _, err := s.Reserve("a", 1, "same")
			if err != nil {
				t.Errorf("reserve: %v", err)
				return
			}
			ids[i] = r.ID
		}(i)
	}
	wg.Wait()
	for _, id := range ids {
		if id != ids[0] {
			t.Fatalf("got different reservation ids %s and %s", id, ids[0])
		}
	}
	checkItem(t, s, "a", 5, 1, 4)
}

func TestIdempotentReserveHTTP(t *testing.T) {
	s := NewStore(newFakeClock())
	h := NewHandler(s)
	mustItem(t, s, "a", 5)

	do := func() (int, Reservation) {
		req := httptest.NewRequest("POST", "/reservations", bytes.NewBufferString(`{"item_id":"a","qty":2}`))
		req.Header.Set("Idempotency-Key", "k")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var r Reservation
		_ = json.Unmarshal(rec.Body.Bytes(), &r)
		return rec.Code, r
	}
	c1, r1 := do()
	c2, r2 := do()
	if c1 != http.StatusCreated || c2 != http.StatusOK || r1.ID != r2.ID || r1.ID == "" {
		t.Fatalf("codes %d %d ids %q %q", c1, c2, r1.ID, r2.ID)
	}
	checkItem(t, s, "a", 5, 2, 3)
}

func TestExpiredHoldReleasedAndConfirmGone(t *testing.T) {
	clk := newFakeClock()
	s := NewStore(clk)
	h := NewHandler(s)
	mustItem(t, s, "a", 3)

	r, _, err := s.Reserve("a", 3, "")
	if err != nil {
		t.Fatal(err)
	}
	checkItem(t, s, "a", 3, 3, 0)

	clk.Advance(HoldDuration - time.Second)
	checkItem(t, s, "a", 3, 3, 0)

	clk.Advance(time.Second)
	// Lazy release on read.
	checkItem(t, s, "a", 3, 0, 3)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/reservations/"+r.ID+"/confirm", nil))
	if rec.Code != http.StatusGone {
		t.Fatalf("confirm after expiry: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := s.Confirm(r.ID); !errors.Is(err, ErrExpired) {
		t.Fatalf("store confirm: %v", err)
	}
	checkItem(t, s, "a", 3, 0, 3)
}

func TestSweeperReleasesExpiredHolds(t *testing.T) {
	clk := newFakeClock()
	s := NewStore(clk)
	mustItem(t, s, "a", 4)
	r, _, err := s.Reserve("a", 4, "")
	if err != nil {
		t.Fatal(err)
	}

	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.RunSweeper(ctx, ticks)
		close(done)
	}()

	clk.Advance(HoldDuration + time.Minute)
	ticks <- clk.Now() // unbuffered: sweeper has received it
	ticks <- clk.Now() // second send guarantees the first sweep completed

	// Inspect the reservation directly without the lazy item path.
	s.mu.Lock()
	st := s.reservations[r.ID].Status
	held := s.items["a"].held
	s.mu.Unlock()
	if st != StatusExpired || held != 0 {
		t.Fatalf("after sweep: status=%s held=%d", st, held)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("sweeper did not stop")
	}
}

func TestStartSweeperStops(t *testing.T) {
	s := NewStore(newFakeClock())
	stop := s.StartSweeper(time.Hour)
	finished := make(chan struct{})
	go func() { stop(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not return")
	}
}

func TestConfirmThenCancelFails(t *testing.T) {
	s := NewStore(newFakeClock())
	h := NewHandler(s)
	mustItem(t, s, "a", 5)
	r, _, _ := s.Reserve("a", 2, "")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/reservations/"+r.ID+"/confirm", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/reservations/"+r.ID+"/cancel", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("cancel after confirm: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/reservations/"+r.ID+"/confirm", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("double confirm: %d", rec.Code)
	}
	checkItem(t, s, "a", 3, 0, 3)
}

func TestCancelThenConfirmFails(t *testing.T) {
	s := NewStore(newFakeClock())
	h := NewHandler(s)
	mustItem(t, s, "a", 5)
	r, _, _ := s.Reserve("a", 2, "")
	checkItem(t, s, "a", 5, 2, 3)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/reservations/"+r.ID+"/cancel", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel: %d", rec.Code)
	}
	checkItem(t, s, "a", 5, 0, 5)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/reservations/"+r.ID+"/confirm", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("confirm after cancel: %d", rec.Code)
	}
	if _, err := s.Confirm(r.ID); !errors.Is(err, ErrCancelled) {
		t.Fatalf("store: %v", err)
	}
	checkItem(t, s, "a", 5, 0, 5)
}

func TestAvailableAfterMix(t *testing.T) {
	clk := newFakeClock()
	s := NewStore(clk)
	mustItem(t, s, "a", 20)

	r1, _, _ := s.Reserve("a", 3, "") // will be confirmed
	r2, _, _ := s.Reserve("a", 4, "") // will be cancelled
	clk.Advance(10 * time.Minute)
	r3, _, _ := s.Reserve("a", 5, "") // stays active
	_, _, _ = s.Reserve("a", 2, "")   // will expire (created at t+10m, expires t+25m)
	r5, _, _ := s.Reserve("a", 1, "") // will be confirmed before expiry
	checkItem(t, s, "a", 20, 15, 5)

	if _, err := s.Confirm(r1.ID); err != nil {
		t.Fatal(err)
	}
	checkItem(t, s, "a", 17, 12, 5)
	if _, err := s.Cancel(r2.ID); err != nil {
		t.Fatal(err)
	}
	checkItem(t, s, "a", 17, 8, 9)
	if _, err := s.Confirm(r5.ID); err != nil {
		t.Fatal(err)
	}
	checkItem(t, s, "a", 16, 7, 9)

	// Too much for what is available.
	if _, _, err := s.Reserve("a", 10, ""); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("want insufficient, got %v", err)
	}

	clk.Advance(15 * time.Minute) // r3 and r4 expire now
	checkItem(t, s, "a", 16, 0, 16)
	if _, err := s.Cancel(r3.ID); !errors.Is(err, ErrExpired) {
		t.Fatalf("cancel expired: %v", err)
	}
	if _, _, err := s.Reserve("a", 16, ""); err != nil {
		t.Fatalf("reserve all: %v", err)
	}
	checkItem(t, s, "a", 16, 16, 0)
}

func TestHTTPErrors(t *testing.T) {
	s := NewStore(newFakeClock())
	h := NewHandler(s)
	cases := []struct {
		method, path, body string
		want               int
	}{
		{"POST", "/items", `{"name":"x"}`, 400},
		{"POST", "/items", `{"name":"x","stock":-1}`, 400},
		{"POST", "/items", `not json`, 400},
		{"POST", "/items", `{"id":"a","name":"x","stock":2}`, 201},
		{"POST", "/items", `{"id":"a","name":"x","stock":2}`, 409},
		{"GET", "/items/missing", ``, 404},
		{"POST", "/reservations", `{"item_id":"missing","qty":1}`, 404},
		{"POST", "/reservations", `{"item_id":"a","qty":0}`, 400},
		{"POST", "/reservations", `{"item_id":"a","qty":3}`, 409},
		{"POST", "/reservations/nope/confirm", ``, 404},
		{"POST", "/reservations/nope/cancel", ``, 404},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		var body *bytes.Buffer
		if c.body != "" {
			body = bytes.NewBufferString(c.body)
		} else {
			body = &bytes.Buffer{}
		}
		h.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, body))
		if rec.Code != c.want {
			t.Errorf("%s %s %s: got %d want %d (%s)", c.method, c.path, c.body, rec.Code, c.want, rec.Body.String())
		}
	}
}

func post(t *testing.T, url, body string) *http.Response {
	t.Helper()
	r, err := http.Post(url, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	return r
}

func getJSON(t *testing.T, url string, want int, v any) {
	t.Helper()
	r, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if r.StatusCode != want {
		t.Fatalf("GET %s: %d", url, r.StatusCode)
	}
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}
