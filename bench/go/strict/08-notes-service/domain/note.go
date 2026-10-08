// Package domain defines the Note type, its validation rules, and pure
// search and pagination logic. It depends on nothing else in this module.
package domain

import (
	"crypto/rand"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Limits enforced by ValidateContent.
const (
	MaxTitleRunes = 200
	MaxTags       = 10
)

// Note is a single note. Version starts at 1 and increases by one on every
// successful update.
type Note struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Body    string    `json:"body"`
	Tags    []string  `json:"tags"`
	Version int64     `json:"version"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

// ValidationError lists every rule a note violates.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return "invalid note: " + strings.Join(e.Problems, "; ")
}

var tagRE = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// ValidateContent checks the user supplied fields of a note: the title must
// be 1 to 200 characters (Unicode code points), there may be at most 10 tags,
// each tag must match [a-z0-9-]{1,32}, and tags must not repeat.
func ValidateContent(title string, tags []string) error {
	var problems []string
	if !utf8.ValidString(title) {
		problems = append(problems, "title must be valid UTF-8")
	} else if n := utf8.RuneCountInString(title); n < 1 || n > MaxTitleRunes {
		problems = append(problems, "title must be 1 to 200 characters, got "+strconv.Itoa(n))
	}
	if len(tags) > MaxTags {
		problems = append(problems, "at most 10 tags are allowed, got "+strconv.Itoa(len(tags)))
	}
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		if !tagRE.MatchString(tag) {
			problems = append(problems, "tag "+strconv.Quote(tag)+" must match [a-z0-9-]{1,32}")
			continue
		}
		if seen[tag] {
			problems = append(problems, "tag "+strconv.Quote(tag)+" is repeated")
		}
		seen[tag] = true
	}
	if len(problems) == 0 {
		return nil
	}
	return &ValidationError{Problems: problems}
}

// Validate checks the user supplied fields of n.
func (n Note) Validate() error {
	return ValidateContent(n.Title, n.Tags)
}

var idRE = regexp.MustCompile(`^[a-z0-9]{1,64}$`)

// windowsReserved holds device names that Windows refuses as file names,
// even with an extension.
var windowsReserved = func() map[string]bool {
	m := map[string]bool{"con": true, "prn": true, "aux": true, "nul": true}
	for i := 0; i <= 9; i++ {
		m["com"+strconv.Itoa(i)] = true
		m["lpt"+strconv.Itoa(i)] = true
	}
	return m
}()

// NewID returns a fresh random note id (26 lowercase base32 characters,
// about 130 bits of randomness).
func NewID() string {
	return strings.ToLower(rand.Text())
}

// ValidID reports whether id is a well formed note id. Valid ids use only
// lowercase ASCII letters and digits, so they are safe as file names on
// Linux, macOS, and Windows (no separators, no dots, no case folding
// collisions, no reserved device names).
func ValidID(id string) bool {
	return idRE.MatchString(id) && !windowsReserved[id]
}
