// Postgres numeric on the Go backend (decision 0681): a port of pgx
// v5.12.0 pgtype/numeric.go (parseNumericString,
// parseScientificNumericString, numberTextBytes, encodeNumericText,
// toBigInt, Float64Value), Copyright (c) 2013-2021 Jack Christensen, MIT
// License; the same as std/native/sql.xo's.

package rt

import (
	"errors"
	"math"
	"math/big"
	"strconv"
	"strings"
)

const (
	numMinExp = -math.MaxInt16
	numMaxExp = math.MaxInt16*4 + 3
)

func numFinite(c *big.Int, exp int64) Sql_Numeric {
	return Sql_Numeric{Tag: 1, P: Sql_Numeric_Finite{F_coef: bigKey(c), F_exp: exp}}
}

// numPlain is pgx's parseNumericString.
func numPlain(s string) (*big.Int, int64, bool) {
	digits := s
	var exp int64
	if idx := strings.IndexByte(digits, '.'); idx == -1 {
		for len(digits) > 1 && digits[len(digits)-1] == '0' && digits[len(digits)-2] != '-' {
			digits = digits[:len(digits)-1]
			exp++
		}
	} else {
		exp = -int64(len(digits) - idx - 1)
		digits = digits[:idx] + digits[idx+1:]
	}
	c, ok := new(big.Int).SetString(digits, 10)
	return c, exp, ok
}

// numParse is a numeric's text as pgx's text scan and ScanScientific
// read it, or the reason it is not one.
func numParse(s string) (Sql_Numeric, string) {
	switch s {
	case "NaN":
		return Sql_Numeric{Tag: 2}, ""
	case "Infinity":
		return Sql_Numeric{Tag: 3}, ""
	case "-Infinity":
		return Sql_Numeric{Tag: 4}, ""
	}
	idx := strings.IndexAny(s, "eE")
	if idx == -1 {
		c, exp, ok := numPlain(s)
		if !ok {
			return Sql_Numeric{}, s + " is not a number"
		}
		return numFinite(c, exp), ""
	}
	x, err := strconv.ParseInt(s[idx+1:], 10, 32)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return Sql_Numeric{}, s + " exponent out of range"
		}
		return Sql_Numeric{}, s + " is not a number"
	}
	c, m, ok := numPlain(s[:idx])
	if !ok {
		return Sql_Numeric{}, s + " is not a number"
	}
	if e := m + x; e < numMinExp || e > numMaxExp {
		return Sql_Numeric{}, s + " exponent out of range"
	} else {
		return numFinite(c, e), ""
	}
}

// Sql_parse_numeric is `sql.parse_numeric(s)`.
func Sql_parse_numeric(_ *Task, s string) Result[Sql_Numeric, Sql_SqlErr] {
	n, msg := numParse(s)
	if msg != "" {
		return Err[Sql_Numeric](sqlMsgErr(sqlTagSyntax, msg))
	}
	return Ok[Sql_Numeric, Sql_SqlErr](n)
}

func pgNumeric(s string) (Sql_Numeric, bool) {
	n, msg := numParse(s)
	return n, msg == ""
}

func pow10Big(e int64) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(e), nil) }

func numericText(n Sql_Numeric) string {
	switch n.Tag {
	case 2:
		return "NaN"
	case 3:
		return "Infinity"
	case 4:
		return "-Infinity"
	}
	f := n.P.(Sql_Numeric_Finite)
	digits := bigOf(f.F_coef).String()
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}
	exp := int(f.F_exp)
	switch {
	case exp > 0:
		return sign + digits + strings.Repeat("0", exp)
	case exp < 0:
		if len(digits) <= -exp {
			return sign + "0." + strings.Repeat("0", -exp-len(digits)) + digits
		}
		dp := len(digits) + exp
		return sign + digits[:dp] + "." + digits[dp:]
	}
	return sign + digits
}

// M_display is `n.display()`.
func (n Sql_Numeric) M_display(_ *Task) string { return numericText(n) }

// M_to_rat is `n.to_rat()`.
func (n Sql_Numeric) M_to_rat(_ *Task) Option[Big_BigRat] {
	if n.Tag != 1 {
		return Option[Big_BigRat]{}
	}
	f := n.P.(Sql_Numeric_Finite)
	c := bigOf(f.F_coef)
	if f.F_exp >= 0 {
		return Some(ratKey(new(big.Rat).SetInt(new(big.Int).Mul(c, pow10Big(f.F_exp)))))
	}
	return Some(ratKey(new(big.Rat).SetFrac(c, pow10Big(-f.F_exp))))
}

// M_to_float is `n.to_float()` (pgx's Float64Value).
func (n Sql_Numeric) M_to_float(_ *Task) float64 {
	switch n.Tag {
	case 2:
		return math.NaN()
	case 3:
		return math.Inf(1)
	case 4:
		return math.Inf(-1)
	}
	f := n.P.(Sql_Numeric_Finite)
	v, _ := strconv.ParseFloat(bigOf(f.F_coef).String()+"e"+strconv.FormatInt(f.F_exp, 10), 64)
	return v
}

// M_to_int is `n.to_int()` (pgx's Int64Value).
func (n Sql_Numeric) M_to_int(_ *Task) Option[int64] {
	if n.Tag != 1 {
		return Option[int64]{}
	}
	f := n.P.(Sql_Numeric_Finite)
	c := bigOf(f.F_coef)
	if f.F_exp >= 0 {
		c = new(big.Int).Mul(c, pow10Big(f.F_exp))
	} else {
		q, r := new(big.Int).QuoRem(c, pow10Big(-f.F_exp), new(big.Int))
		if r.Sign() != 0 {
			return Option[int64]{}
		}
		c = q
	}
	if !c.IsInt64() {
		return Option[int64]{}
	}
	return Some(c.Int64())
}

// Sql_numeric is `sql.numeric(n)`.
func Sql_numeric(tk *Task, n Sql_Numeric) Sql_SqlValue { return Sql_str(tk, numericText(n)) }

// Sql_range_num is `sql.range_num(r)`.
func Sql_range_num(tk *Task, r Sql_Range[Sql_Numeric]) Sql_SqlValue {
	return Sql_str(tk, rgText(r, numericText))
}

// Sql_multirange_num is `sql.multirange_num(rs)`.
func Sql_multirange_num(tk *Task, rs List[Sql_Range[Sql_Numeric]]) Sql_SqlValue {
	return Sql_str(tk, mrText(rs, numericText))
}

func asNumeric(v any) (Sql_Numeric, bool) {
	switch x := v.(type) {
	case int64:
		return numFinite(big.NewInt(x), 0), true
	case string:
		return pgNumeric(x)
	}
	return Sql_Numeric{}, false
}

// M_numeric is `row.numeric(name)`.
func (r Sql_Row) M_numeric(_ *Task, name string) Result[Sql_Numeric, Sql_SqlErr] {
	return sqlCol(r, name, "a numeric", asNumeric)
}

// M_numeric_opt is `row.numeric_opt(name)`.
func (r Sql_Row) M_numeric_opt(_ *Task, name string) Result[Option[Sql_Numeric], Sql_SqlErr] {
	return sqlColOpt(r, name, "a numeric", asNumeric)
}

// M_range_num is `row.range_num(name)`.
func (r Sql_Row) M_range_num(_ *Task, name string) Result[Sql_Range[Sql_Numeric], Sql_SqlErr] {
	return sqlCol(r, name, "a numeric range", pgTyped(rgTyped(pgNumeric)))
}

// M_range_num_opt is `row.range_num_opt(name)`.
func (r Sql_Row) M_range_num_opt(_ *Task, name string) Result[Option[Sql_Range[Sql_Numeric]], Sql_SqlErr] {
	return sqlColOpt(r, name, "a numeric range", pgTyped(rgTyped(pgNumeric)))
}

// M_multirange_num is `row.multirange_num(name)`.
func (r Sql_Row) M_multirange_num(_ *Task, name string) Result[List[Sql_Range[Sql_Numeric]], Sql_SqlErr] {
	return sqlCol(r, name, "a numeric multirange", pgTyped(mrTyped(pgNumeric)))
}

// M_multirange_num_opt is `row.multirange_num_opt(name)`.
func (r Sql_Row) M_multirange_num_opt(_ *Task, name string) Result[Option[List[Sql_Range[Sql_Numeric]]], Sql_SqlErr] {
	return sqlColOpt(r, name, "a numeric multirange", pgTyped(mrTyped(pgNumeric)))
}
