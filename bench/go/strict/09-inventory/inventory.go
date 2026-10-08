package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// HoldDuration is how long a reservation holds stock before it expires.
const HoldDuration = 15 * time.Minute

// SweepInterval is how often the background sweeper releases expired holds.
const SweepInterval = 30 * time.Second

// Sentinel errors. The HTTP layer maps them to status codes.
var (
	ErrInvalid           = errors.New("invalid request")
	ErrNotFound          = errors.New("not found")
	ErrInsufficientStock = errors.New("not enough stock")
	ErrAlreadyConfirmed  = errors.New("reservation already confirmed")
	ErrCancelled         = errors.New("reservation cancelled")
	ErrConflict          = errors.New("conflict")
	ErrExpired           = errors.New("reservation expired")
)

// Status is the lifecycle state of a reservation.
type Status int

// Reservation states.
const (
	StatusActive Status = iota
	StatusConfirmed
	StatusCancelled
	StatusExpired
)

func (s Status) String() string {
	switch s {
	case StatusActive:
		return "active"
	case StatusConfirmed:
		return "confirmed"
	case StatusCancelled:
		return "cancelled"
	case StatusExpired:
		return "expired"
	}
	return "unknown"
}

// MarshalText renders the status as its name in JSON.
func (s Status) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// Reservation is a hold of Qty units of an item.
type Reservation struct {
	ID             string    `json:"id"`
	ItemID         string    `json:"item_id"`
	Qty            int       `json:"qty"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	Status         Status    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// ItemView is the externally visible state of an item.
type ItemView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Stock     int    `json:"stock"`
	Held      int    `json:"held"`
	Available int    `json:"available"`
}

type item struct {
	id    string
	name  string
	stock int
	held  int
}

func (it *item) view() ItemView {
	return ItemView{ID: it.id, Name: it.name, Stock: it.stock, Held: it.held, Available: it.stock - it.held}
}

// Service is an in memory inventory that never oversells. All state is
// guarded by a single mutex, which makes the availability check and the hold
// one atomic step.
type Service struct {
	mu       sync.Mutex
	now      func() time.Time
	items    map[string]*item
	res      map[string]*Reservation
	active   map[string]*Reservation
	byKey    map[string]string
	nextItem int
	nextRes  int
}

// NewService creates a service using now as its clock. A nil clock means time.Now.
func NewService(now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{
		now:    now,
		items:  make(map[string]*item),
		res:    make(map[string]*Reservation),
		active: make(map[string]*Reservation),
		byKey:  make(map[string]string),
	}
}

// CreateItem adds an item. An empty id is generated.
func (s *Service) CreateItem(id, name string, stock int) (ItemView, error) {
	if name == "" {
		return ItemView{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if stock < 0 {
		return ItemView{}, fmt.Errorf("%w: stock must not be negative", ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == "" {
		for {
			s.nextItem++
			id = fmt.Sprintf("item-%d", s.nextItem)
			if _, exists := s.items[id]; !exists {
				break
			}
		}
	}
	if _, exists := s.items[id]; exists {
		return ItemView{}, fmt.Errorf("%w: item %q already exists", ErrConflict, id)
	}
	it := &item{id: id, name: name, stock: stock}
	s.items[id] = it
	return it.view(), nil
}

// GetItem returns stock, held and available counts. Expired holds are
// released first.
func (s *Service) GetItem(id string) (ItemView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(s.now())
	it, ok := s.items[id]
	if !ok {
		return ItemView{}, fmt.Errorf("%w: item %q", ErrNotFound, id)
	}
	return it.view(), nil
}

// Reserve holds qty units of itemID for HoldDuration. A non empty
// idempotency key that was already used returns the original reservation and
// replayed=true without holding more stock.
func (s *Service) Reserve(itemID string, qty int, key string) (r Reservation, replayed bool, err error) {
	if qty <= 0 {
		return Reservation{}, false, fmt.Errorf("%w: qty must be positive", ErrInvalid)
	}
	if itemID == "" {
		return Reservation{}, false, fmt.Errorf("%w: item_id is required", ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.expireLocked(now)

	if key != "" {
		if id, ok := s.byKey[key]; ok {
			prev, found := s.res[id]
			if found {
				if prev.ItemID != itemID || prev.Qty != qty {
					return Reservation{}, false, fmt.Errorf("%w: idempotency key reused with different parameters", ErrConflict)
				}
				return *prev, true, nil
			}
		}
	}

	it, ok := s.items[itemID]
	if !ok {
		return Reservation{}, false, fmt.Errorf("%w: item %q", ErrNotFound, itemID)
	}
	if it.stock-it.held < qty {
		return Reservation{}, false, fmt.Errorf("%w: requested %d, available %d", ErrInsufficientStock, qty, it.stock-it.held)
	}
	s.nextRes++
	nr := &Reservation{
		ID:             fmt.Sprintf("res-%d", s.nextRes),
		ItemID:         itemID,
		Qty:            qty,
		IdempotencyKey: key,
		Status:         StatusActive,
		CreatedAt:      now,
		ExpiresAt:      now.Add(HoldDuration),
	}
	it.held += qty
	s.res[nr.ID] = nr
	s.active[nr.ID] = nr
	if key != "" {
		s.byKey[key] = nr.ID
	}
	return *nr, false, nil
}

// Confirm turns an active hold into a sale.
func (s *Service) Confirm(id string) (Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.res[id]
	if !ok {
		return Reservation{}, fmt.Errorf("%w: reservation %q", ErrNotFound, id)
	}
	now := s.now()
	switch r.Status {
	case StatusActive:
		if !now.Before(r.ExpiresAt) {
			s.releaseLocked(r, StatusExpired)
			return *r, ErrExpired
		}
		it, found := s.items[r.ItemID]
		if !found {
			return *r, fmt.Errorf("%w: item %q", ErrNotFound, r.ItemID)
		}
		it.held -= r.Qty
		it.stock -= r.Qty
		r.Status = StatusConfirmed
		delete(s.active, r.ID)
		return *r, nil
	case StatusConfirmed:
		return *r, ErrAlreadyConfirmed
	case StatusCancelled:
		return *r, ErrCancelled
	case StatusExpired:
		return *r, ErrExpired
	}
	return *r, fmt.Errorf("unknown reservation status %d", int(r.Status))
}

// Cancel releases an active hold. Cancelling twice is a no op.
func (s *Service) Cancel(id string) (Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.res[id]
	if !ok {
		return Reservation{}, fmt.Errorf("%w: reservation %q", ErrNotFound, id)
	}
	switch r.Status {
	case StatusActive:
		if !s.now().Before(r.ExpiresAt) {
			s.releaseLocked(r, StatusExpired)
			return *r, ErrExpired
		}
		s.releaseLocked(r, StatusCancelled)
		return *r, nil
	case StatusConfirmed:
		return *r, ErrAlreadyConfirmed
	case StatusCancelled:
		return *r, nil
	case StatusExpired:
		return *r, ErrExpired
	}
	return *r, fmt.Errorf("unknown reservation status %d", int(r.Status))
}

// GetReservation returns a reservation, expiring it first if due.
func (s *Service) GetReservation(id string) (Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(s.now())
	r, ok := s.res[id]
	if !ok {
		return Reservation{}, fmt.Errorf("%w: reservation %q", ErrNotFound, id)
	}
	return *r, nil
}

// Sweep releases every expired hold and returns how many were released.
func (s *Service) Sweep() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.expireLocked(s.now())
}

// RunSweeper calls Sweep on every tick until ctx is done.
func (s *Service) RunSweeper(ctx context.Context, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ticks:
			if !ok {
				return
			}
			s.Sweep()
		}
	}
}

func (s *Service) expireLocked(now time.Time) int {
	n := 0
	for _, r := range s.active {
		if !now.Before(r.ExpiresAt) {
			s.releaseLocked(r, StatusExpired)
			n++
		}
	}
	return n
}

func (s *Service) releaseLocked(r *Reservation, to Status) {
	if it, ok := s.items[r.ItemID]; ok {
		it.held -= r.Qty
	}
	r.Status = to
	delete(s.active, r.ID)
}
