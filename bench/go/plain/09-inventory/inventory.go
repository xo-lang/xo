package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// HoldDuration is how long a reservation holds stock before it expires.
const HoldDuration = 15 * time.Minute

// SweepInterval is how often the background sweeper releases expired holds.
const SweepInterval = 30 * time.Second

// Clock abstracts time so tests can control it.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Errors returned by the store. The HTTP layer maps them to status codes.
var (
	ErrInvalid           = errors.New("invalid input")
	ErrNotFound          = errors.New("not found")
	ErrItemExists        = errors.New("item already exists")
	ErrInsufficientStock = errors.New("not enough stock")
	ErrAlreadyConfirmed  = errors.New("reservation already confirmed")
	ErrCancelled         = errors.New("reservation cancelled")
	ErrExpired           = errors.New("reservation expired")
	ErrIdempotencyReuse  = errors.New("idempotency key reused with different parameters")
)

// Status of a reservation.
type Status string

const (
	StatusActive    Status = "active"
	StatusConfirmed Status = "confirmed"
	StatusCancelled Status = "cancelled"
	StatusExpired   Status = "expired"
)

// Item is a stocked product.
type Item struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Stock int    `json:"stock"`
}

// ItemView is an item together with its held and available counts.
type ItemView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Stock     int    `json:"stock"`
	Held      int    `json:"held"`
	Available int    `json:"available"`
}

// Reservation is a hold on some units of an item.
type Reservation struct {
	ID             string    `json:"id"`
	ItemID         string    `json:"item_id"`
	Qty            int       `json:"qty"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	Status         Status    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type itemState struct {
	item Item
	held int
	// active reservation ids for this item, used for lazy expiry.
	active map[string]struct{}
}

// Store is an in memory inventory. All operations are serialized by one mutex,
// which makes the "never oversell" invariant trivial to reason about.
type Store struct {
	mu           sync.Mutex
	clock        Clock
	items        map[string]*itemState
	reservations map[string]*Reservation
	idem         map[string]string // idempotency key -> reservation id
}

// NewStore creates an empty store using the given clock (nil means real time).
func NewStore(c Clock) *Store {
	if c == nil {
		c = realClock{}
	}
	return &Store{
		clock:        c,
		items:        make(map[string]*itemState),
		reservations: make(map[string]*Reservation),
		idem:         make(map[string]string),
	}
}

func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

// CreateItem adds a new item. An empty id is generated.
func (s *Store) CreateItem(id, name string, stock int) (ItemView, error) {
	if name == "" || stock < 0 {
		return ItemView{}, ErrInvalid
	}
	if id == "" {
		id = newID("item_")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; ok {
		return ItemView{}, ErrItemExists
	}
	st := &itemState{item: Item{ID: id, Name: name, Stock: stock}, active: make(map[string]struct{})}
	s.items[id] = st
	return st.view(), nil
}

func (st *itemState) view() ItemView {
	return ItemView{
		ID:        st.item.ID,
		Name:      st.item.Name,
		Stock:     st.item.Stock,
		Held:      st.held,
		Available: st.item.Stock - st.held,
	}
}

// expireItemLocked releases all expired holds on one item. Caller holds s.mu.
func (s *Store) expireItemLocked(st *itemState, now time.Time) int {
	n := 0
	for rid := range st.active {
		r := s.reservations[rid]
		if !now.Before(r.ExpiresAt) {
			r.Status = StatusExpired
			st.held -= r.Qty
			delete(st.active, rid)
			n++
		}
	}
	return n
}

// expireResLocked expires a single reservation if its time has passed.
func (s *Store) expireResLocked(r *Reservation, now time.Time) {
	if r.Status != StatusActive || now.Before(r.ExpiresAt) {
		return
	}
	r.Status = StatusExpired
	if st, ok := s.items[r.ItemID]; ok {
		st.held -= r.Qty
		delete(st.active, r.ID)
	}
}

// GetItem returns stock, held and available. Expired holds are released lazily.
func (s *Store) GetItem(id string) (ItemView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.items[id]
	if !ok {
		return ItemView{}, ErrNotFound
	}
	s.expireItemLocked(st, s.clock.Now())
	return st.view(), nil
}

// Reserve holds qty units of an item. The bool reports whether a new
// reservation was created (false means an idempotent replay).
func (s *Store) Reserve(itemID string, qty int, key string) (Reservation, bool, error) {
	if itemID == "" || qty <= 0 {
		return Reservation{}, false, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock.Now()
	if key != "" {
		if rid, ok := s.idem[key]; ok {
			r := s.reservations[rid]
			if r.ItemID != itemID || r.Qty != qty {
				return Reservation{}, false, ErrIdempotencyReuse
			}
			s.expireResLocked(r, now)
			return *r, false, nil
		}
	}
	st, ok := s.items[itemID]
	if !ok {
		return Reservation{}, false, ErrNotFound
	}
	s.expireItemLocked(st, now)
	if st.item.Stock-st.held < qty {
		return Reservation{}, false, ErrInsufficientStock
	}
	r := &Reservation{
		ID:             newID("res_"),
		ItemID:         itemID,
		Qty:            qty,
		IdempotencyKey: key,
		Status:         StatusActive,
		CreatedAt:      now,
		ExpiresAt:      now.Add(HoldDuration),
	}
	s.reservations[r.ID] = r
	st.held += qty
	st.active[r.ID] = struct{}{}
	if key != "" {
		s.idem[key] = r.ID
	}
	return *r, true, nil
}

// GetReservation returns a reservation by id.
func (s *Store) GetReservation(id string) (Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.reservations[id]
	if !ok {
		return Reservation{}, ErrNotFound
	}
	s.expireResLocked(r, s.clock.Now())
	return *r, nil
}

func statusErr(st Status) error {
	switch st {
	case StatusConfirmed:
		return ErrAlreadyConfirmed
	case StatusCancelled:
		return ErrCancelled
	case StatusExpired:
		return ErrExpired
	}
	return nil
}

// Confirm turns an active hold into a sale.
func (s *Store) Confirm(id string) (Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.reservations[id]
	if !ok {
		return Reservation{}, ErrNotFound
	}
	s.expireResLocked(r, s.clock.Now())
	if err := statusErr(r.Status); err != nil {
		return *r, err
	}
	st := s.items[r.ItemID]
	st.held -= r.Qty
	st.item.Stock -= r.Qty
	delete(st.active, r.ID)
	r.Status = StatusConfirmed
	return *r, nil
}

// Cancel releases an active hold.
func (s *Store) Cancel(id string) (Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.reservations[id]
	if !ok {
		return Reservation{}, ErrNotFound
	}
	s.expireResLocked(r, s.clock.Now())
	if err := statusErr(r.Status); err != nil {
		return *r, err
	}
	st := s.items[r.ItemID]
	st.held -= r.Qty
	delete(st.active, r.ID)
	r.Status = StatusCancelled
	return *r, nil
}

// Sweep releases every expired hold and returns how many were released.
func (s *Store) Sweep() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock.Now()
	n := 0
	for _, st := range s.items {
		n += s.expireItemLocked(st, now)
	}
	return n
}

// RunSweeper calls Sweep on every tick until ctx is done. It returns when
// stopped, so callers can wait for it during graceful shutdown.
func (s *Store) RunSweeper(ctx context.Context, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			s.Sweep()
		}
	}
}

// StartSweeper runs the sweeper on a real ticker. The returned function stops
// it and waits for the goroutine to exit.
func (s *Store) StartSweeper(interval time.Duration) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	t := time.NewTicker(interval)
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.RunSweeper(ctx, t.C)
	}()
	return func() {
		cancel()
		t.Stop()
		<-done
	}
}
