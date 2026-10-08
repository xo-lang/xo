package jobqueue

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newTestQueue(maxAttempts int) (*Queue, *FakeClock) {
	c := NewFakeClock(t0)
	return New(Options{MaxAttempts: maxAttempts, Clock: c}), c
}

func TestLeaseExpiryRedeliversAndStaleAckFails(t *testing.T) {
	q, clk := newTestQueue(0)
	id, _ := q.Enqueue([]byte("x"), 0, "")

	j1, tok1, ok := q.Lease("w1", 10*time.Second)
	if !ok || j1.ID != id {
		t.Fatalf("w1 lease: ok=%v id=%q", ok, j1.ID)
	}
	if _, _, ok := q.Lease("w2", 10*time.Second); ok {
		t.Fatal("job should be hidden while leased")
	}

	clk.Advance(10 * time.Second)
	j2, tok2, ok := q.Lease("w2", 10*time.Second)
	if !ok || j2.ID != id {
		t.Fatalf("w2 should get redelivered job: ok=%v id=%q", ok, j2.ID)
	}
	if tok1 == tok2 {
		t.Fatal("each lease must get a new token")
	}
	if j2.Attempts != 2 || j2.LastError != "lease expired" {
		t.Fatalf("attempts=%d lastErr=%q, want 2 and lease expired", j2.Attempts, j2.LastError)
	}
	if err := q.Ack(id, tok1); !errors.Is(err, ErrStaleToken) {
		t.Fatalf("stale ack: got %v, want ErrStaleToken", err)
	}
	if err := q.Extend(id, tok1, time.Second); !errors.Is(err, ErrStaleToken) {
		t.Fatalf("stale extend: got %v, want ErrStaleToken", err)
	}
	if err := q.Nack(id, tok1, "x"); !errors.Is(err, ErrStaleToken) {
		t.Fatalf("stale nack: got %v, want ErrStaleToken", err)
	}
	if err := q.Ack(id, tok2); err != nil {
		t.Fatalf("w2 ack: %v", err)
	}
	if err := q.Ack(id, tok2); !errors.Is(err, ErrStaleToken) {
		t.Fatalf("double ack: got %v, want ErrStaleToken", err)
	}
	if s := q.Stats(); s != (Stats{Done: 1}) {
		t.Fatalf("stats = %+v", s)
	}
}

func TestExpiredLeaseAckFailsEvenWithoutRedelivery(t *testing.T) {
	q, clk := newTestQueue(0)
	id, _ := q.Enqueue(nil, 0, "")
	_, tok, _ := q.Lease("w1", time.Second)
	clk.Advance(2 * time.Second)
	if err := q.Ack(id, tok); !errors.Is(err, ErrStaleToken) {
		t.Fatalf("got %v, want ErrStaleToken", err)
	}
	j, _ := q.Get(id)
	if j.State != StateQueued || j.Attempts != 1 {
		t.Fatalf("state=%s attempts=%d", j.State, j.Attempts)
	}
}

func TestDeadLetterAndRetryDead(t *testing.T) {
	q, clk := newTestQueue(3)
	id, _ := q.Enqueue([]byte("p"), 0, "")

	for attempt := 1; attempt <= 3; attempt++ {
		j, tok, ok := q.Lease("w", time.Minute)
		if !ok {
			t.Fatalf("attempt %d: no job", attempt)
		}
		if j.Attempts != attempt {
			t.Fatalf("attempts = %d, want %d", j.Attempts, attempt)
		}
		if err := q.Nack(id, tok, "boom"); err != nil {
			t.Fatal(err)
		}
		clk.Advance(Backoff(attempt))
	}
	j, _ := q.Get(id)
	if j.State != StateDead || j.LastError != "boom" {
		t.Fatalf("state=%s lastErr=%q, want dead/boom", j.State, j.LastError)
	}
	if s := q.Stats(); s != (Stats{Dead: 1}) {
		t.Fatalf("stats = %+v", s)
	}
	clk.Advance(time.Hour)
	if _, _, ok := q.Lease("w", time.Minute); ok {
		t.Fatal("dead job must not be leased")
	}

	if err := q.RetryDead("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	if err := q.RetryDead(id); err != nil {
		t.Fatal(err)
	}
	if err := q.RetryDead(id); !errors.Is(err, ErrNotDead) {
		t.Fatalf("got %v, want ErrNotDead", err)
	}
	j, tok, ok := q.Lease("w", time.Minute)
	if !ok || j.ID != id || j.Attempts != 1 {
		t.Fatalf("after retry: ok=%v attempts=%d", ok, j.Attempts)
	}
	if err := q.Ack(id, tok); err != nil {
		t.Fatal(err)
	}
}

func TestExpiryCountsTowardDeadLetter(t *testing.T) {
	q, clk := newTestQueue(2)
	id, _ := q.Enqueue(nil, 0, "")
	for range 2 {
		if _, _, ok := q.Lease("w", time.Second); !ok {
			t.Fatal("expected lease")
		}
		clk.Advance(time.Second)
	}
	j, _ := q.Get(id)
	if j.State != StateDead || j.Attempts != 2 {
		t.Fatalf("state=%s attempts=%d", j.State, j.Attempts)
	}
}

func TestBackoffDelays(t *testing.T) {
	want := []time.Duration{1, 2, 4, 8, 16, 30, 30, 30}
	for i, w := range want {
		if got := Backoff(i + 1); got != w*time.Second {
			t.Errorf("Backoff(%d) = %v, want %v", i+1, got, w*time.Second)
		}
	}

	q, clk := newTestQueue(len(want) + 1)
	id, _ := q.Enqueue(nil, 0, "")
	_, tok, _ := q.Lease("w", time.Minute)
	for i, w := range want {
		if err := q.Nack(id, tok, "fail"); err != nil {
			t.Fatalf("nack %d: %v", i+1, err)
		}
		clk.Advance(w*time.Second - time.Millisecond)
		if _, _, ok := q.Lease("w", time.Minute); ok {
			t.Fatalf("after attempt %d: job visible before %v backoff", i+1, w*time.Second)
		}
		clk.Advance(time.Millisecond)
		var ok bool
		_, tok, ok = q.Lease("w", time.Minute)
		if !ok {
			t.Fatalf("after attempt %d: job not visible after %v backoff", i+1, w*time.Second)
		}
	}
}

func TestHeartbeatPreventsRedelivery(t *testing.T) {
	q, clk := newTestQueue(0)
	id, _ := q.Enqueue(nil, 0, "")
	_, tok, _ := q.Lease("w1", 10*time.Second)
	for range 10 { // 80s total, far past the original 10s lease
		clk.Advance(8 * time.Second)
		if err := q.Extend(id, tok, 10*time.Second); err != nil {
			t.Fatalf("extend: %v", err)
		}
		if _, _, ok := q.Lease("w2", 10*time.Second); ok {
			t.Fatal("job redelivered despite heartbeats")
		}
	}
	if err := q.Ack(id, tok); err != nil {
		t.Fatal(err)
	}
	j, _ := q.Get(id)
	if j.Attempts != 1 || j.State != StateDone {
		t.Fatalf("attempts=%d state=%s", j.Attempts, j.State)
	}
}

func TestIdempotencyKeyDeduplicates(t *testing.T) {
	q, clk := newTestQueue(0)
	a, dup := q.Enqueue([]byte("a"), 0, "k1")
	if dup {
		t.Fatal("first enqueue reported duplicate")
	}
	b, dup := q.Enqueue([]byte("b"), 5, "k1")
	if !dup || b != a {
		t.Fatalf("second enqueue: id=%q dup=%v, want %q true", b, dup, a)
	}
	c, dup := q.Enqueue([]byte("c"), 0, "k2")
	if dup || c == a {
		t.Fatal("different key must create a new job")
	}
	n1, _ := q.Enqueue(nil, 0, "")
	n2, _ := q.Enqueue(nil, 0, "")
	if n1 == n2 {
		t.Fatal("empty key must not deduplicate")
	}
	if s := q.Stats(); s.Queued != 4 {
		t.Fatalf("queued = %d, want 4", s.Queued)
	}

	clk.Advance(24*time.Hour - time.Second)
	if id, dup := q.Enqueue(nil, 0, "k1"); !dup || id != a {
		t.Fatal("key should still dedupe within 24h")
	}
	clk.Advance(time.Second)
	if id, dup := q.Enqueue(nil, 0, "k1"); dup || id == a {
		t.Fatal("key should not dedupe after 24h")
	}
}

func TestPriorityOrderingFIFOWithinPriority(t *testing.T) {
	q, clk := newTestQueue(0)
	ids := map[string]string{}
	for _, e := range []struct {
		name string
		prio int
	}{{"a", 0}, {"b", 5}, {"c", 0}, {"d", 5}, {"e", -1}, {"f", 10}} {
		id, _ := q.Enqueue([]byte(e.name), e.prio, "")
		ids[id] = e.name
		clk.Advance(time.Millisecond)
	}
	var got string
	for {
		j, _, ok := q.Lease("w", time.Minute)
		if !ok {
			break
		}
		got += ids[j.ID]
		if string(j.Payload) != ids[j.ID] {
			t.Fatalf("payload mismatch %q", j.Payload)
		}
	}
	if got != "fbdace" {
		t.Fatalf("order = %q, want fbdace", got)
	}
}

func TestSnapshotIsolation(t *testing.T) {
	q, _ := newTestQueue(0)
	p := []byte("abc")
	id, _ := q.Enqueue(p, 0, "")
	p[0] = 'X'
	j, _, _ := q.Lease("w", time.Minute)
	j.Payload[1] = 'Y'
	got, _ := q.Get(id)
	if string(got.Payload) != "abc" {
		t.Fatalf("payload = %q", got.Payload)
	}
}
