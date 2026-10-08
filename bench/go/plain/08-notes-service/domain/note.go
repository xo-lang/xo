// Package domain holds the Note type, its validation rules, and pure search
// and pagination logic. It depends on nothing else in this module.
package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// MaxTitleRunes is the maximum title length in characters (runes).
	MaxTitleRunes = 200
	// MaxTags is the maximum number of tags on a note.
	MaxTags = 10
)

var (
	tagPattern = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
	// IDs are 32 lowercase hex characters: safe as file names on Linux,
	// macOS (case insensitive file systems) and Windows (no reserved names,
	// no reserved characters, no trailing dots or spaces).
	idPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

// Note is a single note.
type Note struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Body    string    `json:"body"`
	Tags    []string  `json:"tags"`
	Version int64     `json:"version"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

// Clone returns a deep copy of n. Tags is never nil in the copy.
func (n Note) Clone() Note {
	if n.Tags == nil {
		n.Tags = []string{}
	} else {
		n.Tags = slices.Clone(n.Tags)
	}
	return n
}

// Validate checks the note's user supplied fields.
func (n Note) Validate() error {
	return Validate(n.Title, n.Tags)
}

// ValidationError lists every rule a note breaks.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return "invalid note: " + strings.Join(e.Problems, "; ")
}

// Validate checks a title and tag list against the note rules.
func Validate(title string, tags []string) error {
	var problems []string
	if !utf8.ValidString(title) {
		problems = append(problems, "title must be valid UTF-8")
	} else if n := utf8.RuneCountInString(title); n < 1 || n > MaxTitleRunes {
		problems = append(problems, fmt.Sprintf("title must be 1 to %d characters, got %d", MaxTitleRunes, n))
	}
	if len(tags) > MaxTags {
		problems = append(problems, fmt.Sprintf("at most %d tags allowed, got %d", MaxTags, len(tags)))
	}
	seen := make(map[string]bool, len(tags))
	for i, tag := range tags {
		if !tagPattern.MatchString(tag) {
			problems = append(problems, fmt.Sprintf("tag %d (%q) must match [a-z0-9-]{1,32}", i, tag))
			continue
		}
		if seen[tag] {
			problems = append(problems, fmt.Sprintf("tag %q is duplicated", tag))
		}
		seen[tag] = true
	}
	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}
	return nil
}

// ValidID reports whether id has the shape produced by NewID.
func ValidID(id string) bool {
	return idPattern.MatchString(id)
}

// NewID returns a new random note id (32 lowercase hex characters).
func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
