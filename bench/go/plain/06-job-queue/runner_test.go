package jobqueue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// waitFor polls cond until it is true or the deadline passes.
func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met before deadline")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestRunnerHeartbeatKeepsLongJob(t *testing.T) {
	// Real clock with short durations: the handler runs 4x the lease length.
	q := New(Options{})
	id, _ := q.Enqueue(nil, 0, "")
	var calls atomic.Int32
	var errs atomic.Int32
	r := NewRunner(q, RunnerConfig{
		Workers:       3,
		LeaseDuration: 150 * time.Millisecond,
		PollInterval:  5 * time.Millisecond,
		Handler: func(ctx context.Context, j Job) error {
			calls.Add(1)
			select {
			case <-time.After(600 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		OnError: func(Job, error) { errs.Add(1) },
	})
	r.Start()
	waitFor(t, 10*time.Second, func() bool { return q.Stats().Done == 1 })
	r.Shutdown(time.Second)
	j, _ := q.Get(id)
	if calls.Load() != 1 || j.Attempts != 1 || errs.Load() != 0 {
		t.Fatalf("calls=%d attempts=%d errs=%d, want 1/1/0", calls.Load(), j.Attempts, errs.Load())
	}
}

func TestRunnerHandlerErrorNacksAndPanicIsRecovered(t *testing.T) {
	clk := NewFakeClock(t0)
	q := New(Options{Clock: clk, MaxAttempts: 1})
	bad, _ := q.Enqueue([]byte("err"), 0, "")
	pan, _ := q.Enqueue([]byte("panic"), 0, "")
	good, _ := q.Enqueue([]byte("ok"), 0, "")
	r := NewRunner(q, RunnerConfig{
		Workers:      2,
		PollInterval: 2 * time.Millisecond,
		Handler: func(ctx context.Context, j Job) error {
			switch string(j.Payload) {
			case "err":
				return errors.New("handler failed")
			case "panic":
				panic("kaboom")
			}
			return nil
		},
	})
	r.Start()
	waitFor(t, 10*time.Second, func() bool {
		s := q.Stats()
		return s.Done == 1 && s.Dead == 2
	})
	r.Shutdown(time.Second)
	if j, _ := q.Get(bad); j.LastError != "handler failed" {
		t.Fatalf("bad last error %q", j.LastError)
	}
	if j, _ := q.Get(pan); !strings.Contains(j.LastError, "kaboom") {
		t.Fatalf("panic last error %q", j.LastError)
	}
	if j, _ := q.Get(good); j.State != StateDone {
		t.Fatalf("good state %s", j.State)
	}
}

func TestRunnerShutdownNacksInFlightAfterGrace(t *testing.T) {
	// Fake queue clock: leases never expire on their own during the test.
	clk := NewFakeClock(t0)
	q := New(Options{Clock: clk})
	const n = 4
	var ids []string
	for i := range n {
		id, _ := q.Enqueue(fmt.Appendf(nil, "%d", i), 0, "")
		ids = append(ids, id)
	}
	var started sync.WaitGroup
	started.Add(n)
	var active, exited atomic.Int32
	r := NewRunner(q, RunnerConfig{
		Workers:       n,
		LeaseDuration: 30 * time.Millisecond,
		PollInterval:  2 * time.Millisecond,
		Handler: func(ctx context.Context, j Job) error {
			active.Add(1)
			defer func() { active.Add(-1); exited.Add(1) }()
			started.Done()
			<-ctx.Done() // a long job that only stops when cancelled
			return ctx.Err()
		},
	})
	r.Start()
	started.Wait()

	begin := time.Now()
	forced := r.Shutdown(50 * time.Millisecond)
	elapsed := time.Since(begin)

	if !forced {
		t.Fatal("expected forced cancellation")
	}
	if elapsed < 50*time.Millisecond {
		t.Fatalf("shutdown returned after %v, before the grace period", elapsed)
	}
	if r.Running() != 0 || active.Load() != 0 || exited.Load() != n {
		t.Fatalf("running=%d active=%d exited=%d", r.Running(), active.Load(), exited.Load())
	}
	for _, id := range ids {
		j, _ := q.Get(id)
		if j.State != StateQueued || j.Attempts != 1 || !strings.Contains(j.LastError, "canceled") {
			t.Fatalf("job %s: state=%s attempts=%d lastErr=%q", id, j.State, j.Attempts, j.LastError)
		}
	}
	if s := q.Stats(); s != (Stats{Queued: n}) {
		t.Fatalf("stats = %+v", s)
	}
}

func TestRunnerShutdownWaitsForHandlersWithinGrace(t *testing.T) {
	clk := NewFakeClock(t0)
	q := New(Options{Clock: clk})
	id, _ := q.Enqueue(nil, 0, "")
	started := make(chan struct{})
	release := make(chan struct{})
	r := NewRunner(q, RunnerConfig{
		Workers:      2,
		PollInterval: 2 * time.Millisecond,
		Handler: func(ctx context.Context, j Job) error {
			close(started)
			<-release
			return nil
		},
	})
	r.Start()
	<-started
	go func() {
		time.Sleep(20 * time.Millisecond)
		close(release)
	}()
	if forced := r.Shutdown(5 * time.Second); forced {
		t.Fatal("did not expect forced shutdown")
	}
	if r.Running() != 0 {
		t.Fatalf("running = %d", r.Running())
	}
	if j, _ := q.Get(id); j.State != StateDone {
		t.Fatalf("state = %s, want done", j.State)
	}
}

func TestRunnerShutdownStopsLeasing(t *testing.T) {
	clk := NewFakeClock(t0)
	q := New(Options{Clock: clk})
	r := NewRunner(q, RunnerConfig{
		Workers:      3,
		PollInterval: time.Millisecond,
		Handler:      func(ctx context.Context, j Job) error { return nil },
	})
	r.Start()
	r.Shutdown(time.Second)
	q.Enqueue(nil, 0, "")
	time.Sleep(10 * time.Millisecond)
	if s := q.Stats(); s.Queued != 1 {
		t.Fatalf("stats after shutdown = %+v", s)
	}
}

func TestRunner50Workers1000JobsExactlyOnce(t *testing.T) {
	q := New(Options{})
	const jobs = 1000
	want := make(map[string]bool, jobs)
	for i := range jobs {
		id, _ := q.Enqueue(fmt.Appendf(nil, "%d", i), i%7, "")
		want[id] = true
	}
	var mu sync.Mutex
	counts := make(map[string]int, jobs)
	var errs atomic.Int32
	r := NewRunner(q, RunnerConfig{
		Workers:       50,
		LeaseDuration: 10 * time.Second,
		PollInterval:  time.Millisecond,
		Handler: func(ctx context.Context, j Job) error {
			mu.Lock()
			counts[j.ID]++
			mu.Unlock()
			return nil
		},
		OnError: func(Job, error) { errs.Add(1) },
	})
	r.Start()
	waitFor(t, 30*time.Second, func() bool { return q.Stats().Done == jobs })
	if r.Shutdown(5 * time.Second) {
		t.Fatal("unexpected forced shutdown")
	}
	if r.Running() != 0 {
		t.Fatalf("running = %d", r.Running())
	}
	if errs.Load() != 0 {
		t.Fatalf("%d runner errors", errs.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(counts) != jobs {
		t.Fatalf("handled %d distinct jobs, want %d", len(counts), jobs)
	}
	for id, c := range counts {
		if !want[id] || c != 1 {
			t.Fatalf("job %s handled %d times", id, c)
		}
	}
	if s := q.Stats(); s != (Stats{Done: jobs}) {
		t.Fatalf("stats = %+v", s)
	}
}
