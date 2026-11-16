package rt

import (
	"encoding/json"
	"strconv"
	"strings"
)

// json.Number (Go's json.Number, decision 0673): a JSON number kept as its
// text. std/native/json.xo has the same rules (number_append, number_at).

// jsonValidNumber is encoding/json's isValidNumber.
func jsonValidNumber(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == '-' {
		s = s[1:]
		if s == "" {
			return false
		}
	}
	switch {
	case s[0] == '0':
		s = s[1:]
	case '1' <= s[0] && s[0] <= '9':
		s = s[1:]
		for len(s) > 0 && '0' <= s[0] && s[0] <= '9' {
			s = s[1:]
		}
	default:
		return false
	}
	if len(s) >= 2 && s[0] == '.' && '0' <= s[1] && s[1] <= '9' {
		s = s[2:]
		for len(s) > 0 && '0' <= s[0] && s[0] <= '9' {
			s = s[1:]
		}
	}
	if len(s) >= 2 && (s[0] == 'e' || s[0] == 'E') {
		s = s[1:]
		if s[0] == '+' || s[0] == '-' {
			s = s[1:]
			if s == "" {
				return false
			}
		}
		for len(s) > 0 && '0' <= s[0] && s[0] <= '9' {
			s = s[1:]
		}
	}
	return s == ""
}

// XoJsonEnc writes the text as it is: 0 for an empty text (Go's rule),
// null for one that is not a JSON number (encoding cannot fail).
func (n Json_Number) XoJsonEnc(tk *Task, w *JsonW) {
	switch {
	case n.F_text == "":
		w.b = append(w.b, '0')
	case jsonValidNumber(n.F_text):
		w.b = append(w.b, n.F_text...)
	default:
		w.b = append(w.b, "null"...)
	}
}

// XoJsonDec reads a number token's text; a string is not a number
// (decision 0673).
func (n *Json_Number) XoJsonDec(tk *Task, x any, path JsonPath) *Json_DecodeErr {
	switch t := x.(type) {
	case json.Number:
		n.F_text = string(t)
		return nil
	}
	return JsonErr(path, "expected number")
}

// M_int is `n.int()`: Go's Number.Int64.
func (n Json_Number) M_int(*Task) Result[int64, Json_DecodeErr] {
	if !strings.Contains(n.F_text, "_") {
		if v, err := strconv.ParseInt(n.F_text, 10, 64); err == nil {
			return Ok[int64, Json_DecodeErr](v)
		}
	}
	_, err := strconv.ParseInt(n.F_text, 10, 64)
	if err == nil || strings.Contains(n.F_text, "_") {
		err = &strconv.NumError{Func: "ParseInt", Num: n.F_text, Err: strconv.ErrSyntax}
	}
	return Err[int64](Json_DecodeErr{F_path: "$", F_msg: err.Error()})
}

// M_float is `n.float()`: Go's Number.Float64.
func (n Json_Number) M_float(*Task) Result[float64, Json_DecodeErr] {
	f, err := strconv.ParseFloat(n.F_text, 64)
	if err != nil {
		return Err[float64](Json_DecodeErr{F_path: "$", F_msg: err.Error()})
	}
	return Ok[float64, Json_DecodeErr](f)
}

// M_display is `n.display()`: the text.
func (n Json_Number) M_display(*Task) string { return n.F_text }
