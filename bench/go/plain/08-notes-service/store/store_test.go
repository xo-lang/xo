package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"benchtask/domain"
)

func newNote(t *testing.T, title string) domain.Note {
	t.Helper()
	id, err := domain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	return domain.Note{ID: id, Title: title, Body: "body", Tags: []string{"a"}, Version: 1, Created: now, Updated: now}
}

func tempFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), tmpPrefix) {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	s1, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := newNote(t, "first")
	b := newNote(t, "second")
	for _, n := range []domain.Note{a, b} {
		if err := s1.Create(n); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s1.Update(a.ID, 1, func(n *domain.Note) { n.Title = "first edited" }); err != nil {
		t.Fatal(err)
	}
	if err := s1.Delete(b.ID, 1); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s2.Get(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "first edited" || got.Version != 2 || !got.Created.Equal(a.Created) || got.Body != "body" || len(got.Tags) != 1 {
		t.Fatalf("reloaded note mismatch: %+v", got)
	}
	if _, err := s2.Get(b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted note came back: %v", err)
	}
	if n := len(s2.List()); n != 1 {
		t.Fatalf("List after restart has %d notes, want 1", n)
	}
}

func TestFailedCreateLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.rename = func(string, string) error { return errors.New("simulated rename failure") }
	n := newNote(t, "doomed")
	if err := s.Create(n); err == nil {
		t.Fatal("expected Create to fail")
	}
	if tmp := tempFiles(t, dir); len(tmp) != 0 {
		t.Fatalf("temp files left behind: %v", tmp)
	}
	if _, err := s.Get(n.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("failed create is visible: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, n.ID+fileSuffix)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("note file exists after failed create: %v", err)
	}
}

func TestFailedUpdateKeepsOldVersion(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := newNote(t, "original")
	if err := s.Create(n); err != nil {
		t.Fatal(err)
	}
	s.rename = func(string, string) error { return errors.New("simulated rename failure") }
	if _, err := s.Update(n.ID, 1, func(n *domain.Note) { n.Title = "changed" }); err == nil {
		t.Fatal("expected Update to fail")
	}
	if tmp := tempFiles(t, dir); len(tmp) != 0 {
		t.Fatalf("temp files left behind: %v", tmp)
	}
	got, err := s.Get(n.ID)
	if err != nil || got.Title != "original" || got.Version != 1 {
		t.Fatalf("in memory note changed after failed update: %+v %v", got, err)
	}
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err = s2.Get(n.ID)
	if err != nil || got.Title != "original" || got.Version != 1 {
		t.Fatalf("on disk note changed after failed update: %+v %v", got, err)
	}
}

func TestOpenRemovesStaleTempFiles(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, tmpPrefix+"leftover-123")
	if err := os.WriteFile(stale, []byte("{partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	if tmp := tempFiles(t, dir); len(tmp) != 0 {
		t.Fatalf("stale temp files not removed: %v", tmp)
	}
}

func TestConcurrentUpdatesExactlyOneWins(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	n := newNote(t, "contended")
	if err := s.Create(n); err != nil {
		t.Fatal(err)
	}
	const workers = 32
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, workers)
	for i := range workers {
		wg.Go(func() {
			<-start
			_, err := s.Update(n.ID, 1, func(n *domain.Note) { n.Body = string(rune('a' + i)) })
			results <- err
		})
	}
	close(start)
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, ErrVersionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if wins != 1 || conflicts != workers-1 {
		t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
	}
	got, _ := s.Get(n.ID)
	if got.Version != 2 {
		t.Fatalf("version = %d, want 2", got.Version)
	}
}

func TestRejectsUnsafeIDs(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	n := newNote(t, "x")
	n.ID = "../escape"
	if err := s.Create(n); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("got %v, want ErrInvalidID", err)
	}
}

func TestStaleDeleteConflicts(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	n := newNote(t, "x")
	if err := s.Create(n); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(n.ID, 7); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("got %v, want ErrVersionConflict", err)
	}
	if err := s.Delete("0123456789abcdef0123456789abcdef", 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}
