// Package jobqueue implements an in memory job queue with leases, retries,
// a dead letter list, a worker runner, and a small HTTP API.
package jobqueue

import (
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Errors returned by Queue operations.
var (
	ErrNotFound   = errors.New("jobqueue: job not found")
	ErrStaleToken = errors.New("jobqueue: lease token is not current")
	ErrNotDead    = errors.New("jobqueue: job is not dead")
)

// Defaults used when Options leaves a field zero.
const (
	DefaultMaxAttempts       = 5
	DefaultIdempotencyWindow = 24 * time.Hour
	DefaultLeaseDuration     = 30 * time.Second
	baseBackoff              = time.Second
	maxBackoff               = 30 * time.Second
)

// State is the lifecycle state of a job.
type State string

// Job states.
const (
	StateQueued State = "queued" // waiting, possibly hidden until VisibleAt (backoff)
	StateLeased State = "leased"
	StateDone   State = "done"
	StateDead   State = "dead"
)

// Clock abstracts time so tests can control it.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// RealClock returns a Clock backed by time.Now.
func RealClock() Clock { return realClock{} }

// FakeClock is a manually advanced Clock for tests.
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewFakeClock returns a FakeClock set to start.
func NewFakeClock(start time.Time) *FakeClock { return &FakeClock{now: start} }

// Now returns the current fake time.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the fake time forward by d.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// Job is a snapshot of a job's state. Mutating it does not affect the queue.
type Job struct {
	ID             string
	Payload        []byte
	Priority       int
	State          State
	Attempts       int
	LastError      string
	IdempotencyKey string
	EnqueuedAt     time.Time
	VisibleAt      time.Time // when a queued job becomes leasable
	LeaseExpiresAt time.Time // zero unless leased
	WorkerID       string    // zero unless leased
}

// Stats holds job counts by state.
type Stats struct {
	Queued int `json:"queued"`
	Leased int `json:"leased"`
	Done   int `json:"done"`
	Dead   int `json:"dead"`
}

// Options configures a Queue.
type Options struct {
	MaxAttempts       int           // default 5
	Clock             Clock         // default real clock
	IdempotencyWindow time.Duration // default 24h
}

type job struct {
	Job
	seq   uint64
	token string
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
	idemWindow  time.Duration

	jobs   map[string]*job
	queued map[string]*job
	leased map[string]*job
	idem   map[string]idemEntry
	seq    uint64
	nextID uint64
}

// New returns an empty Queue.
func New(opts Options) *Queue {
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = DefaultMaxAttempts
	}
	if opts.Clock == nil {
		opts.Clock = RealClock()
	}
	if opts.IdempotencyWindow <= 0 {
		opts.IdempotencyWindow = DefaultIdempotencyWindow
	}
	return &Queue{
		clock:       opts.Clock,
		maxAttempts: opts.MaxAttempts,
		idemWindow:  opts.IdempotencyWindow,
		jobs:        make(map[string]*job),
		queued:      make(map[string]*job),
		leased:      make(map[string]*job),
		idem:        make(map[string]idemEntry),
	}
}

// Backoff returns the visibility delay after the given number of attempts:
// min(30s, 1s * 2^(attempts-1)).
func Backoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 6 { // 2^5 = 32s already exceeds the cap
		return maxBackoff
	}
	return min(maxBackoff, baseBackoff<<(attempts-1))
}

// Enqueue adds a job. If idempotencyKey is non empty and was used within the
// idempotency window, the existing job id is returned with duplicate true and
// no job is created.
func (q *Queue) Enqueue(payload []byte, priority int, idempotencyKey string) (id string, duplicate bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.clock.Now()
	if idempotencyKey != "" {
		if e, ok := q.idem[idempotencyKey]; ok && now.Sub(e.at) < q.idemWindow {
			return e.id, true
		}
	}
	q.nextID++
	q.seq++
	j := &job{
		Job: Job{
			ID:             fmt.Sprintf("job-%d", q.nextID),
			Payload:        append([]byte(nil), payload...),
			Priority:       priority,
			State:          StateQueued,
			IdempotencyKey: idempotencyKey,
			EnqueuedAt:     now,
			VisibleAt:      now,
		},
		seq: q.seq,
	}
	q.jobs[j.ID] = j
	q.queued[j.ID] = j
	if idempotencyKey != "" {
		q.idem[idempotencyKey] = idemEntry{id: j.ID, at: now}
	}
	return j.ID, false
}

// reapLocked expires leases whose deadline has passed. Caller holds q.mu.
func (q *Queue) reapLocked(now time.Time) {
	for _, j := range q.leased {
		if !now.Before(j.LeaseExpiresAt) {
			// An expired lease is a failed attempt. The job becomes visible
			// again at the moment the lease expired (no extra backoff).
			q.failLocked(j, "lease expired", j.LeaseExpiresAt, false)
		}
	}
}

// failLocked records a failed attempt on a leased job. Caller holds q.mu.
func (q *Queue) failLocked(j *job, reason string, at time.Time, applyBackoff bool) {
	delete(q.leased, j.ID)
	j.token = ""
	j.WorkerID = ""
	j.LeaseExpiresAt = time.Time{}
	j.LastError = reason
	if j.Attempts >= q.maxAttempts {
		j.State = StateDead
		return
	}
	j.State = StateQueued
	j.VisibleAt = at
	if applyBackoff {
		j.VisibleAt = at.Add(Backoff(j.Attempts))
	}
	q.queued[j.ID] = j
}

func newToken() string { return rand.Text() }

// Lease returns the visible job with the highest priority (FIFO within a
// priority), hides it for leaseDuration, and returns a fresh lease token.
// ok is false when no job is visible. A non positive leaseDuration uses
// DefaultLeaseDuration.
func (q *Queue) Lease(workerID string, leaseDuration time.Duration) (j Job, token string, ok bool) {
	if leaseDuration <= 0 {
		leaseDuration = DefaultLeaseDuration
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.clock.Now()
	q.reapLocked(now)

	var best *job
	for _, c := range q.queued {
		if now.Before(c.VisibleAt) {
			continue
		}
		if best == nil || c.Priority > best.Priority || (c.Priority == best.Priority && c.seq < best.seq) {
			best = c
		}
	}
	if best == nil {
		return Job{}, "", false
	}
	delete(q.queued, best.ID)
	best.State = StateLeased
	best.Attempts++
	best.token = newToken()
	best.WorkerID = workerID
	best.LeaseExpiresAt = now.Add(leaseDuration)
	q.leased[best.ID] = best
	return best.snapshot(), best.token, true
}

// currentLocked returns the leased job if token is its current lease.
func (q *Queue) currentLocked(id, token string) (*job, error) {
	j, ok := q.jobs[id]
	if !ok {
		return nil, ErrNotFound
	}
	if j.State != StateLeased || token == "" || j.token != token {
		return nil, ErrStaleToken
	}
	return j, nil
}

// Ack marks a leased job done. It fails with ErrStaleToken if token is not
// the current lease.
func (q *Queue) Ack(id, token string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.reapLocked(q.clock.Now())
	j, err := q.currentLocked(id, token)
	if err != nil {
		return err
	}
	delete(q.leased, id)
	j.State = StateDone
	j.token = ""
	j.WorkerID = ""
	j.LeaseExpiresAt = time.Time{}
	return nil
}

// Nack records a failed attempt. The job is dead lettered if it has used
// MaxAttempts, otherwise it becomes visible again after Backoff(attempts).
func (q *Queue) Nack(id, token, reason string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.clock.Now()
	q.reapLocked(now)
	j, err := q.currentLocked(id, token)
	if err != nil {
		return err
	}
	q.failLocked(j, reason, now, true)
	return nil
}

// Extend sets the lease to expire leaseDuration from now. It fails with
// ErrStaleToken if token is not the current lease.
func (q *Queue) Extend(id, token string, leaseDuration time.Duration) error {
	if leaseDuration <= 0 {
		leaseDuration = DefaultLeaseDuration
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.clock.Now()
	q.reapLocked(now)
	j, err := q.currentLocked(id, token)
	if err != nil {
		return err
	}
	j.LeaseExpiresAt = now.Add(leaseDuration)
	return nil
}

// RetryDead moves a dead job back to the queue with attempts reset. It is
// placed at the back of its priority level.
func (q *Queue) RetryDead(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.clock.Now()
	q.reapLocked(now)
	j, ok := q.jobs[id]
	if !ok {
		return ErrNotFound
	}
	if j.State != StateDead {
		return ErrNotDead
	}
	q.seq++
	j.seq = q.seq
	j.State = StateQueued
	j.Attempts = 0
	j.VisibleAt = now
	q.queued[id] = j
	return nil
}

// Get returns a snapshot of a job.
func (q *Queue) Get(id string) (Job, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.reapLocked(q.clock.Now())
	j, ok := q.jobs[id]
	if !ok {
		return Job{}, false
	}
	return j.snapshot(), true
}

// Stats returns counts of jobs by state. Queued includes jobs waiting out a
// backoff.
func (q *Queue) Stats() Stats {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.reapLocked(q.clock.Now())
	var s Stats
	for _, j := range q.jobs {
		switch j.State {
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

func (j *job) snapshot() Job {
	s := j.Job
	s.Payload = append([]byte(nil), j.Payload...)
	return s
}
