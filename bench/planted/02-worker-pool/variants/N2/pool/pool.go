// Package pool runs jobs on a bounded number of goroutines with an overall
// timeout and reports results in the original job order.
package pool

import (
	"context"
	"errors"
	"sort"
	"time"
)

// ErrTimeout is reported for every job that did not finish before the
// overall timeout passed.
var ErrTimeout = errors.New("pool: overall timeout exceeded")

// Job is one unit of work. ID identifies the job in failure reports.
type Job[T any] struct {
	ID    string
	Input T
}

// Result is the successful output of one job.
type Result[R any] struct {
	Index int // position of the job in the input slice
	JobID string
	Value R
}

// Failure is a job that returned an error or was cancelled.
type Failure struct {
	Index int // position of the job in the input slice
	JobID string
	Err   error
}

// Report is the outcome of a Run.
type Report[R any] struct {
	Results  []Result[R] // successful results, in original job order
	Failures []Failure   // failed jobs, in original job order
	Elapsed  time.Duration
}

// Clock abstracts time so tests can control the timeout.
type Clock interface {
	Now() time.Time
	// NewTimer returns a channel that fires after d and a stop function.
	NewTimer(d time.Duration) (<-chan time.Time, func() bool)
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) NewTimer(d time.Duration) (<-chan time.Time, func() bool) {
	t := time.NewTimer(d)
	return t.C, t.Stop
}

// RealClock is the wall clock.
var RealClock Clock = realClock{}

// Options holds optional settings for RunWithOptions.
type Options struct {
	Clock Clock // nil means RealClock
}

// Run processes every job with f, with at most workers calls of f running at
// once. A timeout of zero or less means no overall timeout. workers below 1
// is treated as 1.
func Run[T, R any](ctx context.Context, jobs []Job[T], workers int, timeout time.Duration, f func(context.Context, T) (R, error)) Report[R] {
	return RunWithOptions(ctx, jobs, workers, timeout, f, Options{})
}

type outcome[R any] struct {
	index int
	value R
	err   error
}

// RunWithOptions is Run with an injectable clock.
//
// When the timeout passes (or ctx is cancelled), the context passed to f is
// cancelled, no new jobs are started, and every job that has not delivered an
// outcome yet is reported as a failure wrapping ErrTimeout (or the parent
// context's cause). RunWithOptions returns at that point and does not wait
// for calls of f that ignore their context.
func RunWithOptions[T, R any](ctx context.Context, jobs []Job[T], workers int, timeout time.Duration, f func(context.Context, T) (R, error), opts Options) Report[R] {
	clock := opts.Clock
	if clock == nil {
		clock = RealClock
	}
	if workers < 1 {
		workers = 1
	}
	start := clock.Now()

	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	var timerC <-chan time.Time
	if timeout > 0 {
		c, stop := clock.NewTimer(timeout)
		defer stop()
		timerC = c
	}

	// Buffered so senders never block, even after the collector stops.
	outcomes := make(chan outcome[R], len(jobs))
	sem := make(chan struct{}, workers)

	go func() {
		for i := range jobs {
			select {
			case sem <- struct{}{}:
			case <-runCtx.Done():
				return
			}
			if runCtx.Err() != nil {
				<-sem
				return
			}
			go func(i int) {
				v, err := f(runCtx, jobs[i].Input)
				// Deliver the outcome before freeing the slot.
				outcomes <- outcome[R]{index: i, value: v, err: err}
				<-sem
			}(i)
		}
	}()

	done := make([]bool, len(jobs))
	var report Report[R]
	record := func(o outcome[R], cancelled bool) {
		done[o.index] = true
		id := jobs[o.index].ID
		if o.err == nil {
			report.Results = append(report.Results, Result[R]{Index: o.index, JobID: id, Value: o.value})
			return
		}
		err := o.err
		if cancelled && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			err = cancelErr(runCtx, err)
		}
		report.Failures = append(report.Failures, Failure{Index: o.index, JobID: id, Err: err})
	}

	received := 0
	cancelled := false
loop:
	for received < len(jobs) {
		select {
		case o := <-outcomes:
			record(o, false)
			received++
		case <-timerC:
			cancel(context.DeadlineExceeded)
			cancelled = true
			break loop
		case <-ctx.Done():
			cancel(context.Cause(ctx))
			cancelled = true
			break loop
		}
	}

	if cancelled {
		// Keep outcomes that were already delivered.
	drain:
		for {
			select {
			case o := <-outcomes:
				record(o, true)
			default:
				break drain
			}
		}
		for i := range jobs {
			if !done[i] {
				report.Failures = append(report.Failures, Failure{
					Index: i,
					JobID: jobs[i].ID,
					Err:   cancelErr(runCtx, nil),
				})
			}
		}
	}

	sort.Slice(report.Results, func(a, b int) bool { return report.Results[a].Index < report.Results[b].Index })
	sort.Slice(report.Failures, func(a, b int) bool { return report.Failures[a].Index < report.Failures[b].Index })
	report.Elapsed = clock.Now().Sub(start)
	return report
}

// cancelErr builds the error for a job cut off by cancellation. The result
// matches both the cancellation cause (for example ErrTimeout) and, when
// given, the job's own error.
func cancelErr(ctx context.Context, jobErr error) error {
	cause := context.Cause(ctx)
	if cause == nil {
		cause = context.Canceled
	}
	if jobErr == nil {
		return cause
	}
	return errors.Join(cause, jobErr)
}
