package main

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

// User is the public representation of a stored user.
type User struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Email    string  `json:"email"`
	Nickname *string `json:"nickname,omitempty"`
}

// clone returns a deep copy so callers cannot mutate store internals.
func (u User) clone() User {
	if u.Nickname != nil {
		n := *u.Nickname
		u.Nickname = &n
	}
	return u
}

// ErrNotFound is returned when a user id does not exist.
var ErrNotFound = errors.New("user not found")

// ErrDuplicateEmail is returned when another user already has the email.
var ErrDuplicateEmail = errors.New("email already exists")

// Patch describes a partial update. Nil pointers mean "leave unchanged".
// When SetNickname is true, Nickname is applied (nil clears it).
type Patch struct {
	Name        *string
	Email       *string
	Nickname    *string
	SetNickname bool
}

// Store is an in-memory, concurrency-safe user store.
type Store struct {
	mu     sync.Mutex
	nextID int64
	users  map[int64]User
	emails map[string]int64
}

// NewStore returns an empty store whose first id is 1.
func NewStore() *Store {
	return &Store{
		nextID: 1,
		users:  make(map[int64]User),
		emails: make(map[string]int64),
	}
}

func emailKey(email string) string {
	return strings.ToLower(email)
}

// Create stores a new user and returns it with its assigned id.
func (s *Store) Create(name, email string, nickname *string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := emailKey(email)
	if _, ok := s.emails[key]; ok {
		return User{}, ErrDuplicateEmail
	}
	u := User{ID: s.nextID, Name: name, Email: email, Nickname: nickname}.clone()
	s.nextID++
	s.users[u.ID] = u
	s.emails[key] = u.ID
	return u.clone(), nil
}

// Get returns the user with the given id.
func (s *Store) Get(id int64) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u.clone(), nil
}

// List returns all users ordered by id.
func (s *Store) List() []User {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u.clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Update applies p to the user with the given id.
func (s *Store) Update(id int64, p Patch) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return User{}, ErrNotFound
	}
	oldKey := emailKey(u.Email)
	newKey := oldKey
	if p.Email != nil {
		newKey = emailKey(*p.Email)
		if owner, taken := s.emails[newKey]; taken && owner != id {
			return User{}, ErrDuplicateEmail
		}
		u.Email = *p.Email
	}
	if p.Name != nil {
		u.Name = *p.Name
	}
	if p.SetNickname {
		u.Nickname = p.Nickname
	}
	u = u.clone()
	if newKey != oldKey {
		delete(s.emails, oldKey)
		s.emails[newKey] = id
	}
	s.users[id] = u
	return u.clone(), nil
}

// Delete removes the user with the given id.
func (s *Store) Delete(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return ErrNotFound
	}
	delete(s.emails, emailKey(u.Email))
	delete(s.users, id)
	return nil
}
