package rt

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// std/csv (std.md 16, decision 0164). The native backends implement the
// same parser in Xo (std/native/csv.xo); the results and error texts
// must stay identical (the golden program csv_basic runs on both).

// M_display is `csv: line 3, column 5: msg`.
func (e Csv_CsvErr) M_display(*Task) string {
	return "csv: line " + strconv.FormatInt(e.F_line, 10) + ", column " + strconv.FormatInt(e.F_column, 10) + ": " + e.F_msg
}

func csvErr(line, col int, msg string) *Csv_CsvErr {
	return &Csv_CsvErr{F_line: int64(line), F_column: int64(col), F_msg: msg}
}

// csvParse reads the records of text; lines holds the line each record
// starts on.
func csvParse(text string, sep rune) (recs [][]string, lines []int, err *Csv_CsvErr) {
	if sep == '"' || sep == '\r' || sep == '\n' || !utf8.ValidRune(sep) {
		FaultHere("csv: %s cannot separate fields", strconv.QuoteRune(sep))
	}
	ss := string(sep)
	i, line, col0 := 0, 1, 0 // col0: offset of the current line's start
	n := len(text)
	for i < n {
		// an empty line is skipped
		if text[i] == '\n' {
			i++
			line++
			col0 = i
			continue
		}
		// a final \r ends the text as \n would (Go's csv.Reader)
		if text[i] == '\r' && i+1 == n {
			i++
			continue
		}
		if text[i] == '\r' && i+1 < n && text[i+1] == '\n' {
			i += 2
			line++
			col0 = i
			continue
		}
		var rec []string
		start := line
		for {
			var field strings.Builder
			if i < n && text[i] == '"' {
				ql, qc := line, i-col0+1
				i++
				closed := false
				for i < n {
					c := text[i]
					if c == '"' {
						if i+1 < n && text[i+1] == '"' {
							field.WriteByte('"')
							i += 2
							continue
						}
						i++
						closed = true
						break
					}
					if c == '\r' && i+1 < n && text[i+1] == '\n' {
						field.WriteByte('\n')
						i += 2
						line++
						col0 = i
						continue
					}
					if c == '\n' {
						line++
						col0 = i + 1
					}
					field.WriteByte(c)
					i++
				}
				if !closed {
					return nil, nil, csvErr(ql, qc, "quoted field not closed")
				}
				if i < n && !strings.HasPrefix(text[i:], ss) && text[i] != '\n' && !(text[i] == '\r' && i+1 < n && text[i+1] == '\n') && !(text[i] == '\r' && i+1 == n) {
					return nil, nil, csvErr(line, i-col0+1, "unexpected character after a quoted field")
				}
			} else {
				for i < n {
					c := text[i]
					if c == '\n' || c == '\r' && (i+1 == n || text[i+1] == '\n') || strings.HasPrefix(text[i:], ss) {
						break
					}
					if c == '"' {
						return nil, nil, csvErr(line, i-col0+1, "\" in an unquoted field")
					}
					field.WriteByte(c)
					i++
				}
			}
			rec = append(rec, field.String())
			if i < n && strings.HasPrefix(text[i:], ss) {
				i += len(ss)
				continue
			}
			// the end of the record: \n, \r\n, a final \r, or the end
			if i < n && text[i] == '\r' {
				i++
			}
			if i < n && text[i] == '\n' {
				i++
			}
			line++
			col0 = i
			break
		}
		recs = append(recs, rec)
		lines = append(lines, start)
	}
	return recs, lines, nil
}

// Csv_parse is `csv.parse(text, sep)`.
func Csv_parse(_ *Task, text string, sep Rune) Result[List[List[string]], Csv_CsvErr] {
	recs, _, err := csvParse(text, rune(sep))
	if err != nil {
		return Err[List[List[string]]](*err)
	}
	out := make([]List[string], len(recs))
	for i, r := range recs {
		out[i] = ListOf(r...)
	}
	return Ok[List[List[string]], Csv_CsvErr](ListOf(out...))
}

// Csv_parse_maps is `csv.parse_maps(text, sep)`.
func Csv_parse_maps(_ *Task, text string, sep Rune) Result[List[Map[string, string]], Csv_CsvErr] {
	recs, lines, err := csvParse(text, rune(sep))
	if err != nil {
		return Err[List[Map[string, string]]](*err)
	}
	if len(recs) == 0 {
		return Ok[List[Map[string, string]], Csv_CsvErr](List[Map[string, string]]{})
	}
	head := recs[0]
	var out []Map[string, string]
	for k, r := range recs[1:] {
		if len(r) != len(head) {
			return Err[List[Map[string, string]]](*csvErr(lines[k+1], 1, "expected "+strconv.Itoa(len(head))+" fields as in the header, found "+strconv.Itoa(len(r))))
		}
		var m Map[string, string]
		for j, h := range head {
			Map_put(nil, &m, h, r[j])
		}
		out = append(out, m)
	}
	return Ok[List[Map[string, string]], Csv_CsvErr](ListOf(out...))
}

// Csv_format is `csv.format(rows, sep)`.
func Csv_format(_ *Task, rows List[List[string]], sep Rune) string {
	if rune(sep) == '"' || sep == '\r' || sep == '\n' || !utf8.ValidRune(rune(sep)) {
		FaultHere("csv: %s cannot separate fields", strconv.QuoteRune(rune(sep)))
	}
	ss := string(rune(sep))
	var b strings.Builder
	for _, r := range rows.s {
		for j, f := range r.s {
			if j > 0 {
				b.WriteString(ss)
			}
			quote := f == "" && len(r.s) == 1 || strings.Contains(f, ss) || strings.ContainsAny(f, "\"\r\n") || strings.HasPrefix(f, " ") || strings.HasPrefix(f, "\t")
			if !quote {
				b.WriteString(f)
				continue
			}
			b.WriteByte('"')
			b.WriteString(strings.ReplaceAll(f, `"`, `""`))
			b.WriteByte('"')
		}
		b.WriteByte('\n')
	}
	return b.String()
}
