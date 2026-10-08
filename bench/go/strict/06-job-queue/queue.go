// Package jobqueue implements an in memory job queue with leases, retries,
// dead lettering, a worker runner, and a small HTTP API.
package jobqueue

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// State is the lifecycle state of a job.
type State int

// Job states.
const (
	StateQueued State = iota
	StateLeased
	StateDone
	StateDead
)

// String returns the lowercase name of the state.
func (s State) String() string {
	switch s {
	case StateQueued:
		return "queued"
	case StateLeased:
		return "leased"
	case StateDone:
		return "done"
	case StateDead:
		return "dead"
	}
	return fmt.Sprintf("State(%d)", int(s))
}

// Errors returned by the queue.
var (
	ErrNotFound        = errors.New("job not found")
	ErrStaleToken      = errors.New("lease token is not the current lease")
	ErrNotDead         = errors.New("job is not dead")
	ErrInvalidDuration = errors.New("lease duration must be positive")
)

// Tunables.
const (
	DefaultMaxAttempts = 5
	IdempotencyWindow  = 24 * time.Hour
	MaxBackoff         = 30 * time.Second
)

// Clock supplies the current time. Tests inject a fake one.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// Backoff returns the delay before a job becomes visible again after its
// attempts-th failed attempt: min(30s, 1s * 2^(attempts-1)).
func Backoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 6 { // 2^5 s = 32 s already exceeds the cap
		return MaxBackoff
	}
	d := time.Second << (attempts - 1)
	if d > MaxBackoff {
		return MaxBackoff
	}
	return d
}

// Job is an immutable snapshot of a job.
type Job struct {
	ID         string
	Payload    []byte
	Priority   int
	State      State
	Attempts   int
	LastError  string
	WorkerID   string
	EnqueuedAt time.Time
}

// Leased is a job together with the token of the lease that holds it.
type Leased struct {
	Job   Job
	Token string
}

// Stats holds per state counts.
type Stats struct {
	Queued int `json:"queued"`
	Leased int `json:"leased"`
	Done   int `json:"done"`
	Dead   int `json:"dead"`
}

// Config configures a Queue. Zero values select defaults.
type Config struct {
	Clock       Clock
	MaxAttempts int
}

type record struct {
	id           string
	seq          uint64
	payload      []byte
	priority     int
	state        State
	attempts     int
	lastErr      string
	enqueuedAt   time.Time
	visibleAt    time.Time
	leaseExpires time.Time
	token        string
	workerID     string
}

func (r *record) snapshot() Job {
	return Job{
		ID:         r.id,
		Payload:    bytes.Clone(r.payload),
		Priority:   r.priority,
		State:      r.state,
		Attempts:   r.attempts,
		LastError:  r.lastErr,
		WorkerID:   r.workerID,
		EnqueuedAt: r.enqueuedAt,
	}
}

type idemEntry struct {
	id string
	at time.Time
}

// Queue is an in memory job queue. It is safe for concurrent use.
type Queue struct {
	mu          sync.Mutex
	clock       Clock
	maxAttempts int
	seq         uint64
	jobs        map[string]*record
	queued      map[string]*record
	leased      map[string]*record
	idem        map[string]idemEntry
}

// New returns an empty queue.
func New(cfg Config) *Queue {
	clk := cfg.Clock
	if clk == nil {
		clk = systemClock{}
	}
	maxAttempts := cfg.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	return &Queue{
		clock:       clk,
		maxAttempts: maxAttempts,
		jobs:        make(map[string]*record),
		queued:      make(map[string]*record),
		leased:      make(map[string]*record),
		idem:        make(map[string]idemEntry),
	}
}

// Enqueue adds a job and returns its id. A non empty key that was used within
// the last 24 hours returns the existing job id and created is false.
func (q *Queue) Enqueue(payload []byte, priority int, key string) (id string, created bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.clock.Now()
	q.reapLocked(now)

	if key != "" {
		if e, ok := q.idem[key]; ok && now.Sub(e.at) < IdempotencyWindow {
			return e.id, false
		}
	}
	q.seq++
	r := &record{
		id:         fmt.Sprintf("job-%d", q.seq),
		seq:        q.seq,
		payload:    bytes.Clone(payload),
		priority:   priority,
		state:      StateQueued,
		enqueuedAt: now,
		visibleAt:  now,
	}
	q.jobs[r.id] = r
	q.queued[r.id] = r
	if key != "" {
		q.idem[key] = idemEntry{id: r.id, at: now}
	}
	return r.id, true
}

// Lease hides the visible job with the highest priority (FIFO within a
// priority) for d and returns it with a fresh lease token. ok is false when
// no job is visible.
func (q *Queue) Lease(workerID string, d time.Duration) (l Leased, ok bool, err error) {
	if d <= 0 {
		return Leased{}, false, ErrInvalidDuration
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.clock.Now()
	q.reapLocked(now)

	var best *record
	for _, r := range q.queued {
		if r.visibleAt.After(now) {
			continue
		}
		if best == nil || r.priority > best.priority ||
			(r.priority == best.priority && r.seq < best.seq) {
			best = r
		}
	}
	if best == nil {
		return Leased{}, false, nil
	}
	delete(q.queued, best.id)
	best.state = StateLeased
	best.attempts++
	best.token = rand.Text()
	best.workerID = workerID
	best.leaseExpires = now.Add(d)
	q.leased[best.id] = best
	return Leased{Job: best.snapshot(), Token: best.token}, true, nil
}

// Ack marks a leased job done.
func (q *Queue) Ack(id, token string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.reapLocked(q.clock.Now())
	r, err := q.currentLocked(id, token)
	if err != nil {
		return err
	}
	delete(q.leased, r.id)
	r.state = StateDone
	r.token = ""
	return nil
}

// Nack records a failed attempt. The job is dead lettered when its attempts
// are used up, otherwise it becomes visible again after Backoff(attempts).
func (q *Queue) Nack(id, token, reason string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.clock.Now()
	q.reapLocked(now)
	r, err := q.currentLocked(id, token)
	if err != nil {
		return err
	}
	q.failLocked(r, reason, now)
	return nil
}

// Extend pushes the lease expiry to now + d.
func (q *Queue) Extend(id, token string, d time.Duration) error {
	if d <= 0 {
		return ErrInvalidDuration
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.clock.Now()
	q.reapLocked(now)
	r, err := q.currentLocked(id, token)
	if err != nil {
		return err
	}
	r.leaseExpires = now.Add(d)
	return nil
}

// RetryDead moves a dead job back to the queue with attempts reset.
func (q *Queue) RetryDead(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.clock.Now()
	q.reapLocked(now)
	r, ok := q.jobs[id]
	if !ok {
		return ErrNotFound
	}
	if r.state != StateDead {
		return ErrNotDead
	}
	r.state = StateQueued
	r.attempts = 0
	r.visibleAt = now
	q.queued[r.id] = r
	return nil
}

// Get returns a snapshot of a job.
func (q *Queue) Get(id string) (Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.reapLocked(q.clock.Now())
	r, ok := q.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	return r.snapshot(), nil
}

// DeadLetters returns the dead jobs in enqueue order.
func (q *Queue) DeadLetters() []Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.reapLocked(q.clock.Now())
	var dead []*record
	for _, r := range q.jobs {
		if r.state == StateDead {
			dead = append(dead, r)
		}
	}
	sort.Slice(dead, func(i, j int) bool { return dead[i].seq < dead[j].seq })
	out := make([]Job, 0, len(dead))
	for _, r := range dead {
		out = append(out, r.snapshot())
	}
	return out
}

// Stats returns counts of jobs per state.
func (q *Queue) Stats() Stats {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.reapLocked(q.clock.Now())
	var s Stats
	for _, r := range q.jobs {
		switch r.state {
		case StateQueued:
			s.Queued++
		case StateLeased:
			s.Leased++
		case StateDone:
			s.Done++
		case StateDead:
			s.Dead++
		}
	}
	return s
}

// currentLocked returns the job if token is its current lease.
func (q *Queue) currentLocked(id, token string) (*record, error) {
	r, ok := q.jobs[id]
	if !ok {
		return nil, ErrNotFound
	}
	if r.state != StateLeased || token == "" || r.token != token {
		return nil, ErrStaleToken
	}
	return r, nil
}

// reapLocked treats every expired lease as a failed attempt.
func (q *Queue) reapLocked(now time.Time) {
	for _, r := range q.leased {
		if !now.Before(r.leaseExpires) {
			q.failLocked(r, "lease expired", r.leaseExpires)
		}
	}
}

// failLocked records a failed attempt that happened at time at.
func (q *Queue) failLocked(r *record, reason string, at time.Time) {
	delete(q.leased, r.id)
	r.token = ""
	r.workerID = ""
	r.lastErr = reason
	if r.attempts >= q.maxAttempts {
		r.state = StateDead
		return
	}
	r.state = StateQueued
	r.visibleAt = at.Add(Backoff(r.attempts))
	q.queued[r.id] = r
}
