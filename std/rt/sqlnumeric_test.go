package rt

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// Ported from pgx v5.12.0 pgtype/numeric_test.go
// (TestNumericScanScientificPreservesPrecision,
// TestNumericScanScientificErrors, TestNumericFloat64Valuer,
// TestNumericInt64Valuer, TestNumericCodec's text values), Copyright (c)
// 2013-2021 Jack Christensen, MIT License; the same cases as
// std/native/sql_test.xo's, for the Go backend.

func numParts(s string) string {
	r := Sql_parse_numeric(nil, s)
	if !r.Ok {
		return "error: " + r.E.P.(Sql_SqlErr_Syntax).F_msg
	}
	if r.V.Tag != 1 {
		return numericText(r.V)
	}
	f := r.V.P.(Sql_Numeric_Finite)
	return bigOf(f.F_coef).String() + "e" + strconv.FormatInt(f.F_exp, 10)
}

func mustNumeric(t *testing.T, s string) Sql_Numeric {
	t.Helper()
	r := Sql_parse_numeric(nil, s)
	if !r.Ok {
		t.Fatalf("parse_numeric(%q) failed", s)
	}
	return r.V
}

func TestSqlNumericParse(t *testing.T) {
	for _, tt := range []struct{ src, want string }{
		{"1234567890123456789e0", "1234567890123456789e0"},
		{"1.234567890123456789e5", "1234567890123456789e-13"},
		{"1e131071", "1e131071"},
		{"1.50", "150e-2"},
		{"100", "1e2"},
		{"-100", "-1e2"},
		{"0", "0e0"},
		{"-0", "0e0"},
		{"1e+3", "1e3"},
		{"NaN", "NaN"},
		{"-Infinity", "-Infinity"},
		{"1e", "error: 1e is not a number"},
		{"1e+", "error: 1e+ is not a number"},
		{"1e1.5", "error: 1e1.5 is not a number"},
		{"1e5e5", "error: 1e5e5 is not a number"},
		{"1.2.3e4", "error: 1.2.3e4 is not a number"},
		{"e5", "error: e5 is not a number"},
		{"1.2.3", "error: 1.2.3 is not a number"},
		{"1e99999999999999999999", "error: 1e99999999999999999999 exponent out of range"},
		{"1000e2147483647", "error: 1000e2147483647 exponent out of range"},
		{"1e131072", "error: 1e131072 exponent out of range"},
		{"1e-32768", "error: 1e-32768 exponent out of range"},
		{"", "error:  is not a number"},
		{"infinity", "error: infinity is not a number"},
	} {
		if got := numParts(tt.src); got != tt.want {
			t.Errorf("parse_numeric(%q) = %q, want %q", tt.src, got, tt.want)
		}
	}
}

func TestSqlNumericDisplayRoundTrip(t *testing.T) {
	for _, s := range []string{
		"1", "3.14159", "100010001", "100010001.0001",
		"4237234789234789289347892374324872138321894178943189043890124832108934.43219085471578891547854892438945012347981",
		"0." + strings.Repeat("0", 180) + "123",
		"67" + strings.Repeat("0", 44535) + ".0",
		"-0.003", "1.50", "-100", "NaN", "Infinity", "-Infinity",
	} {
		if got := numericText(mustNumeric(t, s)); got != s {
			t.Errorf("display(parse(%.40q)) = %.40q", s, got)
		}
	}
}

func TestSqlNumericConversions(t *testing.T) {
	for _, tt := range []struct {
		s string
		f float64
	}{{"1", 1}, {"0.0000000000000000001", 0.0000000000000000001}, {"-99999999999", -99999999999}} {
		if got := mustNumeric(t, tt.s).M_to_float(nil); got != tt.f {
			t.Errorf("to_float(%s) = %v, want %v", tt.s, got, tt.f)
		}
	}
	if !math.IsInf(mustNumeric(t, "1e400").M_to_float(nil), 1) || !math.IsNaN(Sql_Numeric{Tag: 2}.M_to_float(nil)) || !math.IsInf(Sql_Numeric{Tag: 4}.M_to_float(nil), -1) {
		t.Error("to_float of 1e400, NaN or -Infinity")
	}
	for _, tt := range []struct {
		s  string
		ok bool
		n  int64
	}{
		{"1", true, 1}, {"-99999999999", true, -99999999999}, {"0", true, 0}, {"12.00", true, 12},
		{"12.5", false, 0}, {"9223372036854775807", true, math.MaxInt64}, {"9223372036854775808", false, 0},
		{"NaN", false, 0}, {"Infinity", false, 0},
	} {
		got := mustNumeric(t, tt.s).M_to_int(nil)
		if got.Some != tt.ok || got.Some && got.V != tt.n {
			t.Errorf("to_int(%s) = %v, want %v %v", tt.s, got, tt.ok, tt.n)
		}
	}
	r := mustNumeric(t, "1.50").M_to_rat(nil)
	if !r.Some || ratOf(r.V).String() != "3/2" {
		t.Errorf("to_rat(1.50) = %v", r)
	}
	if mustNumeric(t, "NaN").M_to_rat(nil).Some {
		t.Error("to_rat(NaN) is Some")
	}
}
