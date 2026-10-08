package jobqueue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Handler processes one job. It should return promptly when ctx is cancelled.
type Handler func(ctx context.Context, job Job) error

// RunnerConfig configures a Runner.
type RunnerConfig struct {
	// Workers is the number of concurrent workers (at least 1).
	Workers int
	// LeaseDuration is the lease length; the lease is extended every
	// LeaseDuration / 3 while the handler runs.
	LeaseDuration time.Duration
	// PollInterval is how long an idle worker waits before leasing again.
	// Defaults to 100ms.
	PollInterval time.Duration
	// Handler processes jobs. Required.
	Handler Handler
	// OnError, if set, receives errors the runner cannot otherwise surface
	// (failed ack, nack, or heartbeat).
	OnError func(jobID string, err error)
}

// Runner runs workers that lease jobs from a Queue and call a Handler.
type Runner struct {
	q         *Queue
	cfg       RunnerConfig
	stop      chan struct{}
	stopOnce  sync.Once
	startOnce sync.Once
	baseCtx   context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	running   atomic.Int64
}

// NewRunner validates cfg and returns a Runner that is not yet started.
func NewRunner(q *Queue, cfg RunnerConfig) (*Runner, error) {
	if q == nil {
		return nil, errors.New("jobqueue: nil queue")
	}
	if cfg.Workers < 1 {
		return nil, errors.New("jobqueue: Workers must be at least 1")
	}
	if cfg.LeaseDuration/3 <= 0 {
		return nil, errors.New("jobqueue: LeaseDuration too small")
	}
	if cfg.Handler == nil {
		return nil, errors.New("jobqueue: nil Handler")
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 100 * time.Millisecond
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Runner{
		q:       q,
		cfg:     cfg,
		stop:    make(chan struct{}),
		baseCtx: ctx,
		cancel:  cancel,
	}, nil
}

// Start launches the workers. Calling it more than once has no effect.
func (r *Runner) Start() {
	r.startOnce.Do(func() {
		for i := range r.cfg.Workers {
			id := fmt.Sprintf("worker-%d", i)
			r.running.Add(1)
			r.wg.Go(func() { r.work(id) })
		}
	})
}

// Running reports how many workers have not yet exited.
func (r *Runner) Running() int {
	return int(r.running.Load())
}

// Shutdown stops leasing, waits up to grace for running handlers, then
// cancels their contexts and waits for every worker to exit. It reports
// whether the grace period expired and handlers were cancelled.
func (r *Runner) Shutdown(grace time.Duration) (forced bool) {
	r.stopOnce.Do(func() { close(r.stop) })
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		forced = true
		r.cancel()
		<-done
	}
	r.cancel()
	return forced
}

func (r *Runner) report(jobID string, err error) {
	if r.cfg.OnError != nil {
		r.cfg.OnError(jobID, err)
	}
}

func (r *Runner) work(workerID string) {
	defer r.running.Add(-1)
	for {
		select {
		case <-r.stop:
			return
		default:
		}
		l, ok, err := r.q.Lease(workerID, r.cfg.LeaseDuration)
		if err != nil {
			r.report("", fmt.Errorf("lease: %w", err))
			return
		}
		if !ok {
			select {
			case <-r.stop:
				return
			case <-time.After(r.cfg.PollInterval):
			}
			continue
		}
		r.process(l)
	}
}

func (r *Runner) callHandler(ctx context.Context, job Job) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("handler panic: %v", p)
		}
	}()
	return r.cfg.Handler(ctx, job)
}

func (r *Runner) process(l Leased) {
	hctx, cancel := context.WithCancel(r.baseCtx)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- r.callHandler(hctx, l.Job) }()

	ticker := time.NewTicker(r.cfg.LeaseDuration / 3)
	defer ticker.Stop()

	lost := false
	var herr error
wait:
	for {
		select {
		case herr = <-done:
			break wait
		case <-ticker.C:
			if lost {
				continue
			}
			if err := r.q.Extend(l.Job.ID, l.Token, r.cfg.LeaseDuration); err != nil {
				// The lease is gone; another worker may own the job now.
				lost = true
				cancel()
				r.report(l.Job.ID, fmt.Errorf("heartbeat: %w", err))
			}
		}
	}
	if lost {
		return
	}
	if herr != nil {
		if err := r.q.Nack(l.Job.ID, l.Token, herr.Error()); err != nil {
			r.report(l.Job.ID, fmt.Errorf("nack: %w", err))
		}
		return
	}
	if err := r.q.Ack(l.Job.ID, l.Token); err != nil {
		r.report(l.Job.ID, fmt.Errorf("ack: %w", err))
	}
}
