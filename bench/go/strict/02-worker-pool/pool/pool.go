// Package pool runs jobs on a bounded number of workers with an overall
// timeout and reports results in the original job order.
package pool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrTimeout is reported for every job that did not finish before the
// overall timeout passed.
var ErrTimeout = errors.New("pool: timeout")

// ErrPanic is reported for a job whose function panicked.
var ErrPanic = errors.New("pool: job panicked")

// ErrInvalidWorkers is returned by Run when workers is less than one.
var ErrInvalidWorkers = errors.New("pool: workers must be at least 1")

// ErrNilFunc is returned by Run when the job function is nil.
var ErrNilFunc = errors.New("pool: job function is nil")

// errNotRun is used for a job that never ran when no cancellation cause is
// available. It should not happen in practice.
var errNotRun = errors.New("pool: job was not run")

// Job is one unit of work.
type Job[T any] struct {
	ID    string
	Input T
}

// Func processes one job input. It must honor ctx cancellation, since Run
// waits for every started call to return.
type Func[T, R any] func(ctx context.Context, input T) (R, error)

// Result is a successful job outcome.
type Result[R any] struct {
	Index int // position of the job in the input slice
	JobID string
	Value R
}

// Failure is a failed job outcome.
type Failure struct {
	Index int // position of the job in the input slice
	JobID string
	Err   error
}

// Report is the outcome of a Run. Results and Failures are both sorted by
// the original job order.
type Report[R any] struct {
	Results  []Result[R]
	Failures []Failure
	Elapsed  time.Duration
}

// Clock abstracts time so tests can control it.
type Clock interface {
	Now() time.Time
	// AfterFunc calls f in its own goroutine after d and returns a function
	// that stops the timer.
	AfterFunc(d time.Duration, f func()) (stop func() bool)
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) AfterFunc(d time.Duration, f func()) func() bool {
	return time.AfterFunc(d, f).Stop
}

// Option configures Run.
type Option func(*config)

type config struct {
	clock Clock
}

// WithClock replaces the real clock, mainly for tests.
func WithClock(c Clock) Option {
	return func(cfg *config) {
		if c != nil {
			cfg.clock = c
		}
	}
}

type outcome[R any] struct {
	value R
	err   error
	done  bool
}

// Run processes every job with at most workers calls of f running at once.
// If timeout is greater than zero and passes before all jobs finish, the
// context passed to f is cancelled and every unfinished job is reported as a
// failure wrapping ErrTimeout. A timeout of zero or less means no timeout.
// Run returns only after every started call of f has returned.
func Run[T, R any](ctx context.Context, jobs []Job[T], workers int, timeout time.Duration, f Func[T, R], opts ...Option) (Report[R], error) {
	if workers < 1 {
		return Report[R]{}, ErrInvalidWorkers
	}
	if f == nil {
		return Report[R]{}, ErrNilFunc
	}
	cfg := config{clock: realClock{}}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	start := cfg.clock.Now()
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	if timeout > 0 {
		stop := cfg.clock.AfterFunc(timeout, func() { cancel(ErrTimeout) })
		defer stop()
	}

	outcomes := make([]outcome[R], len(jobs))
	indexes := make(chan int)
	var wg sync.WaitGroup
	for range min(workers, len(jobs)) {
		wg.Go(func() {
			for i := range indexes {
				outcomes[i] = runOne(ctx, jobs[i], f)
			}
		})
	}

feed:
	for i := range jobs {
		select {
		case indexes <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(indexes)
	wg.Wait()

	report := Report[R]{
		Results:  make([]Result[R], 0, len(jobs)),
		Failures: []Failure{},
	}
	for i, job := range jobs {
		o := outcomes[i]
		err := o.err
		if !o.done {
			err = context.Cause(ctx)
			if err == nil {
				err = errNotRun
			}
		}
		if err != nil {
			report.Failures = append(report.Failures, Failure{Index: i, JobID: job.ID, Err: err})
			continue
		}
		report.Results = append(report.Results, Result[R]{Index: i, JobID: job.ID, Value: o.value})
	}
	report.Elapsed = cfg.clock.Now().Sub(start)
	return report, nil
}

// runOne runs f for a single job, converting panics into errors and tagging
// errors that happened after cancellation with the cancellation cause.
func runOne[T, R any](ctx context.Context, job Job[T], f Func[T, R]) (o outcome[R]) {
	if ctx.Err() != nil {
		return outcome[R]{err: context.Cause(ctx), done: true}
	}
	defer func() {
		if r := recover(); r != nil {
			o = outcome[R]{err: fmt.Errorf("%w: %v", ErrPanic, r), done: true}
		}
	}()
	v, err := f(ctx, job.Input)
	if err != nil && ctx.Err() != nil {
		if cause := context.Cause(ctx); cause != nil && !errors.Is(err, cause) {
			err = fmt.Errorf("%w: %w", cause, err)
		}
	}
	return outcome[R]{value: v, err: err, done: true}
}
