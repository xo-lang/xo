package domain

import (
	"cmp"
	"encoding/base64"
	"errors"
	"slices"
	"strconv"
	"strings"
)

// Filter selects notes by tag and by case insensitive text.
type Filter struct {
	Tag   string
	Query string
}

// Match reports whether n passes the filter.
func (f Filter) Match(n Note) bool {
	if f.Tag != "" && !slices.Contains(n.Tags, f.Tag) {
		return false
	}
	if f.Query != "" {
		q := strings.ToLower(f.Query)
		if !strings.Contains(strings.ToLower(n.Title), q) && !strings.Contains(strings.ToLower(n.Body), q) {
			return false
		}
	}
	return true
}

// compareNewestFirst orders notes by creation time descending, then by id
// descending so the order is total even when timestamps collide.
func compareNewestFirst(a, b Note) int {
	if c := cmp.Compare(b.Created.UnixNano(), a.Created.UnixNano()); c != 0 {
		return c
	}
	return cmp.Compare(b.ID, a.ID)
}

// SortNewestFirst sorts notes in place, newest first.
func SortNewestFirst(notes []Note) {
	slices.SortFunc(notes, compareNewestFirst)
}

// Cursor marks a position in the newest first order. Because the position is
// a (created, id) key and not an offset, notes created between pages (which
// sort before the cursor) never shift later pages.
type Cursor struct {
	CreatedNano int64
	ID          string
}

// ErrBadCursor is returned for a cursor that cannot be decoded.
var ErrBadCursor = errors.New("invalid cursor")

// CursorAt returns the cursor positioned at n.
func CursorAt(n Note) Cursor {
	return Cursor{CreatedNano: n.Created.UnixNano(), ID: n.ID}
}

// Encode returns the opaque string form of c.
func (c Cursor) Encode() string {
	raw := strconv.FormatInt(c.CreatedNano, 10) + "." + c.ID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor parses a string produced by Cursor.Encode.
func DecodeCursor(s string) (Cursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, ErrBadCursor
	}
	ts, id, ok := strings.Cut(string(b), ".")
	if !ok || !ValidID(id) {
		return Cursor{}, ErrBadCursor
	}
	nano, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return Cursor{}, ErrBadCursor
	}
	return Cursor{CreatedNano: nano, ID: id}, nil
}

// precedes reports whether n comes strictly after the cursor position in the
// newest first order (that is, n is older than the cursor).
func (c Cursor) precedes(n Note) bool {
	nano := n.Created.UnixNano()
	if nano != c.CreatedNano {
		return nano < c.CreatedNano
	}
	return n.ID < c.ID
}

// Page returns up to limit notes matching f, newest first, starting strictly
// after the cursor (if any). next is non-nil when more matching notes exist.
func Page(notes []Note, f Filter, limit int, after *Cursor) (page []Note, next *Cursor) {
	if limit < 1 {
		limit = 1
	}
	sorted := slices.Clone(notes)
	SortNewestFirst(sorted)
	page = make([]Note, 0, min(limit, len(sorted)))
	for _, n := range sorted {
		if after != nil && !after.precedes(n) {
			continue
		}
		if !f.Match(n) {
			continue
		}
		if len(page) == limit {
			c := CursorAt(page[len(page)-1])
			return page, &c
		}
		page = append(page, n)
	}
	return page, nil
}
