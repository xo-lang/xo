package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	tag32 := strings.Repeat("a", 32)
	tag33 := strings.Repeat("a", 33)
	tags := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = "t" + string(rune('a'+i))
		}
		return out
	}
	tests := []struct {
		name  string
		title string
		tags  []string
		ok    bool
	}{
		{"minimal", "x", nil, true},
		{"empty title", "", nil, false},
		{"200 ascii chars", strings.Repeat("a", 200), nil, true},
		{"201 ascii chars", strings.Repeat("a", 201), nil, false},
		{"200 multibyte chars", strings.Repeat("é", 200), nil, true},
		{"201 multibyte chars", strings.Repeat("é", 201), nil, false},
		{"invalid utf8", "\xff", nil, false},
		{"10 tags", "t", tags(10), true},
		{"11 tags", "t", tags(11), false},
		{"tag charset", "t", []string{"go", "web-dev", "v2"}, true},
		{"uppercase tag", "t", []string{"Go"}, false},
		{"underscore tag", "t", []string{"a_b"}, false},
		{"space tag", "t", []string{"a b"}, false},
		{"empty tag", "t", []string{""}, false},
		{"32 char tag", "t", []string{tag32}, true},
		{"33 char tag", "t", []string{tag33}, false},
		{"newline suffix tag", "t", []string{"go\n"}, false},
		{"duplicate tag", "t", []string{"go", "go"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.title, tc.tags)
			if tc.ok && err != nil {
				t.Fatalf("expected valid, got %v", err)
			}
			if !tc.ok {
				var ve *ValidationError
				if !errors.As(err, &ve) {
					t.Fatalf("expected ValidationError, got %v", err)
				}
				if len(ve.Problems) == 0 {
					t.Fatal("validation error has no problems")
				}
			}
		})
	}
}

func TestValidateReportsAllProblems(t *testing.T) {
	err := Validate("", []string{"BAD", "ok"})
	var ve *ValidationError
	if !errors.As(err, &ve) || len(ve.Problems) != 2 {
		t.Fatalf("expected 2 problems, got %v", err)
	}
}

func TestIDs(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		id, err := NewID()
		if err != nil {
			t.Fatal(err)
		}
		if !ValidID(id) {
			t.Fatalf("NewID produced invalid id %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
	for _, bad := range []string{"", "../etc/passwd", "CON", strings.Repeat("A", 32), strings.Repeat("a", 31), strings.Repeat("g", 32), strings.Repeat("a", 32) + "\n"} {
		if ValidID(bad) {
			t.Errorf("ValidID(%q) = true", bad)
		}
	}
}

func TestFilter(t *testing.T) {
	n := Note{Title: "Grocery List", Body: "Buy MILK and eggs", Tags: []string{"home", "todo"}}
	tests := []struct {
		f    Filter
		want bool
	}{
		{Filter{}, true},
		{Filter{Tag: "home"}, true},
		{Filter{Tag: "work"}, false},
		{Filter{Query: "grocery"}, true},
		{Filter{Query: "milk"}, true},
		{Filter{Query: "BREAD"}, false},
		{Filter{Tag: "todo", Query: "EGGS"}, true},
		{Filter{Tag: "work", Query: "eggs"}, false},
	}
	for _, tc := range tests {
		if got := tc.f.Match(n); got != tc.want {
			t.Errorf("%+v.Match = %v, want %v", tc.f, got, tc.want)
		}
	}
}

func mkID(i int) string {
	const hex = "0123456789abcdef"
	b := []byte(strings.Repeat("0", 32))
	b[30] = hex[(i/16)%16]
	b[31] = hex[i%16]
	return string(b)
}

func TestPagePure(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var notes []Note
	for i := range 25 {
		notes = append(notes, Note{ID: mkID(i), Title: "n", Created: base.Add(time.Duration(i) * time.Second)})
	}
	var got []string
	var cur *Cursor
	for pages := 0; ; pages++ {
		if pages > 5 {
			t.Fatal("too many pages")
		}
		page, next := Page(notes, Filter{}, 10, cur)
		for _, n := range page {
			got = append(got, n.ID)
		}
		if next == nil {
			break
		}
		// Round trip through the opaque encoding.
		c, err := DecodeCursor(next.Encode())
		if err != nil {
			t.Fatal(err)
		}
		cur = &c
	}
	if len(got) != 25 {
		t.Fatalf("got %d notes, want 25", len(got))
	}
	for i, id := range got {
		if want := mkID(24 - i); id != want {
			t.Fatalf("position %d: got %s want %s", i, id, want)
		}
	}
}

func TestPageExactMultipleHasNoEmptyTrailingPage(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var notes []Note
	for i := range 10 {
		notes = append(notes, Note{ID: mkID(i), Created: base.Add(time.Duration(i) * time.Second)})
	}
	page, next := Page(notes, Filter{}, 10, nil)
	if len(page) != 10 || next != nil {
		t.Fatalf("got %d notes, next=%v", len(page), next)
	}
}

func TestDecodeCursorRejectsGarbage(t *testing.T) {
	for _, s := range []string{"!!!", "abc", Cursor{CreatedNano: 1, ID: "../x"}.Encode()} {
		if _, err := DecodeCursor(s); err == nil {
			t.Errorf("DecodeCursor(%q) succeeded", s)
		}
	}
}
