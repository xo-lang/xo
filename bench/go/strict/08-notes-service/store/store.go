// Package store persists notes on the filesystem, one JSON file per note.
// Every write goes to a temporary file in the same directory that is then
// renamed over the target, so readers never see a partial note.
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

// Store errors.
var (
	ErrNotFound        = errors.New("note not found")
	ErrExists          = errors.New("note already exists")
	ErrVersionConflict = errors.New("version conflict")
	ErrInvalidID       = errors.New("invalid note id")
)

const (
	tempPrefix = ".tmp-"
	ext        = ".json"
)

// Store is a directory of note files. It is safe for concurrent use within
// one process: all mutations are serialized, so version checks and writes
// happen atomically with respect to each other.
type Store struct {
	dir string
	mu  sync.Mutex
	// rename is os.Rename, replaceable in tests to simulate failures.
	rename func(oldpath, newpath string) error
}

// Open creates dir if needed and removes temporary files left behind by a
// previous crash.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read data dir: %w", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), tempPrefix) {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("remove stale temp file: %w", err)
			}
		}
	}
	return &Store{dir: dir, rename: os.Rename}, nil
}

// Dir returns the data directory.
func (s *Store) Dir() string { return s.dir }

func (s *Store) path(id string) (string, error) {
	if !domain.ValidID(id) {
		return "", ErrNotFound
	}
	return filepath.Join(s.dir, id+ext), nil
}

// Create stores a new note. It fails with ErrExists if the id is taken.
func (s *Store) Create(n domain.Note) error {
	if !domain.ValidID(n.ID) {
		return ErrInvalidID
	}
	p := filepath.Join(s.dir, n.ID+ext)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(p); err == nil {
		return ErrExists
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return s.writeAtomic(p, n)
}

// Get returns the note with the given id.
func (s *Store) Get(id string) (domain.Note, error) {
	p, err := s.path(id)
	if err != nil {
		return domain.Note{}, err
	}
	return readNote(p)
}

// Update loads the note, checks that its version equals expected, applies
// fn, bumps the version, and writes the result. ID, Created and Version are
// controlled by the store and cannot be changed by fn.
func (s *Store) Update(id string, expected int64, fn func(domain.Note) domain.Note) (domain.Note, error) {
	p, err := s.path(id)
	if err != nil {
		return domain.Note{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, err := readNote(p)
	if err != nil {
		return domain.Note{}, err
	}
	if cur.Version != expected {
		return domain.Note{}, ErrVersionConflict
	}
	next := fn(cur)
	next.ID = cur.ID
	next.Created = cur.Created
	next.Version = cur.Version + 1
	if err := s.writeAtomic(p, next); err != nil {
		return domain.Note{}, err
	}
	return next, nil
}

// Delete removes the note if its version equals expected.
func (s *Store) Delete(id string, expected int64) error {
	p, err := s.path(id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, err := readNote(p)
	if err != nil {
		return err
	}
	if cur.Version != expected {
		return ErrVersionConflict
	}
	if err := os.Remove(p); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// Each calls fn for every stored note in file name order, reading one file
// at a time. It stops at the first error fn returns.
func (s *Store) Each(fn func(domain.Note) error) error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("read data dir: %w", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, tempPrefix) || !strings.HasSuffix(name, ext) {
			continue
		}
		if !domain.ValidID(strings.TrimSuffix(name, ext)) {
			continue
		}
		n, err := readNote(filepath.Join(s.dir, name))
		if errors.Is(err, ErrNotFound) {
			continue // deleted between ReadDir and read
		}
		if err != nil {
			return err
		}
		if err := fn(n); err != nil {
			return err
		}
	}
	return nil
}

// List returns every stored note.
func (s *Store) List() ([]domain.Note, error) {
	out := make([]domain.Note, 0)
	err := s.Each(func(n domain.Note) error {
		out = append(out, n)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func readNote(p string) (domain.Note, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return domain.Note{}, ErrNotFound
		}
		return domain.Note{}, err
	}
	var n domain.Note
	if err := json.Unmarshal(data, &n); err != nil {
		return domain.Note{}, fmt.Errorf("decode %s: %w", filepath.Base(p), err)
	}
	return n, nil
}

// writeAtomic writes n to a temp file in the data directory, syncs it, and
// renames it to p. On any failure the temp file is removed.
func (s *Store) writeAtomic(p string, n domain.Note) (err error) {
	data, err := json.MarshalIndent(n, "", "  ")
	if err != nil {
		return fmt.Errorf("encode note: %w", err)
	}
	f, err := os.CreateTemp(s.dir, tempPrefix+"*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			if rmErr := os.Remove(tmp); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
				err = errors.Join(err, rmErr)
			}
		}
	}()
	if _, err = f.Write(data); err != nil {
		return errors.Join(fmt.Errorf("write temp file: %w", err), f.Close())
	}
	if err = f.Sync(); err != nil {
		return errors.Join(fmt.Errorf("sync temp file: %w", err), f.Close())
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err = s.rename(tmp, p); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
