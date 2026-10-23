package rt

import (
	"encoding/csv"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// std/csv on the Go backend (decisions 0164 and 0298): Go's encoding/csv
// itself; std/native/csv.xo ports it.

// M_display is Go's ParseError.Error.
func (e Csv_CsvErr) M_display(*Task) string {
	pe := &csv.ParseError{StartLine: int(e.F_start_line), Line: int(e.F_line), Column: int(e.F_column), Err: errors.New(e.F_msg)}
	if e.F_msg == csv.ErrFieldCount.Error() {
		pe.Err = csv.ErrFieldCount
	}
	return pe.Error()
}

func csvValidDelim(r rune) bool {
	return r != 0 && r != '"' && r != '\r' && r != '\n' && utf8.ValidRune(r) && r != utf8.RuneError
}

// csvValidSep and csvValidComment are the one delimiter rule, for
// Csv_valid_sep, Csv_valid_comment and csvCheckDelims: Go's validDelim, and
// '\0' (Go's zero value, no comment) for a comment.
func csvValidSep(r rune) bool { return csvValidDelim(r) }

func csvValidComment(r rune) bool { return r == 0 || csvValidDelim(r) }

func csvCheckDelims(sep rune, comment rune) {
	if !csvValidSep(sep) || !csvValidComment(comment) || comment != 0 && comment == sep {
		FaultHere("csv: invalid field or comment delimiter")
	}
}

// Csv_valid_sep is `csv.valid_sep(r)`.
func Csv_valid_sep(_ *Task, r Rune) bool { return csvValidSep(rune(r)) }

// Csv_valid_comment is `csv.valid_comment(r)`.
func Csv_valid_comment(_ *Task, r Rune) bool { return csvValidComment(rune(r)) }

func csvReadAll(text string, sep Rune, comment Rune, fields int64, lazy, trim bool) ([][]string, *Csv_CsvErr) {
	csvCheckDelims(rune(sep), rune(comment))
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = rune(sep)
	r.Comment = rune(comment)
	r.FieldsPerRecord = int(fields)
	r.LazyQuotes = lazy
	r.TrimLeadingSpace = trim
	var recs [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			return recs, nil
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				return nil, &Csv_CsvErr{F_start_line: int64(pe.StartLine), F_line: int64(pe.Line), F_column: int64(pe.Column), F_msg: pe.Err.Error()}
			}
			return nil, &Csv_CsvErr{F_msg: err.Error()}
		}
		recs = append(recs, rec)
	}
}

// Csv_parse is `csv.parse(text, sep, comment, fields, lazy_quotes,
// trim_leading_space)`.
func Csv_parse(_ *Task, text string, sep Rune, comment Rune, fields int64, lazy bool, trim bool) Result[List[List[string]], Csv_CsvErr] {
	recs, err := csvReadAll(text, sep, comment, fields, lazy, trim)
	if err != nil {
		return Err[List[List[string]]](*err)
	}
	out := make([]List[string], len(recs))
	for i, r := range recs {
		out[i] = ListOf(r...)
	}
	return Ok[List[List[string]], Csv_CsvErr](ListOf(out...))
}

// Csv_parse_maps is `csv.parse_maps(...)`: FieldsPerRecord 0.
func Csv_parse_maps(_ *Task, text string, sep Rune, comment Rune, lazy bool, trim bool) Result[List[Map[string, string]], Csv_CsvErr] {
	recs, err := csvReadAll(text, sep, comment, 0, lazy, trim)
	if err != nil {
		return Err[List[Map[string, string]]](*err)
	}
	if len(recs) == 0 {
		return Ok[List[Map[string, string]], Csv_CsvErr](List[Map[string, string]]{})
	}
	head := recs[0]
	var out []Map[string, string]
	for _, r := range recs[1:] {
		var m Map[string, string]
		for j, h := range head {
			Map_put(nil, &m, h, r[j])
		}
		out = append(out, m)
	}
	return Ok[List[Map[string, string]], Csv_CsvErr](ListOf(out...))
}

// Csv_format is `csv.format(rows, sep, crlf)`: Go's Writer, and the only,
// empty field of a record quoted (so it reads back as a record).
func Csv_format(_ *Task, rows List[List[string]], sep Rune, crlf bool) string {
	csvCheckDelims(rune(sep), 0)
	var b strings.Builder
	for _, r := range rows.s {
		if len(r.s) == 1 && r.s[0] == "" {
			b.WriteString(`""`)
			if crlf {
				b.WriteString("\r\n")
			} else {
				b.WriteString("\n")
			}
			continue
		}
		w := csv.NewWriter(&b)
		w.Comma = rune(sep)
		w.UseCRLF = crlf
		w.Write(r.s)
		w.Flush()
	}
	return b.String()
}
