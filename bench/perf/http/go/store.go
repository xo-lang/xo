package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"sync"
)

// User is the stored resource.
type User struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	Email    string  `json:"email"`
	Nickname *string `json:"nickname,omitempty"`
}

var (
	errNotFound       = errors.New("user not found")
	errDuplicateEmail = errors.New("email already exists")
)

// Store is an in-memory, concurrency-safe user store.
type Store struct {
	mu      sync.RWMutex
	nextID  int
	users   map[int]User
	byEmail map[string]int
}

// NewStore returns an empty store whose first id is 1.
func NewStore() *Store {
	return &Store{
		nextID:  1,
		users:   make(map[int]User),
		byEmail: make(map[string]int),
	}
}

func cloneUser(u User) User {
	if u.Nickname != nil {
		n := *u.Nickname
		u.Nickname = &n
	}
	return u
}

// Create stores a new user and assigns its id.
func (s *Store) Create(name, email string, nickname *string) (User, error) {
	t0 := statNow()
	s.mu.Lock()
	t1 := statNow()
	defer func() {
		t2 := statNow()
		s.mu.Unlock()
		statRecord(opCreate, t0, t1, t2)
	}()
	if _, ok := s.byEmail[email]; ok {
		return User{}, errDuplicateEmail
	}
	u := cloneUser(User{ID: s.nextID, Name: name, Email: email, Nickname: nickname})
	s.nextID++
	s.users[u.ID] = u
	s.byEmail[email] = u.ID
	return cloneUser(u), nil
}

// Get returns the user with the given id.
func (s *Store) Get(id int) (User, error) {
	t0 := statNow()
	s.mu.RLock()
	t1 := statNow()
	defer func() {
		t2 := statNow()
		s.mu.RUnlock()
		statRecord(opGet, t0, t1, t2)
	}()
	u, ok := s.users[id]
	if !ok {
		return User{}, errNotFound
	}
	return cloneUser(u), nil
}

// ListJSON returns all users ordered by id as the JSON GET /users
// answers (json.Encoder's: the array and a newline), encoded under the
// read lock (owner, 2026-10-21): nothing is cloned out, and a writer
// waits for the encoding. Until 2026-10-21 GET /users took List (sorted
// clones) and encoded them after the lock.
func (s *Store) ListJSON() []byte {
	t0 := statNow()
	s.mu.RLock()
	t1 := statNow()
	defer func() {
		t2 := statNow()
		s.mu.RUnlock()
		statRecord(opList, t0, t1, t2)
	}()
	ids := make([]int, 0, len(s.users))
	for id := range s.users {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	var b bytes.Buffer
	b.Grow(64*len(ids) + 3)
	b.WriteByte('[')
	enc := json.NewEncoder(&b)
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		u := s.users[id]
		_ = enc.Encode(&u)
		// Encode ends each value with a newline; the array has none inside
		b.Truncate(b.Len() - 1)
	}
	b.WriteString("]\n")
	return b.Bytes()
}

// List returns all users ordered by id.
func (s *Store) List() []User {
	t0 := statNow()
	s.mu.RLock()
	t1 := statNow()
	defer func() {
		t2 := statNow()
		s.mu.RUnlock()
		statRecord(opList, t0, t1, t2)
	}()
	out := make([]User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, cloneUser(u))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Patch describes a partial update. Nil fields are left unchanged.
// ClearNickname removes the nickname.
type Patch struct {
	Name          *string
	Email         *string
	Nickname      *string
	ClearNickname bool
}

// Update applies p to the user with the given id.
func (s *Store) Update(id int, p Patch) (User, error) {
	t0 := statNow()
	s.mu.Lock()
	t1 := statNow()
	defer func() {
		t2 := statNow()
		s.mu.Unlock()
		statRecord(opUpdate, t0, t1, t2)
	}()
	u, ok := s.users[id]
	if !ok {
		return User{}, errNotFound
	}
	if p.Email != nil && *p.Email != u.Email {
		if _, taken := s.byEmail[*p.Email]; taken {
			return User{}, errDuplicateEmail
		}
		delete(s.byEmail, u.Email)
		u.Email = *p.Email
		s.byEmail[u.Email] = id
	}
	if p.Name != nil {
		u.Name = *p.Name
	}
	if p.ClearNickname {
		u.Nickname = nil
	} else if p.Nickname != nil {
		n := *p.Nickname
		u.Nickname = &n
	}
	s.users[id] = u
	return cloneUser(u), nil
}

// Delete removes the user with the given id.
func (s *Store) Delete(id int) error {
	t0 := statNow()
	s.mu.Lock()
	t1 := statNow()
	defer func() {
		t2 := statNow()
		s.mu.Unlock()
		statRecord(opDelete, t0, t1, t2)
	}()
	u, ok := s.users[id]
	if !ok {
		return errNotFound
	}
	delete(s.users, id)
	delete(s.byEmail, u.Email)
	return nil
}
