package jobqueue

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
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

func mustLease(t *testing.T, q *Queue, worker string, d time.Duration) Leased {
	t.Helper()
	l, ok, err := q.Lease(worker, d)
	if err != nil {
		t.Fatalf("lease: %v", err)
	}
	if !ok {
		t.Fatalf("lease by %s: no visible job", worker)
	}
	return l
}

func expectNoLease(t *testing.T, q *Queue, worker string) {
	t.Helper()
	l, ok, err := q.Lease(worker, time.Minute)
	if err != nil {
		t.Fatalf("lease: %v", err)
	}
	if ok {
		t.Fatalf("lease by %s: unexpectedly got job %s", worker, l.Job.ID)
	}
}

func mustGet(t *testing.T, q *Queue, id string) Job {
	t.Helper()
	j, err := q.Get(id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return j
}

func TestLeaseExpiryRedeliversAndStaleAckFails(t *testing.T) {
	clk := newFakeClock()
	q := New(Config{Clock: clk})
	id, _ := q.Enqueue([]byte("x"), 0, "")

	l1 := mustLease(t, q, "w1", 10*time.Second)
	clk.Advance(9 * time.Second)
	expectNoLease(t, q, "w2")

	// Lease expires at 10s; that is failed attempt 1, so backoff is 1s.
	clk.Advance(time.Second)
	expectNoLease(t, q, "w2")
	clk.Advance(time.Second)

	l2 := mustLease(t, q, "w2", 10*time.Second)
	if l2.Job.ID != id {
		t.Fatalf("redelivered job = %s, want %s", l2.Job.ID, id)
	}
	if l2.Token == l1.Token {
		t.Fatal("second lease reused the first token")
	}
	if l2.Job.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2", l2.Job.Attempts)
	}
	if l2.Job.LastError != "lease expired" {
		t.Fatalf("last error = %q", l2.Job.LastError)
	}

	if err := q.Ack(id, l1.Token); !errors.Is(err, ErrStaleToken) {
		t.Fatalf("stale ack err = %v, want ErrStaleToken", err)
	}
	if err := q.Extend(id, l1.Token, time.Minute); !errors.Is(err, ErrStaleToken) {
		t.Fatalf("stale extend err = %v, want ErrStaleToken", err)
	}
	if err := q.Nack(id, l1.Token, "x"); !errors.Is(err, ErrStaleToken) {
		t.Fatalf("stale nack err = %v, want ErrStaleToken", err)
	}
	if err := q.Ack(id, l2.Token); err != nil {
		t.Fatalf("ack: %v", err)
	}
	if err := q.Ack(id, l2.Token); !errors.Is(err, ErrStaleToken) {
		t.Fatalf("double ack err = %v, want ErrStaleToken", err)
	}
	if got := mustGet(t, q, id).State; got != StateDone {
		t.Fatalf("state = %v, want done", got)
	}
}

func TestAckAfterExpiryWithoutRedeliveryFails(t *testing.T) {
	clk := newFakeClock()
	q := New(Config{Clock: clk})
	id, _ := q.Enqueue(nil, 0, "")
	l := mustLease(t, q, "w1", time.Second)
	clk.Advance(time.Second)
	if err := q.Ack(id, l.Token); !errors.Is(err, ErrStaleToken) {
		t.Fatalf("ack after expiry err = %v, want ErrStaleToken", err)
	}
}

func TestDeadLetterAndRetryDead(t *testing.T) {
	clk := newFakeClock()
	q := New(Config{Clock: clk, MaxAttempts: 3})
	id, _ := q.Enqueue([]byte("p"), 0, "")

	for i := 1; i <= 3; i++ {
		l := mustLease(t, q, "w", time.Minute)
		if l.Job.Attempts != i {
			t.Fatalf("attempt %d: attempts = %d", i, l.Job.Attempts)
		}
		if err := q.Nack(id, l.Token, fmt.Sprintf("boom %d", i)); err != nil {
			t.Fatalf("nack: %v", err)
		}
		clk.Advance(time.Minute)
	}
	j := mustGet(t, q, id)
	if j.State != StateDead || j.Attempts != 3 || j.LastError != "boom 3" {
		t.Fatalf("after max attempts: %+v", j)
	}
	if s := q.Stats(); s != (Stats{Dead: 1}) {
		t.Fatalf("stats = %+v", s)
	}
	expectNoLease(t, q, "w")
	if dl := q.DeadLetters(); len(dl) != 1 || dl[0].ID != id {
		t.Fatalf("dead letters = %+v", dl)
	}

	if err := q.RetryDead(id); err != nil {
		t.Fatalf("retry dead: %v", err)
	}
	j = mustGet(t, q, id)
	if j.State != StateQueued || j.Attempts != 0 {
		t.Fatalf("after retry: %+v", j)
	}
	l := mustLease(t, q, "w", time.Minute)
	if l.Job.Attempts != 1 {
		t.Fatalf("attempts after retry lease = %d, want 1", l.Job.Attempts)
	}
	if err := q.RetryDead(id); !errors.Is(err, ErrNotDead) {
		t.Fatalf("retry of leased job err = %v, want ErrNotDead", err)
	}
	if err := q.RetryDead("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("retry of missing job err = %v, want ErrNotFound", err)
	}
}

func TestExpiredLeaseDeadLetters(t *testing.T) {
	clk := newFakeClock()
	q := New(Config{Clock: clk, MaxAttempts: 1})
	id, _ := q.Enqueue(nil, 0, "")
	mustLease(t, q, "w", time.Second)
	clk.Advance(2 * time.Second)
	if s := q.Stats(); s != (Stats{Dead: 1}) {
		t.Fatalf("stats = %+v", s)
	}
	if j := mustGet(t, q, id); j.LastError != "lease expired" {
		t.Fatalf("last error = %q", j.LastError)
	}
}

func TestBackoffFunction(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{0, time.Second},
		{1, time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{5, 16 * time.Second},
		{6, 30 * time.Second},
		{7, 30 * time.Second},
		{100, 30 * time.Second},
	}
	for _, c := range cases {
		if got := Backoff(c.attempts); got != c.want {
			t.Errorf("Backoff(%d) = %v, want %v", c.attempts, got, c.want)
		}
	}
}

func TestBackoffDelaysWithFakeClock(t *testing.T) {
	clk := newFakeClock()
	q := New(Config{Clock: clk, MaxAttempts: 8})
	id, _ := q.Enqueue(nil, 0, "")
	want := []time.Duration{
		1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second,
		16 * time.Second, 30 * time.Second, 30 * time.Second,
	}
	for i, delay := range want {
		l := mustLease(t, q, "w", time.Minute)
		if err := q.Nack(id, l.Token, "fail"); err != nil {
			t.Fatalf("nack %d: %v", i+1, err)
		}
		clk.Advance(delay - time.Millisecond)
		expectNoLease(t, q, "w")
		clk.Advance(time.Millisecond)
	}
	l := mustLease(t, q, "w", time.Minute)
	if err := q.Nack(id, l.Token, "fail"); err != nil {
		t.Fatalf("final nack: %v", err)
	}
	if j := mustGet(t, q, id); j.State != StateDead || j.Attempts != 8 {
		t.Fatalf("after 8 failures: %+v", j)
	}
}

func TestDefaultMaxAttemptsIsFive(t *testing.T) {
	clk := newFakeClock()
	q := New(Config{Clock: clk})
	id, _ := q.Enqueue(nil, 0, "")
	for range 5 {
		l := mustLease(t, q, "w", time.Minute)
		if err := q.Nack(id, l.Token, "fail"); err != nil {
			t.Fatal(err)
		}
		clk.Advance(time.Minute)
	}
	if j := mustGet(t, q, id); j.State != StateDead {
		t.Fatalf("state = %v, want dead", j.State)
	}
}

func TestHeartbeatExtendKeepsLease(t *testing.T) {
	clk := newFakeClock()
	q := New(Config{Clock: clk})
	id, _ := q.Enqueue(nil, 0, "")
	l := mustLease(t, q, "w1", 3*time.Second)
	for range 10 {
		clk.Advance(2 * time.Second)
		if err := q.Extend(id, l.Token, 3*time.Second); err != nil {
			t.Fatalf("extend: %v", err)
		}
		expectNoLease(t, q, "w2")
	}
	if err := q.Ack(id, l.Token); err != nil {
		t.Fatalf("ack: %v", err)
	}
	if _, _, err := q.Lease("w", 0); !errors.Is(err, ErrInvalidDuration) {
		t.Fatalf("zero lease err = %v", err)
	}
}

func TestIdempotencyKey(t *testing.T) {
	clk := newFakeClock()
	q := New(Config{Clock: clk})
	id1, created1 := q.Enqueue([]byte("a"), 0, "k")
	id2, created2 := q.Enqueue([]byte("b"), 5, "k")
	if !created1 || created2 || id1 != id2 {
		t.Fatalf("dedupe: (%s,%v) (%s,%v)", id1, created1, id2, created2)
	}
	if s := q.Stats(); s.Queued != 1 {
		t.Fatalf("queued = %d, want 1", s.Queued)
	}
	id3, created3 := q.Enqueue(nil, 0, "other")
	if !created3 || id3 == id1 {
		t.Fatalf("distinct key reused id")
	}
	id4, created4 := q.Enqueue(nil, 0, "")
	id5, created5 := q.Enqueue(nil, 0, "")
	if !created4 || !created5 || id4 == id5 {
		t.Fatal("empty key must not deduplicate")
	}

	clk.Advance(IdempotencyWindow - time.Minute)
	if id, created := q.Enqueue(nil, 0, "k"); created || id != id1 {
		t.Fatalf("within window: (%s,%v)", id, created)
	}
	clk.Advance(time.Minute)
	if id, created := q.Enqueue(nil, 0, "k"); !created || id == id1 {
		t.Fatalf("after window: (%s,%v)", id, created)
	}
}

func TestPriorityOrderingFIFOWithinPriority(t *testing.T) {
	q := New(Config{Clock: newFakeClock()})
	a, _ := q.Enqueue([]byte("a"), 1, "")
	b, _ := q.Enqueue([]byte("b"), 5, "")
	c, _ := q.Enqueue([]byte("c"), 1, "")
	d, _ := q.Enqueue([]byte("d"), 5, "")
	e, _ := q.Enqueue([]byte("e"), -3, "")
	f, _ := q.Enqueue([]byte("f"), 1, "")
	want := []string{b, d, a, c, f, e}
	for i, w := range want {
		l := mustLease(t, q, "w", time.Minute)
		if l.Job.ID != w {
			t.Fatalf("lease %d = %s, want %s", i, l.Job.ID, w)
		}
	}
	expectNoLease(t, q, "w")
}

func TestQueueConcurrentLeasing(t *testing.T) {
	q := New(Config{})
	const n = 500
	for i := range n {
		q.Enqueue([]byte{byte(i)}, i%7, "")
	}
	var mu sync.Mutex
	seen := make(map[string]int)
	var wg sync.WaitGroup
	for w := range 20 {
		wg.Go(func() {
			for {
				l, ok, err := q.Lease(fmt.Sprintf("w%d", w), time.Hour)
				if err != nil {
					t.Errorf("lease: %v", err)
					return
				}
				if !ok {
					return
				}
				mu.Lock()
				seen[l.Job.ID]++
				mu.Unlock()
				if err := q.Ack(l.Job.ID, l.Token); err != nil {
					t.Errorf("ack: %v", err)
				}
			}
		})
	}
	wg.Wait()
	if len(seen) != n {
		t.Fatalf("saw %d jobs, want %d", len(seen), n)
	}
	for id, c := range seen {
		if c != 1 {
			t.Fatalf("job %s leased %d times", id, c)
		}
	}
	if s := q.Stats(); s != (Stats{Done: n}) {
		t.Fatalf("stats = %+v", s)
	}
}
