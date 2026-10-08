package domain

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestValidateContent(t *testing.T) {
	tags := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("tag-%d", i)
		}
		return out
	}
	cases := []struct {
		name  string
		title string
		tags  []string
		ok    bool
	}{
		{"minimal", "a", nil, true},
		{"empty title", "", nil, false},
		{"200 ascii", strings.Repeat("x", 200), nil, true},
		{"201 ascii", strings.Repeat("x", 201), nil, false},
		{"200 multibyte runes", strings.Repeat("é", 200), nil, true},
		{"201 multibyte runes", strings.Repeat("é", 201), nil, false},
		{"invalid utf8", "\xff", nil, false},
		{"10 tags", "t", tags(10), true},
		{"11 tags", "t", tags(11), false},
		{"tag charset", "t", []string{"a-b-9"}, true},
		{"uppercase tag", "t", []string{"Go"}, false},
		{"tag with space", "t", []string{"a b"}, false},
		{"tag with underscore", "t", []string{"a_b"}, false},
		{"empty tag", "t", []string{""}, false},
		{"32 char tag", "t", []string{strings.Repeat("a", 32)}, true},
		{"33 char tag", "t", []string{strings.Repeat("a", 33)}, false},
		{"trailing newline tag", "t", []string{"abc\n"}, false},
		{"non ascii tag", "t", []string{"café"}, false},
		{"duplicate tag", "t", []string{"go", "go"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateContent(tc.title, tc.tags)
			if tc.ok && err != nil {
				t.Fatalf("expected valid, got %v", err)
			}
			if !tc.ok {
				var ve *ValidationError
				if !errors.As(err, &ve) {
					t.Fatalf("expected *ValidationError, got %v", err)
				}
			}
		})
	}
}

func TestValidateReportsAllProblems(t *testing.T) {
	err := Note{Title: "", Tags: []string{"BAD", "ok", "ok"}}.Validate()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
	if len(ve.Problems) != 3 {
		t.Fatalf("expected 3 problems, got %d: %v", len(ve.Problems), ve.Problems)
	}
}

func TestValidID(t *testing.T) {
	for _, id := range []string{"abc", "a1", strings.Repeat("z", 64), NewID()} {
		if !ValidID(id) {
			t.Errorf("ValidID(%q) = false, want true", id)
		}
	}
	for _, id := range []string{"", "ABC", "a.b", "../x", "a/b", `a\b`, "a b", "con", "nul", "com1", "lpt9", strings.Repeat("z", 65)} {
		if ValidID(id) {
			t.Errorf("ValidID(%q) = true, want false", id)
		}
	}
}

func TestNewIDUnique(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		id := NewID()
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestFilter(t *testing.T) {
	n := Note{Title: "Shopping List", Body: "Buy MILK and eggs", Tags: []string{"home", "todo"}}
	cases := []struct {
		f    Filter
		want bool
	}{
		{Filter{}, true},
		{Filter{Tag: "home"}, true},
		{Filter{Tag: "work"}, false},
		{Filter{Text: "shopping"}, true},
		{Filter{Text: "milk"}, true},
		{Filter{Text: "LIST"}, true},
		{Filter{Text: "bread"}, false},
		{Filter{Tag: "todo", Text: "eggs"}, true},
		{Filter{Tag: "work", Text: "eggs"}, false},
	}
	for _, tc := range cases {
		if got := tc.f.Match(n); got != tc.want {
			t.Errorf("%+v.Match = %v, want %v", tc.f, got, tc.want)
		}
	}
}

var base = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func makeNotes(n int) []Note {
	out := make([]Note, n)
	for i := range out {
		out[i] = Note{
			ID:      fmt.Sprintf("n%03d", i),
			Title:   fmt.Sprintf("note %d", i),
			Created: base.Add(time.Duration(i) * time.Second),
			Version: 1,
		}
	}
	return out
}

func pageAll(t *testing.T, notes func() []Note, limit int, between func(page int)) []string {
	t.Helper()
	var ids []string
	cursor := ""
	for page := 1; ; page++ {
		if page > 100 {
			t.Fatal("pagination did not terminate")
		}
		p, err := Search(notes(), Filter{}, limit, cursor)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		for _, n := range p.Notes {
			ids = append(ids, n.ID)
		}
		if between != nil {
			between(page)
		}
		if p.NextCursor == "" {
			return ids
		}
		cursor = p.NextCursor
	}
}

func checkUnique(t *testing.T, ids []string) map[string]bool {
	t.Helper()
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate id %s in %v", id, ids)
		}
		seen[id] = true
	}
	return seen
}

func TestSearchPagination(t *testing.T) {
	notes := makeNotes(25)
	ids := pageAll(t, func() []Note { return notes }, 10, nil)
	if len(ids) != 25 {
		t.Fatalf("got %d ids, want 25", len(ids))
	}
	checkUnique(t, ids)
	for i, id := range ids {
		if want := fmt.Sprintf("n%03d", 24-i); id != want {
			t.Fatalf("position %d: got %s, want %s", i, id, want)
		}
	}
}

func TestSearchPaginationWithInsertsMidway(t *testing.T) {
	notes := makeNotes(25)
	ids := pageAll(t, func() []Note { return notes }, 10, func(page int) {
		if page == 1 {
			// A brand new note (newest) and a backdated note (oldest).
			notes = append(notes,
				Note{ID: "newest", Created: base.Add(time.Hour)},
				Note{ID: "oldest", Created: base.Add(-time.Hour)},
			)
		}
	})
	seen := checkUnique(t, ids)
	for i := range 25 {
		if !seen[fmt.Sprintf("n%03d", i)] {
			t.Fatalf("gap: n%03d missing from %v", i, ids)
		}
	}
	if seen["newest"] {
		t.Fatal("note created before the cursor position must not appear on later pages")
	}
	if !seen["oldest"] {
		t.Fatal("note created after the cursor position should appear on a later page")
	}
}

func TestSearchTiesBrokenByID(t *testing.T) {
	notes := make([]Note, 7)
	for i := range notes {
		notes[i] = Note{ID: fmt.Sprintf("id%d", i), Created: base}
	}
	ids := pageAll(t, func() []Note { return notes }, 3, nil)
	if len(ids) != 7 {
		t.Fatalf("got %d ids, want 7", len(ids))
	}
	checkUnique(t, ids)
}

func TestSearchFilterAndEmpty(t *testing.T) {
	notes := makeNotes(5)
	notes[2].Tags = []string{"x"}
	p, err := Search(notes, Filter{Tag: "x"}, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Notes) != 1 || p.Notes[0].ID != "n002" || p.NextCursor != "" {
		t.Fatalf("unexpected page %+v", p)
	}
	p, err = Search(nil, Filter{}, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Notes == nil || len(p.Notes) != 0 {
		t.Fatalf("expected empty non nil page, got %#v", p.Notes)
	}
}

func TestSearchBadInput(t *testing.T) {
	if _, err := Search(nil, Filter{}, 0, ""); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("limit 0: got %v", err)
	}
	for _, c := range []string{"!!!", "bm9kb3Q", Cursor{Created: base, ID: "../x"}.Encode()} {
		if _, err := Search(nil, Filter{}, 10, c); !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("cursor %q: got %v", c, err)
		}
	}
}

func TestCursorRoundTrip(t *testing.T) {
	c := Cursor{Created: base.Add(123456789), ID: "abc123"}
	got, err := DecodeCursor(c.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Created.Equal(c.Created) || got.ID != c.ID {
		t.Fatalf("round trip mismatch: %+v vs %+v", got, c)
	}
}
