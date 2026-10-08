package domain

import (
	"encoding/base64"
	"errors"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Search errors.
var (
	ErrInvalidCursor = errors.New("invalid cursor")
	ErrInvalidLimit  = errors.New("limit must be at least 1")
)

// Filter selects notes. Empty fields match everything.
type Filter struct {
	Tag  string
	Text string
}

// Match reports whether n has the tag (when set) and contains the text
// (when set) in its title or body, ignoring case.
func (f Filter) Match(n Note) bool {
	if f.Tag != "" && !slices.Contains(n.Tags, f.Tag) {
		return false
	}
	if f.Text != "" {
		q := strings.ToLower(f.Text)
		if !strings.Contains(strings.ToLower(n.Title), q) && !strings.Contains(strings.ToLower(n.Body), q) {
			return false
		}
	}
	return true
}

// CompareNewestFirst orders notes by creation time, newest first, breaking
// ties by id descending so the order is total.
func CompareNewestFirst(a, b Note) int {
	if c := b.Created.Compare(a.Created); c != 0 {
		return c
	}
	return strings.Compare(b.ID, a.ID)
}

// Cursor is a position in the newest first order. It names the last note
// returned, so later pages start strictly after it. Notes created after the
// cursor was issued sort before it and never shift later pages.
type Cursor struct {
	Created time.Time
	ID      string
}

// CursorAfter returns the cursor that continues after n.
func CursorAfter(n Note) Cursor {
	return Cursor{Created: n.Created, ID: n.ID}
}

// Encode returns the opaque string form of c.
func (c Cursor) Encode() string {
	raw := strconv.FormatInt(c.Created.UnixNano(), 10) + "." + c.ID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor parses a string produced by Cursor.Encode.
func DecodeCursor(s string) (Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	ts, id, ok := strings.Cut(string(raw), ".")
	if !ok || !ValidID(id) {
		return Cursor{}, ErrInvalidCursor
	}
	nanos, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	return Cursor{Created: time.Unix(0, nanos).UTC(), ID: id}, nil
}

// Page is one page of search results. NextCursor is empty on the last page.
type Page struct {
	Notes      []Note
	NextCursor string
}

// Search filters notes, sorts them newest first, and returns up to limit
// notes after cursor (an empty cursor starts at the beginning). The input
// slice is not modified.
func Search(notes []Note, f Filter, limit int, cursor string) (Page, error) {
	if limit < 1 {
		return Page{}, ErrInvalidLimit
	}
	matched := make([]Note, 0, len(notes))
	for _, n := range notes {
		if f.Match(n) {
			matched = append(matched, n)
		}
	}
	slices.SortFunc(matched, CompareNewestFirst)

	start := 0
	if cursor != "" {
		c, err := DecodeCursor(cursor)
		if err != nil {
			return Page{}, err
		}
		key := Note{Created: c.Created, ID: c.ID}
		start = sort.Search(len(matched), func(i int) bool {
			return CompareNewestFirst(matched[i], key) > 0
		})
	}
	end := len(matched)
	if limit < end-start {
		end = start + limit
	}
	page := Page{Notes: slices.Clone(matched[start:end])}
	if end < len(matched) && end > start {
		page.NextCursor = CursorAfter(matched[end-1]).Encode()
	}
	return page, nil
}
