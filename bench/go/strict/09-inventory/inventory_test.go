package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func mustItem(t *testing.T, s *Service, id string, stock int) {
	t.Helper()
	if _, err := s.CreateItem(id, "widget", stock); err != nil {
		t.Fatalf("create item: %v", err)
	}
}

func mustView(t *testing.T, s *Service, id string) ItemView {
	t.Helper()
	v, err := s.GetItem(id)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	return v
}

func checkView(t *testing.T, got ItemView, stock, held, avail int) {
	t.Helper()
	if got.Stock != stock || got.Held != held || got.Available != avail {
		t.Fatalf("item = stock %d held %d available %d, want %d %d %d", got.Stock, got.Held, got.Available, stock, held, avail)
	}
}

func TestConcurrentReservationsNeverOversell(t *testing.T) {
	s := NewService(newFakeClock().Now)
	mustItem(t, s, "a", 10)

	var ok, insufficient atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range 100 {
		wg.Go(func() {
			<-start
			_, _, err := s.Reserve("a", 1, fmt.Sprintf("k-%d", i))
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrInsufficientStock):
				insufficient.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
	close(start)
	wg.Wait()
	if ok.Load() != 10 || insufficient.Load() != 90 {
		t.Fatalf("succeeded %d, insufficient %d; want 10 and 90", ok.Load(), insufficient.Load())
	}
	checkView(t, mustView(t, s, "a"), 10, 10, 0)
}

func TestConcurrentReservationsOverHTTP(t *testing.T) {
	s := NewService(newFakeClock().Now)
	mustItem(t, s, "a", 10)
	srv := httptest.NewServer(NewHandler(s))
	defer srv.Close()

	var created, conflict atomic.Int64
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Go(func() {
			body := fmt.Sprintf(`{"item_id":"a","qty":1,"idempotency_key":"h-%d"}`, i)
			resp, err := post(srv.URL+"/reservations", body)
			if err != nil {
				t.Errorf("post: %v", err)
				return
			}
			defer closeBody(t, resp)
			switch resp.StatusCode {
			case http.StatusCreated:
				created.Add(1)
			case http.StatusConflict:
				conflict.Add(1)
			default:
				t.Errorf("status %d", resp.StatusCode)
			}
		})
	}
	wg.Wait()
	if created.Load() != 10 || conflict.Load() != 90 {
		t.Fatalf("created %d conflict %d", created.Load(), conflict.Load())
	}
}

func TestIdempotentReserve(t *testing.T) {
	s := NewService(newFakeClock().Now)
	mustItem(t, s, "a", 5)
	r1, replayed, err := s.Reserve("a", 2, "key-1")
	if err != nil || replayed {
		t.Fatalf("first reserve: %v replayed=%v", err, replayed)
	}
	r2, replayed, err := s.Reserve("a", 2, "key-1")
	if err != nil || !replayed {
		t.Fatalf("second reserve: %v replayed=%v", err, replayed)
	}
	if r1.ID != r2.ID {
		t.Fatalf("ids differ: %s vs %s", r1.ID, r2.ID)
	}
	checkView(t, mustView(t, s, "a"), 5, 2, 3)

	if _, _, err := s.Reserve("a", 3, "key-1"); !errors.Is(err, ErrConflict) {
		t.Fatalf("mismatched reuse: got %v, want ErrConflict", err)
	}
	checkView(t, mustView(t, s, "a"), 5, 2, 3)
}

func TestIdempotentReserveConcurrent(t *testing.T) {
	s := NewService(newFakeClock().Now)
	mustItem(t, s, "a", 5)
	ids := make([]string, 50)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Go(func() {
			r, _, err := s.Reserve("a", 1, "same")
			if err != nil {
				t.Errorf("reserve: %v", err)
				return
			}
			ids[i] = r.ID
		})
	}
	wg.Wait()
	for _, id := range ids {
		if id != ids[0] {
			t.Fatalf("got different reservation ids %s and %s", id, ids[0])
		}
	}
	checkView(t, mustView(t, s, "a"), 5, 1, 4)
}

func TestExpiredHoldReleasedAndConfirmGone(t *testing.T) {
	clk := newFakeClock()
	s := NewService(clk.Now)
	mustItem(t, s, "a", 3)
	r, _, err := s.Reserve("a", 2, "")
	if err != nil {
		t.Fatal(err)
	}
	clk.Advance(HoldDuration - time.Second)
	checkView(t, mustView(t, s, "a"), 3, 2, 1)

	clk.Advance(time.Second)
	// Lazy release on read.
	checkView(t, mustView(t, s, "a"), 3, 0, 3)

	if _, err := s.Confirm(r.ID); !errors.Is(err, ErrExpired) {
		t.Fatalf("confirm after expiry: got %v, want ErrExpired", err)
	}

	// Same through HTTP: 410.
	srv := httptest.NewServer(NewHandler(s))
	defer srv.Close()
	resp, err := post(srv.URL+"/reservations/"+r.ID+"/confirm", "")
	if err != nil {
		t.Fatal(err)
	}
	closeBody(t, resp)
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("status = %d, want 410", resp.StatusCode)
	}
}

func TestConfirmExpiredWithoutPriorRead(t *testing.T) {
	clk := newFakeClock()
	s := NewService(clk.Now)
	mustItem(t, s, "a", 1)
	r, _, err := s.Reserve("a", 1, "")
	if err != nil {
		t.Fatal(err)
	}
	clk.Advance(HoldDuration)
	if _, err := s.Confirm(r.ID); !errors.Is(err, ErrExpired) {
		t.Fatalf("got %v, want ErrExpired", err)
	}
	checkView(t, mustView(t, s, "a"), 1, 0, 1)
}

func TestSweeperReleasesExpiredHolds(t *testing.T) {
	clk := newFakeClock()
	s := NewService(clk.Now)
	mustItem(t, s, "a", 4)
	r, _, err := s.Reserve("a", 4, "")
	if err != nil {
		t.Fatal(err)
	}
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.RunSweeper(ctx, ticks)
	}()
	clk.Advance(HoldDuration + time.Minute)
	ticks <- clk.Now()
	cancel()
	<-done

	// Inspect internal state without the lazy path to prove the sweeper did it.
	s.mu.Lock()
	it, ok := s.items["a"]
	var held int
	if ok {
		held = it.held
	}
	st := StatusActive
	if rr, found := s.res[r.ID]; found {
		st = rr.Status
	}
	s.mu.Unlock()
	if !ok || held != 0 || st != StatusExpired {
		t.Fatalf("after sweep: held %d status %v", held, st)
	}
}

func TestConfirmThenCancelFails(t *testing.T) {
	s := NewService(newFakeClock().Now)
	mustItem(t, s, "a", 5)
	r, _, err := s.Reserve("a", 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Confirm(r.ID); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if _, err := s.Cancel(r.ID); !errors.Is(err, ErrAlreadyConfirmed) {
		t.Fatalf("cancel after confirm: got %v", err)
	}
	if _, err := s.Confirm(r.ID); !errors.Is(err, ErrAlreadyConfirmed) {
		t.Fatalf("double confirm: got %v", err)
	}
	checkView(t, mustView(t, s, "a"), 3, 0, 3)
}

func TestCancelThenConfirmFails(t *testing.T) {
	s := NewService(newFakeClock().Now)
	mustItem(t, s, "a", 5)
	r, _, err := s.Reserve("a", 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Cancel(r.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := s.Confirm(r.ID); !errors.Is(err, ErrCancelled) {
		t.Fatalf("confirm after cancel: got %v", err)
	}
	checkView(t, mustView(t, s, "a"), 5, 0, 5)
}

func TestAvailableAfterMixedOperations(t *testing.T) {
	clk := newFakeClock()
	s := NewService(clk.Now)
	mustItem(t, s, "a", 20)

	confirmMe, _, err := s.Reserve("a", 3, "c")
	if err != nil {
		t.Fatal(err)
	}
	cancelMe, _, err := s.Reserve("a", 4, "x")
	if err != nil {
		t.Fatal(err)
	}
	expireMe, _, err := s.Reserve("a", 5, "e")
	if err != nil {
		t.Fatal(err)
	}
	checkView(t, mustView(t, s, "a"), 20, 12, 8)

	if _, err := s.Confirm(confirmMe.ID); err != nil {
		t.Fatal(err)
	}
	checkView(t, mustView(t, s, "a"), 17, 9, 8)

	if _, err := s.Cancel(cancelMe.ID); err != nil {
		t.Fatal(err)
	}
	checkView(t, mustView(t, s, "a"), 17, 5, 12)

	clk.Advance(10 * time.Minute)
	keep, _, err := s.Reserve("a", 2, "k")
	if err != nil {
		t.Fatal(err)
	}
	checkView(t, mustView(t, s, "a"), 17, 7, 10)

	clk.Advance(6 * time.Minute) // expireMe is now past 15 min, keep is at 6 min.
	checkView(t, mustView(t, s, "a"), 17, 2, 15)
	if _, err := s.Confirm(expireMe.ID); !errors.Is(err, ErrExpired) {
		t.Fatalf("expected expired, got %v", err)
	}
	if _, err := s.Confirm(keep.ID); err != nil {
		t.Fatal(err)
	}
	checkView(t, mustView(t, s, "a"), 15, 0, 15)

	if _, _, err := s.Reserve("a", 16, ""); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("expected insufficient, got %v", err)
	}
	if _, _, err := s.Reserve("a", 15, ""); err != nil {
		t.Fatal(err)
	}
	checkView(t, mustView(t, s, "a"), 15, 15, 0)
}

func doJSON(t *testing.T, method, url, body string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := send(req)
	if err != nil {
		t.Fatal(err)
	}
	defer closeBody(t, resp)
	out := map[string]any{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.StatusCode, out
}

func closeBody(t *testing.T, resp *http.Response) {
	t.Helper()
	if err := resp.Body.Close(); err != nil {
		t.Errorf("close body: %v", err)
	}
}

func TestHTTPStatusMapping(t *testing.T) {
	s := NewService(newFakeClock().Now)
	srv := httptest.NewServer(NewHandler(s))
	defer srv.Close()
	u := srv.URL

	if code, _ := doJSON(t, "POST", u+"/items", `{"id":"a","name":"widget","stock":2}`); code != http.StatusCreated {
		t.Fatalf("create = %d", code)
	}
	if code, _ := doJSON(t, "POST", u+"/items", `{"id":"a","name":"widget","stock":2}`); code != http.StatusConflict {
		t.Fatalf("duplicate create = %d", code)
	}
	if code, _ := doJSON(t, "POST", u+"/items", `{"name":"widget","stock":-1}`); code != http.StatusBadRequest {
		t.Fatalf("negative stock = %d", code)
	}
	if code, _ := doJSON(t, "POST", u+"/items", `{"name":"widget"}`); code != http.StatusBadRequest {
		t.Fatalf("missing stock = %d", code)
	}
	if code, _ := doJSON(t, "POST", u+"/items", `not json`); code != http.StatusBadRequest {
		t.Fatalf("bad json = %d", code)
	}
	if code, _ := doJSON(t, "GET", u+"/items/nope", ""); code != http.StatusNotFound {
		t.Fatalf("missing item = %d", code)
	}
	if code, _ := doJSON(t, "POST", u+"/reservations", `{"item_id":"a","qty":0}`); code != http.StatusBadRequest {
		t.Fatalf("zero qty = %d", code)
	}
	if code, _ := doJSON(t, "POST", u+"/reservations", `{"item_id":"nope","qty":1}`); code != http.StatusNotFound {
		t.Fatalf("missing item reserve = %d", code)
	}
	if code, _ := doJSON(t, "POST", u+"/reservations", `{"item_id":"a","qty":3}`); code != http.StatusConflict {
		t.Fatalf("over reserve = %d", code)
	}
	code, res := doJSON(t, "POST", u+"/reservations", `{"item_id":"a","qty":2,"idempotency_key":"k"}`)
	if code != http.StatusCreated {
		t.Fatalf("reserve = %d", code)
	}
	id, _ := res["id"].(string)
	if res["status"] != "active" || id == "" {
		t.Fatalf("reservation body = %v", res)
	}
	code, res2 := doJSON(t, "POST", u+"/reservations", `{"item_id":"a","qty":2,"idempotency_key":"k"}`)
	if code != http.StatusOK || res2["id"] != id {
		t.Fatalf("replay = %d %v", code, res2)
	}
	code, item := doJSON(t, "GET", u+"/items/a", "")
	if code != http.StatusOK || item["stock"] != 2.0 || item["held"] != 2.0 || item["available"] != 0.0 {
		t.Fatalf("item = %d %v", code, item)
	}
	if code, _ := doJSON(t, "POST", u+"/reservations/"+id+"/confirm", ""); code != http.StatusOK {
		t.Fatalf("confirm = %d", code)
	}
	if code, _ := doJSON(t, "POST", u+"/reservations/"+id+"/confirm", ""); code != http.StatusConflict {
		t.Fatalf("double confirm = %d", code)
	}
	if code, _ := doJSON(t, "POST", u+"/reservations/"+id+"/cancel", ""); code != http.StatusConflict {
		t.Fatalf("cancel confirmed = %d", code)
	}
	if code, _ := doJSON(t, "POST", u+"/reservations/nope/cancel", ""); code != http.StatusNotFound {
		t.Fatalf("cancel missing = %d", code)
	}
	code, item = doJSON(t, "GET", u+"/items/a", "")
	if code != http.StatusOK || item["stock"] != 0.0 || item["available"] != 0.0 {
		t.Fatalf("item after confirm = %d %v", code, item)
	}
}

func TestRunGracefulShutdownStopsSweeper(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clk := newFakeClock()
	s := NewService(clk.Now)
	mustItem(t, s, "a", 1)
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- run(ctx, ln, s, ticks) }()

	code, item := doJSON(t, "GET", "http://"+ln.Addr().String()+"/items/a", "")
	if code != http.StatusOK || item["available"] != 1.0 {
		t.Fatalf("get = %d %v", code, item)
	}
	ticks <- clk.Now() // sweeper is running

	cancel()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("run returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not return")
	}
	// The sweeper goroutine has exited, so nobody receives ticks any more.
	select {
	case ticks <- clk.Now():
		t.Fatal("sweeper still receiving after shutdown")
	default:
	}
}

// send wraps http.DefaultClient.Do and guarantees a non nil response when
// err is nil.
func send(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("nil response")
	}
	return resp, nil
}

func post(url, body string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return send(req)
}
