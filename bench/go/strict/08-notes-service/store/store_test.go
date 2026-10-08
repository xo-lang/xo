package store

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"benchtask/domain"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newNote(id, title string) domain.Note {
	return domain.Note{ID: id, Title: title, Tags: []string{"a"}, Version: 1, Created: t0, Updated: t0}
}

func open(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func setTitle(title string) func(domain.Note) domain.Note {
	return func(n domain.Note) domain.Note {
		n.Title = title
		return n
	}
}

func TestSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	s1 := open(t, dir)
	if err := s1.Create(newNote("aaa", "first")); err != nil {
		t.Fatal(err)
	}
	if err := s1.Create(newNote("bbb", "second")); err != nil {
		t.Fatal(err)
	}
	if _, err := s1.Update("aaa", 1, setTitle("first edited")); err != nil {
		t.Fatal(err)
	}

	s2 := open(t, dir)
	got, err := s2.Get("aaa")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "first edited" || got.Version != 2 || !got.Created.Equal(t0) {
		t.Fatalf("unexpected note after restart: %+v", got)
	}
	all, err := s2.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("List after restart: got %d notes, want 2", len(all))
	}
}

func TestFailedWriteLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	boom := errors.New("boom")
	s.rename = func(string, string) error { return boom }

	if err := s.Create(newNote("aaa", "x")); !errors.Is(err, boom) {
		t.Fatalf("Create with failing rename: got %v, want boom", err)
	}
	if names := dirNames(t, dir); len(names) != 0 {
		t.Fatalf("expected empty dir after failed create, got %v", names)
	}

	s.rename = os.Rename
	if err := s.Create(newNote("aaa", "x")); err != nil {
		t.Fatal(err)
	}
	s.rename = func(string, string) error { return boom }
	if _, err := s.Update("aaa", 1, setTitle("y")); !errors.Is(err, boom) {
		t.Fatalf("Update with failing rename: got %v, want boom", err)
	}
	if names := dirNames(t, dir); !slices.Equal(names, []string{"aaa.json"}) {
		t.Fatalf("expected only aaa.json after failed update, got %v", names)
	}
	got, err := s.Get("aaa")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "x" || got.Version != 1 {
		t.Fatalf("failed update changed the note: %+v", got)
	}
}

func TestFailedTempCreateReportsError(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	// Point the store at a directory that no longer exists so CreateTemp fails.
	s.dir = filepath.Join(dir, "gone")
	if err := s.Create(newNote("aaa", "x")); err == nil {
		t.Fatal("expected error when data dir is missing")
	}
	if names := dirNames(t, dir); len(names) != 0 {
		t.Fatalf("expected no files, got %v", names)
	}
}

func TestOpenRemovesStaleTempFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, tempPrefix+"123"), []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "keep.json"), []byte(`{"id":"keep","title":"k","version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := open(t, dir)
	if names := dirNames(t, dir); !slices.Equal(names, []string{"keep.json"}) {
		t.Fatalf("got %v", names)
	}
	all, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != "keep" {
		t.Fatalf("got %+v", all)
	}
}

func TestVersionChecks(t *testing.T) {
	s := open(t, t.TempDir())
	if err := s.Create(newNote("aaa", "x")); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(newNote("aaa", "x")); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate create: got %v", err)
	}
	if _, err := s.Update("aaa", 2, setTitle("y")); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale update: got %v", err)
	}
	n, err := s.Update("aaa", 1, func(n domain.Note) domain.Note {
		n.Title = "y"
		n.Version = 99 // must be ignored
		n.ID = "zzz"   // must be ignored
		return n
	})
	if err != nil {
		t.Fatal(err)
	}
	if n.Version != 2 || n.ID != "aaa" {
		t.Fatalf("store must control id and version: %+v", n)
	}
	if err := s.Delete("aaa", 1); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale delete: got %v", err)
	}
	if err := s.Delete("aaa", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("aaa"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete: got %v", err)
	}
	if err := s.Delete("aaa", 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: got %v", err)
	}
	if _, err := s.Update("missing", 1, setTitle("z")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing: got %v", err)
	}
}

func TestRejectsUnsafeIDs(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	for _, id := range []string{"../evil", "a/b", `a\b`, "con", "UPPER", ""} {
		if err := s.Create(newNote(id, "x")); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Create(%q): got %v, want ErrInvalidID", id, err)
		}
		if _, err := s.Get(id); !errors.Is(err, ErrNotFound) {
			t.Errorf("Get(%q): got %v, want ErrNotFound", id, err)
		}
	}
	if names := dirNames(t, dir); len(names) != 0 {
		t.Fatalf("unexpected files %v", names)
	}
}

func TestConcurrentUpdatesExactlyOneWins(t *testing.T) {
	s := open(t, t.TempDir())
	if err := s.Create(newNote("aaa", "x")); err != nil {
		t.Fatal(err)
	}
	const writers = 16
	errs := make([]error, writers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range writers {
		wg.Go(func() {
			<-start
			_, errs[i] = s.Update("aaa", 1, setTitle("w"))
		})
	}
	close(start)
	wg.Wait()
	wins := 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, ErrVersionConflict):
		default:
			t.Fatalf("unexpected error %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("got %d winners, want exactly 1", wins)
	}
}
