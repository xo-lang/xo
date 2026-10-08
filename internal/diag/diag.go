// Package diag defines compiler diagnostics in the JSON format of core 10.2.
package diag

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/token"
)

// Severity of a diagnostic.
type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
	Note    Severity = "note"
)

// Span is the JSON form of a source range. End is exclusive. EndLine and
// EndCol are omitted for point locations (as in the notes of core 10.2).
type Span struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Col     int    `json:"col"`
	EndLine int    `json:"end_line,omitempty"`
	EndCol  int    `json:"end_col,omitempty"`
}

// FromSpan converts a token span to its JSON form.
func FromSpan(s token.Span) Span {
	return Span{File: s.File, Line: s.Start.Line, Col: s.Start.Col, EndLine: s.End.Line, EndCol: s.End.Col}
}

// PointSpan is a zero width span at p, used for insertion fixes.
func PointSpan(file string, p token.Pos) Span {
	return Span{File: file, Line: p.Line, Col: p.Col, EndLine: p.Line, EndCol: p.Col}
}

// NoteItem is a secondary message attached to a diagnostic.
type NoteItem struct {
	Message string `json:"message"`
	Span    *Span  `json:"span,omitempty"`
}

// Edit replaces the text in Span with Replace.
type Edit struct {
	Span    Span   `json:"span"`
	Replace string `json:"replace"`
}

// Fix is a machine applicable change.
type Fix struct {
	Title string `json:"title"`
	Edits []Edit `json:"edits"`
}

// Diagnostic is one reported problem.
type Diagnostic struct {
	Code     string     `json:"code"`
	Severity Severity   `json:"severity"`
	Message  string     `json:"message"`
	Span     Span       `json:"span"`
	Notes    []NoteItem `json:"notes"`
	Fixes    []Fix      `json:"fixes"`
}

// MarshalJSON always emits notes and fixes as arrays so consumers can rely
// on a fixed shape.
func (d Diagnostic) MarshalJSON() ([]byte, error) {
	type plain Diagnostic
	p := plain(d)
	if p.Notes == nil {
		p.Notes = []NoteItem{}
	}
	if p.Fixes == nil {
		p.Fixes = []Fix{}
	}
	for i := range p.Fixes {
		if p.Fixes[i].Edits == nil {
			p.Fixes[i].Edits = []Edit{}
		}
	}
	return json.Marshal(p)
}

// New creates an error diagnostic for code at span.
func New(code Code, span token.Span, format string, args ...any) Diagnostic {
	return Diagnostic{
		Code:     string(code),
		Severity: Error,
		Message:  fmt.Sprintf(format, args...),
		Span:     FromSpan(span),
	}
}

// List collects diagnostics.
type List struct {
	Items []Diagnostic
}

// Add appends d.
func (l *List) Add(d Diagnostic) { l.Items = append(l.Items, d) }

// Errorf adds an error diagnostic and returns a pointer to it so callers can
// attach notes or fixes.
func (l *List) Errorf(code Code, span token.Span, format string, args ...any) *Diagnostic {
	l.Items = append(l.Items, New(code, span, format, args...))
	return &l.Items[len(l.Items)-1]
}

// HasErrors reports whether any diagnostic has error severity.
func (l *List) HasErrors() bool {
	for _, d := range l.Items {
		if d.Severity == Error {
			return true
		}
	}
	return false
}

// Sort orders diagnostics by file and position.
func Sort(ds []Diagnostic) {
	sort.SliceStable(ds, func(i, j int) bool {
		a, b := ds[i].Span, ds[j].Span
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Col < b.Col
	})
}

// Human formats d in a compact, grep friendly form:
//
//	file:line:col: error XO0160: message
//	  note: ...
//	  fix: title
func Human(d Diagnostic) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s:%d:%d: %s %s: %s", d.Span.File, d.Span.Line, d.Span.Col, d.Severity, d.Code, d.Message)
	for _, n := range d.Notes {
		if n.Span != nil {
			fmt.Fprintf(&b, "\n  note: %s:%d:%d: %s", n.Span.File, n.Span.Line, n.Span.Col, n.Message)
		} else {
			fmt.Fprintf(&b, "\n  note: %s", n.Message)
		}
	}
	for _, f := range d.Fixes {
		fmt.Fprintf(&b, "\n  fix: %s", f.Title)
	}
	return b.String()
}
