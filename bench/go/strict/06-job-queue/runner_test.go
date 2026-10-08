package jobqueue

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// Runner tests run inside a synctest bubble, so time.Now, timers, tickers and
// time.Sleep use a fake clock that advances only when every goroutine in the
// bubble is blocked. The queue uses the default clock, which is therefore
// fake as well.

func newTestRunner(t *testing.T, q *Queue, cfg RunnerConfig) *Runner {
	t.Helper()
	if cfg.OnError == nil {
		cfg.OnError = func(jobID string, err error) { t.Logf("runner error for %s: %v", jobID, err) }
	}
	r, err := NewRunner(q, cfg)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	return r
}

func TestRunnerHeartbeatPreventsRedelivery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := New(Config{})
		var calls atomic.Int32
		r := newTestRunner(t, q, RunnerConfig{
			Workers:       3,
			LeaseDuration: 3 * time.Second,
			PollInterval:  100 * time.Millisecond,
			Handler: func(ctx context.Context, _ Job) error {
				calls.Add(1)
				select {
				case <-time.After(20 * time.Second): // far longer than the lease
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		})
		id, _ := q.Enqueue([]byte("long"), 0, "")
		r.Start()
		time.Sleep(30 * time.Second)
		if forced := r.Shutdown(time.Second); forced {
			t.Fatal("shutdown was forced; handler should have finished")
		}
		if n := calls.Load(); n != 1 {
			t.Fatalf("handler called %d times, want 1", n)
		}
		j := mustGet(t, q, id)
		if j.State != StateDone || j.Attempts != 1 {
			t.Fatalf("job = %+v, want done after 1 attempt", j)
		}
	})
}

func TestRunnerHandlerErrorNacks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := New(Config{MaxAttempts: 2})
		var calls atomic.Int32
		r := newTestRunner(t, q, RunnerConfig{
			Workers:       1,
			LeaseDuration: 3 * time.Second,
			PollInterval:  100 * time.Millisecond,
			Handler: func(context.Context, Job) error {
				calls.Add(1)
				return errors.New("handler failed")
			},
		})
		id, _ := q.Enqueue(nil, 0, "")
		r.Start()
		time.Sleep(10 * time.Second)
		r.Shutdown(time.Second)
		if n := calls.Load(); n != 2 {
			t.Fatalf("handler called %d times, want 2", n)
		}
		j := mustGet(t, q, id)
		if j.State != StateDead || j.LastError != "handler failed" {
			t.Fatalf("job = %+v", j)
		}
	})
}

func TestRunnerShutdownNacksInFlightAfterGrace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := New(Config{})
		started := make(chan struct{}, 2)
		r := newTestRunner(t, q, RunnerConfig{
			Workers:       3,
			LeaseDuration: 10 * time.Second,
			PollInterval:  100 * time.Millisecond,
			Handler: func(ctx context.Context, _ Job) error {
				started <- struct{}{}
				<-ctx.Done()
				return ctx.Err()
			},
		})
		id1, _ := q.Enqueue([]byte("1"), 0, "")
		id2, _ := q.Enqueue([]byte("2"), 0, "")
		r.Start()
		<-started
		<-started

		t0 := time.Now()
		forced := r.Shutdown(5 * time.Second)
		elapsed := time.Since(t0)
		if !forced {
			t.Fatal("shutdown should have been forced")
		}
		if elapsed < 5*time.Second || elapsed > 6*time.Second {
			t.Fatalf("shutdown took %v, want about the 5s grace period", elapsed)
		}
		if n := r.Running(); n != 0 {
			t.Fatalf("%d workers still running after shutdown", n)
		}
		for _, id := range []string{id1, id2} {
			j := mustGet(t, q, id)
			if j.State != StateQueued || j.Attempts != 1 ||
				!strings.Contains(j.LastError, context.Canceled.Error()) {
				t.Fatalf("job %s = %+v, want nacked with cancellation", id, j)
			}
		}
		if s := q.Stats(); s != (Stats{Queued: 2}) {
			t.Fatalf("stats = %+v", s)
		}
		// Shutdown is idempotent.
		if r.Shutdown(time.Second) {
			t.Fatal("second shutdown reported forced")
		}
	})
}

func TestRunnerShutdownWaitsForHandlersWithinGrace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := New(Config{})
		started := make(chan struct{}, 1)
		r := newTestRunner(t, q, RunnerConfig{
			Workers:       2,
			LeaseDuration: 3 * time.Second,
			Handler: func(ctx context.Context, _ Job) error {
				started <- struct{}{}
				select {
				case <-time.After(2 * time.Second):
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		})
		id, _ := q.Enqueue(nil, 0, "")
		r.Start()
		<-started
		t0 := time.Now()
		if forced := r.Shutdown(10 * time.Second); forced {
			t.Fatal("shutdown forced, want graceful")
		}
		if elapsed := time.Since(t0); elapsed != 2*time.Second {
			t.Fatalf("shutdown took %v, want 2s", elapsed)
		}
		if r.Running() != 0 {
			t.Fatal("workers still running")
		}
		if j := mustGet(t, q, id); j.State != StateDone {
			t.Fatalf("job state = %v, want done", j.State)
		}
	})
}

func TestRunnerManyWorkersExactlyOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := New(Config{})
		const jobs = 1000
		var mu sync.Mutex
		counts := make(map[string]int)
		var runnerErrs atomic.Int32
		r := newTestRunner(t, q, RunnerConfig{
			Workers:       50,
			LeaseDuration: 30 * time.Second,
			PollInterval:  10 * time.Millisecond,
			OnError: func(jobID string, err error) {
				runnerErrs.Add(1)
				t.Errorf("runner error for %s: %v", jobID, err)
			},
			Handler: func(context.Context, Job) error {
				time.Sleep(time.Millisecond)
				return nil
			},
		})
		// Wrap the handler to count deliveries per job.
		inner := r.cfg.Handler
		r.cfg.Handler = func(ctx context.Context, j Job) error {
			mu.Lock()
			counts[j.ID]++
			mu.Unlock()
			return inner(ctx, j)
		}
		ids := make([]string, 0, jobs)
		for i := range jobs {
			id, _ := q.Enqueue([]byte{byte(i), byte(i >> 8)}, i%5, "")
			ids = append(ids, id)
		}
		r.Start()
		start := time.Now()
		for q.Stats().Done < jobs {
			if time.Since(start) > time.Hour {
				t.Fatalf("timed out; stats = %+v", q.Stats())
			}
			time.Sleep(10 * time.Millisecond)
		}
		if forced := r.Shutdown(time.Second); forced {
			t.Fatal("shutdown forced")
		}
		if r.Running() != 0 {
			t.Fatal("workers still running")
		}
		mu.Lock()
		defer mu.Unlock()
		if len(counts) != jobs {
			t.Fatalf("handled %d distinct jobs, want %d", len(counts), jobs)
		}
		for _, id := range ids {
			if counts[id] != 1 {
				t.Fatalf("job %s handled %d times", id, counts[id])
			}
		}
		if s := q.Stats(); s != (Stats{Done: jobs}) {
			t.Fatalf("stats = %+v", s)
		}
		if runnerErrs.Load() != 0 {
			t.Fatal("runner reported errors")
		}
	})
}

func TestNewRunnerValidation(t *testing.T) {
	q := New(Config{})
	h := func(context.Context, Job) error { return nil }
	bad := []RunnerConfig{
		{Workers: 0, LeaseDuration: time.Second, Handler: h},
		{Workers: 1, LeaseDuration: 0, Handler: h},
		{Workers: 1, LeaseDuration: time.Second},
	}
	for i, cfg := range bad {
		if _, err := NewRunner(q, cfg); err == nil {
			t.Errorf("config %d: expected error", i)
		}
	}
	if _, err := NewRunner(nil, RunnerConfig{Workers: 1, LeaseDuration: time.Second, Handler: h}); err == nil {
		t.Error("nil queue: expected error")
	}
}
