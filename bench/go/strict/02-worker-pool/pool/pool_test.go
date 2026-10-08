package pool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// guard is a hang guard for tests that wait on goroutines. It is never
// reached when the code under test is correct.
const guard = 10 * time.Second

// fakeClock lets tests fire the timeout deterministically.
type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	pending []fakeTimer
}

type fakeTimer struct {
	at      time.Time
	f       func()
	stopped *atomic.Bool
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) AfterFunc(d time.Duration, f func()) func() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	stopped := &atomic.Bool{}
	c.pending = append(c.pending, fakeTimer{at: c.now.Add(d), f: f, stopped: stopped})
	return func() bool { return !stopped.Swap(true) }
}

// Advance moves time forward and runs every due timer synchronously.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	var due []fakeTimer
	var rest []fakeTimer
	for _, t := range c.pending {
		if !t.at.After(c.now) {
			due = append(due, t)
		} else {
			rest = append(rest, t)
		}
	}
	c.pending = rest
	c.mu.Unlock()
	for _, t := range due {
		if !t.stopped.Swap(true) {
			t.f()
		}
	}
}

// waitN receives n signals from ch or fails the test after the guard.
func waitN(t *testing.T, ch <-chan struct{}, n int) {
	t.Helper()
	deadline := time.NewTimer(guard)
	defer deadline.Stop()
	for range n {
		select {
		case <-ch:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %d signals", n)
		}
	}
}

func makeJobs(n int) []Job[int] {
	jobs := make([]Job[int], n)
	for i := range jobs {
		jobs[i] = Job[int]{ID: fmt.Sprintf("job-%d", i), Input: i}
	}
	return jobs
}

func TestOrderPreservedWhenLaterJobsFinishFirst(t *testing.T) {
	const n = 6
	jobs := makeJobs(n)
	// finished[i] is closed when job i completes. Job i waits for job i+1,
	// so jobs complete in reverse order.
	finished := make([]chan struct{}, n)
	for i := range finished {
		finished[i] = make(chan struct{})
	}
	var mu sync.Mutex
	var completion []int
	f := func(ctx context.Context, in int) (int, error) {
		if in+1 < n {
			select {
			case <-finished[in+1]:
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}
		mu.Lock()
		completion = append(completion, in)
		mu.Unlock()
		close(finished[in])
		return in * 10, nil
	}

	report, err := Run(context.Background(), jobs, n, guard, f)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Failures) != 0 {
		t.Fatalf("unexpected failures: %v", report.Failures)
	}
	if len(report.Results) != n {
		t.Fatalf("got %d results, want %d", len(report.Results), n)
	}
	for i, r := range report.Results {
		if r.Index != i || r.JobID != jobs[i].ID || r.Value != i*10 {
			t.Errorf("result %d = %+v, want index %d id %s value %d", i, r, i, jobs[i].ID, i*10)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(completion) != n || completion[0] != n-1 || completion[n-1] != 0 {
		t.Errorf("jobs did not finish in reverse order: %v", completion)
	}
}

func TestFailingJobDoesNotAffectOthers(t *testing.T) {
	jobs := makeJobs(5)
	boom := errors.New("boom")
	f := func(_ context.Context, in int) (int, error) {
		switch in {
		case 1:
			return 0, boom
		case 3:
			panic("kaboom")
		}
		return in + 100, nil
	}

	report, err := Run(context.Background(), jobs, 2, guard, f)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	wantResults := []int{0, 2, 4}
	if len(report.Results) != len(wantResults) {
		t.Fatalf("got results %+v, want indexes %v", report.Results, wantResults)
	}
	for i, idx := range wantResults {
		r := report.Results[i]
		if r.Index != idx || r.Value != idx+100 || r.JobID != jobs[idx].ID {
			t.Errorf("result %d = %+v, want index %d value %d", i, r, idx, idx+100)
		}
	}
	if len(report.Failures) != 2 {
		t.Fatalf("got failures %+v, want 2", report.Failures)
	}
	if f0 := report.Failures[0]; f0.JobID != "job-1" || !errors.Is(f0.Err, boom) {
		t.Errorf("failure 0 = %+v, want job-1 with boom", f0)
	}
	if f1 := report.Failures[1]; f1.JobID != "job-3" || !errors.Is(f1.Err, ErrPanic) {
		t.Errorf("failure 1 = %+v, want job-3 with ErrPanic", f1)
	}
}

func TestTimeoutCancelsSlowJobs(t *testing.T) {
	clock := newFakeClock()
	const timeout = 30 * time.Second
	jobs := []Job[string]{
		{ID: "fast", Input: "fast"},
		{ID: "slow-1", Input: "slow"},
		{ID: "slow-2", Input: "slow"},
		{ID: "never-started", Input: "slow"},
	}
	started := make(chan struct{}, len(jobs))
	var cancelledCount atomic.Int32
	f := func(ctx context.Context, in string) (string, error) {
		if in == "fast" {
			return "done", nil
		}
		started <- struct{}{}
		<-ctx.Done()
		cancelledCount.Add(1)
		return "", ctx.Err()
	}

	type out struct {
		report Report[string]
		err    error
	}
	done := make(chan out, 1)
	go func() {
		r, err := Run(context.Background(), jobs, 2, timeout, f, WithClock(clock))
		done <- out{r, err}
	}()

	// With 2 workers, "fast" and "slow-1" start; "slow-2" starts once
	// "fast" returns. Both workers are then blocked, so "never-started"
	// is still queued when the timeout fires.
	waitN(t, started, 2)
	clock.Advance(timeout)

	var got out
	select {
	case got = <-done:
	case <-time.After(guard):
		t.Fatal("Run did not return after timeout")
	}
	if got.err != nil {
		t.Fatalf("Run: %v", got.err)
	}
	report := got.report
	if len(report.Results) != 1 || report.Results[0].JobID != "fast" || report.Results[0].Value != "done" {
		t.Errorf("results = %+v, want only fast", report.Results)
	}
	wantFailed := []string{"slow-1", "slow-2", "never-started"}
	if len(report.Failures) != len(wantFailed) {
		t.Fatalf("failures = %+v, want %v", report.Failures, wantFailed)
	}
	for i, id := range wantFailed {
		fl := report.Failures[i]
		if fl.JobID != id || fl.Index != i+1 {
			t.Errorf("failure %d = %+v, want %s at index %d", i, fl, id, i+1)
		}
		if !errors.Is(fl.Err, ErrTimeout) {
			t.Errorf("failure %s error = %v, want ErrTimeout", id, fl.Err)
		}
	}
	if c := cancelledCount.Load(); c != 2 {
		t.Errorf("cancelled running jobs = %d, want 2", c)
	}
	if report.Elapsed != timeout {
		t.Errorf("elapsed = %v, want %v", report.Elapsed, timeout)
	}
}

func TestNeverMoreThanWorkersRunning(t *testing.T) {
	const (
		n       = 40
		workers = 4
	)
	jobs := makeJobs(n)
	var active, peak atomic.Int32
	started := make(chan struct{}, n)
	release := make(chan struct{})
	f := func(ctx context.Context, in int) (int, error) {
		cur := active.Add(1)
		defer active.Add(-1)
		for {
			p := peak.Load()
			if cur <= p || peak.CompareAndSwap(p, cur) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
		return in, nil
	}

	done := make(chan Report[int], 1)
	errc := make(chan error, 1)
	go func() {
		r, err := Run(context.Background(), jobs, workers, guard, f)
		errc <- err
		done <- r
	}()

	// Wait until the pool is saturated, then let everything finish.
	waitN(t, started, workers)
	if a := active.Load(); a != workers {
		t.Errorf("active after saturation = %d, want %d", a, workers)
	}
	close(release)

	if err := <-errc; err != nil {
		t.Fatalf("Run: %v", err)
	}
	report := <-done
	if len(report.Results) != n || len(report.Failures) != 0 {
		t.Fatalf("got %d results and %d failures", len(report.Results), len(report.Failures))
	}
	if p := peak.Load(); p > workers {
		t.Errorf("peak concurrency = %d, want at most %d", p, workers)
	}
	if p := peak.Load(); p != workers {
		t.Errorf("peak concurrency = %d, want exactly %d", p, workers)
	}
}

func TestInvalidArguments(t *testing.T) {
	f := func(_ context.Context, in int) (int, error) { return in, nil }
	if _, err := Run(context.Background(), makeJobs(1), 0, time.Second, f); !errors.Is(err, ErrInvalidWorkers) {
		t.Errorf("workers=0 error = %v, want ErrInvalidWorkers", err)
	}
	if _, err := Run[int, int](context.Background(), makeJobs(1), 1, time.Second, nil); !errors.Is(err, ErrNilFunc) {
		t.Errorf("nil f error = %v, want ErrNilFunc", err)
	}
}

func TestEmptyJobs(t *testing.T) {
	f := func(_ context.Context, in int) (int, error) { return in, nil }
	report, err := Run(context.Background(), nil, 3, time.Second, f)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Results) != 0 || len(report.Failures) != 0 {
		t.Errorf("report = %+v, want empty", report)
	}
}
