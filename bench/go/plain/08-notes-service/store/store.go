// Package store persists notes on the file system, one JSON file per note.
// It depends only on domain.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"benchtask/domain"
)

const (
	tmpPrefix  = ".tmp-"
	fileSuffix = ".json"
)

var (
	// ErrNotFound means no note has the requested id.
	ErrNotFound = errors.New("note not found")
	// ErrVersionConflict means the expected version is not the current one.
	ErrVersionConflict = errors.New("version conflict")
	// ErrExists means a note with the id already exists.
	ErrExists = errors.New("note already exists")
	// ErrInvalidID means the id is not a valid note id.
	ErrInvalidID = errors.New("invalid note id")
)

// Store is a file backed note store. All methods are safe for concurrent use.
// Every mutation holds the store lock across the version check and the disk
// write, so two updates with the same expected version can never both win.
type Store struct {
	dir   string
	mu    sync.Mutex
	notes map[string]domain.Note

	// rename is os.Rename, replaceable in tests to simulate failures.
	rename func(oldpath, newpath string) error
}

// Open loads every note in dir (creating dir if needed) and removes temp
// files left behind by a crash.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("store: create data dir: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("store: read data dir: %w", err)
	}
	s := &Store{dir: dir, notes: make(map[string]domain.Note), rename: os.Rename}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(name, tmpPrefix) {
			if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("store: remove stale temp file: %w", err)
			}
			continue
		}
		if !strings.HasSuffix(name, fileSuffix) {
			continue
		}
		id := strings.TrimSuffix(name, fileSuffix)
		if !domain.ValidID(id) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("store: read %s: %w", name, err)
		}
		var n domain.Note
		if err := json.Unmarshal(data, &n); err != nil {
			return nil, fmt.Errorf("store: decode %s: %w", name, err)
		}
		if n.ID != id {
			return nil, fmt.Errorf("store: %s holds note id %q", name, n.ID)
		}
		s.notes[id] = n.Clone()
	}
	return s, nil
}

func (s *Store) path(id string) string {
	return filepath.Join(s.dir, id+fileSuffix)
}

// write atomically replaces the file for n: temp file, fsync, rename. On any
// failure the temp file is removed.
func (s *Store) write(n domain.Note) (err error) {
	data, err := json.MarshalIndent(n, "", "  ")
	if err != nil {
		return fmt.Errorf("store: encode note: %w", err)
	}
	f, err := os.CreateTemp(s.dir, tmpPrefix+n.ID+"-*")
	if err != nil {
		return fmt.Errorf("store: create temp file: %w", err)
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err != nil {
		return fmt.Errorf("store: write temp file: %w", err)
	}
	if err = f.Sync(); err != nil {
		return fmt.Errorf("store: sync temp file: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("store: close temp file: %w", err)
	}
	if err = s.rename(tmp, s.path(n.ID)); err != nil {
		return fmt.Errorf("store: rename temp file: %w", err)
	}
	syncDir(s.dir)
	return nil
}

// syncDir makes the rename durable where the platform supports it. Errors
// are ignored because some platforms (Windows) cannot fsync a directory.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}

// Create stores a new note.
func (s *Store) Create(n domain.Note) error {
	if !domain.ValidID(n.ID) {
		return ErrInvalidID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.notes[n.ID]; ok {
		return ErrExists
	}
	n = n.Clone()
	if err := s.write(n); err != nil {
		return err
	}
	s.notes[n.ID] = n
	return nil
}

// Get returns the note with id.
func (s *Store) Get(id string) (domain.Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.notes[id]
	if !ok {
		return domain.Note{}, ErrNotFound
	}
	return n.Clone(), nil
}

// Update applies mutate to a copy of the note if its current version equals
// expected, then persists it with the version incremented. mutate cannot
// change the id, version or creation time.
func (s *Store) Update(id string, expected int64, mutate func(*domain.Note)) (domain.Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.notes[id]
	if !ok {
		return domain.Note{}, ErrNotFound
	}
	if cur.Version != expected {
		return domain.Note{}, ErrVersionConflict
	}
	next := cur.Clone()
	if mutate != nil {
		mutate(&next)
	}
	next = next.Clone()
	next.ID = cur.ID
	next.Created = cur.Created
	next.Version = cur.Version + 1
	if err := s.write(next); err != nil {
		return domain.Note{}, err
	}
	s.notes[id] = next
	return next.Clone(), nil
}

// Delete removes the note if its current version equals expected.
func (s *Store) Delete(id string, expected int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.notes[id]
	if !ok {
		return ErrNotFound
	}
	if cur.Version != expected {
		return ErrVersionConflict
	}
	if err := os.Remove(s.path(id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("store: delete note: %w", err)
	}
	syncDir(s.dir)
	delete(s.notes, id)
	return nil
}

// List returns a copy of every note in no particular order.
func (s *Store) List() []domain.Note {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.Note, 0, len(s.notes))
	for _, n := range s.notes {
		out = append(out, n.Clone())
	}
	return out
}
