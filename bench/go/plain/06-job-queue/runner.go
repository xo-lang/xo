package jobqueue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Handler processes one job. It should return promptly once ctx is done.
type Handler func(ctx context.Context, job Job) error

// RunnerConfig configures a Runner.
type RunnerConfig struct {
	Workers       int           // number of workers, default 1
	LeaseDuration time.Duration // default 30s; heartbeat every LeaseDuration/3
	PollInterval  time.Duration // sleep when the queue is empty, default 50ms
	Handler       Handler
	// OnError, if set, is called for ack, nack, and heartbeat failures
	// (for example a lost lease). It must be safe for concurrent use.
	OnError func(job Job, err error)
}

// Runner runs workers that lease jobs from a Queue and call a Handler.
type Runner struct {
	q   *Queue
	cfg RunnerConfig

	stop       chan struct{}
	stopOnce   sync.Once
	startOnce  sync.Once
	hardCtx    context.Context
	hardCancel context.CancelFunc
	wg         sync.WaitGroup
	running    atomic.Int64
}

// ErrShutdown is the cancellation cause seen by handlers cancelled at the end
// of the grace period.
var ErrShutdown = errors.New("jobqueue: runner shut down")

// NewRunner returns a Runner. Call Start to launch workers.
func NewRunner(q *Queue, cfg RunnerConfig) *Runner {
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = DefaultLeaseDuration
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 50 * time.Millisecond
	}
	if cfg.Handler == nil {
		panic("jobqueue: RunnerConfig.Handler is nil")
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	return &Runner{
		q:          q,
		cfg:        cfg,
		stop:       make(chan struct{}),
		hardCtx:    ctx,
		hardCancel: func() { cancel(ErrShutdown) },
	}
}

// Start launches the workers. Calling it more than once has no effect.
func (r *Runner) Start() {
	r.startOnce.Do(func() {
		for i := range r.cfg.Workers {
			r.wg.Add(1)
			r.running.Add(1)
			go r.worker(fmt.Sprintf("worker-%d", i))
		}
	})
}

// Running reports how many worker goroutines are still running.
func (r *Runner) Running() int { return int(r.running.Load()) }

// Shutdown stops leasing new jobs, waits up to grace for running handlers to
// finish, then cancels the remaining handlers (their jobs are nacked) and
// waits for every worker to exit. It reports whether handlers had to be
// cancelled. Handlers that ignore cancellation will block Shutdown.
func (r *Runner) Shutdown(grace time.Duration) (forced bool) {
	r.stopOnce.Do(func() { close(r.stop) })
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	t := time.NewTimer(grace)
	defer t.Stop()
	select {
	case <-done:
		r.hardCancel()
		return false
	case <-t.C:
	}
	r.hardCancel()
	<-done
	return true
}

func (r *Runner) worker(id string) {
	defer r.wg.Done()
	defer r.running.Add(-1)
	idle := time.NewTimer(0)
	if !idle.Stop() {
		<-idle.C
	}
	defer idle.Stop()
	for {
		select {
		case <-r.stop:
			return
		default:
		}
		j, token, ok := r.q.Lease(id, r.cfg.LeaseDuration)
		if !ok {
			idle.Reset(r.cfg.PollInterval)
			select {
			case <-r.stop:
				return
			case <-idle.C:
			}
			continue
		}
		r.process(j, token)
	}
}

func (r *Runner) report(j Job, err error) {
	if r.cfg.OnError != nil {
		r.cfg.OnError(j, err)
	}
}

func (r *Runner) process(j Job, token string) {
	ctx, cancel := context.WithCancelCause(r.hardCtx)
	defer cancel(nil)

	interval := max(r.cfg.LeaseDuration/3, time.Millisecond)
	hbStop := make(chan struct{})
	hbDone := make(chan struct{})
	go func() {
		defer close(hbDone)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-hbStop:
				return
			case <-t.C:
				if err := r.q.Extend(j.ID, token, r.cfg.LeaseDuration); err != nil {
					err = fmt.Errorf("heartbeat: %w", err)
					r.report(j, err)
					cancel(err)
					return
				}
			}
		}
	}()

	err := r.call(ctx, j)
	close(hbStop)
	<-hbDone

	if err == nil && r.hardCtx.Err() != nil {
		// Cancelled at shutdown: always nack, even if the handler swallowed
		// the cancellation.
		err = context.Cause(r.hardCtx)
	}
	if err != nil {
		if nerr := r.q.Nack(j.ID, token, err.Error()); nerr != nil {
			r.report(j, fmt.Errorf("nack: %w", nerr))
		}
		return
	}
	if aerr := r.q.Ack(j.ID, token); aerr != nil {
		r.report(j, fmt.Errorf("ack: %w", aerr))
	}
}

func (r *Runner) call(ctx context.Context, j Job) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("handler panic: %v", p)
		}
	}()
	return r.cfg.Handler(ctx, j)
}
