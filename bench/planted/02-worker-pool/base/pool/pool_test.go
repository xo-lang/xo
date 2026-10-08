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

// fakeClock lets the test decide when the overall timeout fires.
type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	timer chan time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(1000, 0), timer: make(chan time.Time, 1)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) NewTimer(time.Duration) (<-chan time.Time, func() bool) {
	return c.timer, func() bool { return true }
}

func (c *fakeClock) fire(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	now := c.now
	c.mu.Unlock()
	c.timer <- now
}

func makeJobs(n int) []Job[int] {
	jobs := make([]Job[int], n)
	for i := range jobs {
		jobs[i] = Job[int]{ID: fmt.Sprintf("job-%d", i), Input: i}
	}
	return jobs
}

func values(r []Result[int]) []int {
	out := make([]int, len(r))
	for i, x := range r {
		out[i] = x.Value
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestResultsKeepOriginalOrder(t *testing.T) {
	const n = 5
	jobs := makeJobs(n)
	// Job i waits until job i+1 has finished, so jobs finish in reverse order.
	finished := make([]chan struct{}, n)
	for i := range finished {
		finished[i] = make(chan struct{})
	}
	var mu sync.Mutex
	var finishOrder []int

	f := func(ctx context.Context, in int) (int, error) {
		if in+1 < n {
			<-finished[in+1]
		}
		mu.Lock()
		finishOrder = append(finishOrder, in)
		mu.Unlock()
		close(finished[in])
		return in * 10, nil
	}

	rep := RunWithOptions(context.Background(), jobs, n, time.Minute, f, Options{Clock: newFakeClock()})

	if !equalInts(finishOrder, []int{4, 3, 2, 1, 0}) {
		t.Fatalf("finish order = %v, want reverse order", finishOrder)
	}
	if len(rep.Failures) != 0 {
		t.Fatalf("unexpected failures: %v", rep.Failures)
	}
	if got, want := values(rep.Results), []int{0, 10, 20, 30, 40}; !equalInts(got, want) {
		t.Fatalf("results = %v, want %v", got, want)
	}
	for i, r := range rep.Results {
		if r.JobID != jobs[i].ID || r.Index != i {
			t.Fatalf("result %d has id %q index %d", i, r.JobID, r.Index)
		}
	}
}

func TestFailingJobDoesNotAffectOthers(t *testing.T) {
	jobs := makeJobs(6)
	boom := errors.New("boom")
	f := func(ctx context.Context, in int) (int, error) {
		if in == 2 {
			return 0, boom
		}
		return in + 100, nil
	}

	rep := Run(context.Background(), jobs, 3, time.Minute, f)

	if got, want := values(rep.Results), []int{100, 101, 103, 104, 105}; !equalInts(got, want) {
		t.Fatalf("results = %v, want %v", got, want)
	}
	if len(rep.Failures) != 1 {
		t.Fatalf("failures = %v, want exactly one", rep.Failures)
	}
	fl := rep.Failures[0]
	if fl.JobID != "job-2" || !errors.Is(fl.Err, boom) {
		t.Fatalf("failure = %+v, want job-2 with boom", fl)
	}
	if errors.Is(fl.Err, ErrTimeout) {
		t.Fatalf("plain failure must not be a timeout: %v", fl.Err)
	}
}

func TestTimeoutCancelsSlowJobs(t *testing.T) {
	clock := newFakeClock()
	jobs := makeJobs(6)
	// Jobs 0 and 1 are fast. Jobs 2, 3, 4 are slow. Job 5 never gets a slot.
	// Job 4 ignores its context entirely.
	slowStarted := make(chan int, 3)
	ignoreCtx := make(chan struct{})
	t.Cleanup(func() { close(ignoreCtx) })
	var calls [6]atomic.Int32

	f := func(ctx context.Context, in int) (int, error) {
		calls[in].Add(1)
		switch in {
		case 0, 1:
			return in, nil
		case 2, 3:
			slowStarted <- in
			<-ctx.Done()
			return 0, ctx.Err()
		case 4:
			slowStarted <- in
			<-ignoreCtx
			return 4, nil
		default:
			return in, nil
		}
	}

	type res struct{ rep Report[int] }
	out := make(chan res, 1)
	go func() {
		out <- res{RunWithOptions(context.Background(), jobs, 3, 5*time.Second, f, Options{Clock: clock})}
	}()

	for range 3 {
		select {
		case <-slowStarted:
		case <-time.After(10 * time.Second):
			t.Fatal("slow jobs did not start")
		}
	}
	clock.fire(5 * time.Second)

	var rep Report[int]
	select {
	case r := <-out:
		rep = r.rep
	case <-time.After(10 * time.Second):
		t.Fatal("pool did not return after timeout")
	}

	if got, want := values(rep.Results), []int{0, 1}; !equalInts(got, want) {
		t.Fatalf("results = %v, want %v", got, want)
	}
	if len(rep.Failures) != 4 {
		t.Fatalf("failures = %+v, want 4", rep.Failures)
	}
	for i, fl := range rep.Failures {
		wantID := fmt.Sprintf("job-%d", i+2)
		if fl.JobID != wantID {
			t.Fatalf("failure %d id = %q, want %q", i, fl.JobID, wantID)
		}
		if !errors.Is(fl.Err, ErrTimeout) {
			t.Fatalf("failure %s err = %v, want ErrTimeout", fl.JobID, fl.Err)
		}
	}
	if calls[5].Load() != 0 {
		t.Fatalf("job 5 should never have started")
	}
	if rep.Elapsed != 5*time.Second {
		t.Fatalf("elapsed = %v, want 5s from fake clock", rep.Elapsed)
	}
}

func TestNeverMoreThanWorkers(t *testing.T) {
	const n, workers = 40, 4
	jobs := makeJobs(n)
	var running, maxRunning atomic.Int32
	started := make(chan int)
	release := make(chan struct{})

	f := func(ctx context.Context, in int) (int, error) {
		cur := running.Add(1)
		for {
			m := maxRunning.Load()
			if cur <= m || maxRunning.CompareAndSwap(m, cur) {
				break
			}
		}
		started <- in
		<-release
		running.Add(-1)
		return in, nil
	}

	out := make(chan Report[int], 1)
	go func() {
		out <- RunWithOptions(context.Background(), jobs, workers, time.Minute, f, Options{Clock: newFakeClock()})
	}()

	timeout := time.After(20 * time.Second)
	recv := func() {
		select {
		case <-started:
		case <-timeout:
			t.Fatal("job did not start")
		}
	}
	// Fill every slot, then keep exactly one release per new start.
	for range workers {
		recv()
	}
	for range n - workers {
		release <- struct{}{}
		recv()
	}
	for range workers {
		release <- struct{}{}
	}

	rep := <-out
	if len(rep.Results) != n || len(rep.Failures) != 0 {
		t.Fatalf("got %d results and %d failures", len(rep.Results), len(rep.Failures))
	}
	if got := maxRunning.Load(); got != workers {
		t.Fatalf("max concurrent = %d, want exactly %d", got, workers)
	}
}

func TestParentCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	f := func(ctx context.Context, in int) (int, error) {
		close(started)
		<-ctx.Done()
		return 0, ctx.Err()
	}
	out := make(chan Report[int], 1)
	go func() { out <- Run(ctx, makeJobs(1), 1, 0, f) }()
	<-started
	cancel()
	rep := <-out
	if len(rep.Failures) != 1 || !errors.Is(rep.Failures[0].Err, context.Canceled) {
		t.Fatalf("failures = %+v, want one context.Canceled", rep.Failures)
	}
	if errors.Is(rep.Failures[0].Err, ErrTimeout) {
		t.Fatalf("parent cancel must not be reported as timeout")
	}
}

func TestEmptyJobs(t *testing.T) {
	rep := Run(context.Background(), nil, 4, time.Second, func(context.Context, int) (int, error) { return 0, nil })
	if len(rep.Results) != 0 || len(rep.Failures) != 0 {
		t.Fatalf("unexpected report %+v", rep)
	}
}
