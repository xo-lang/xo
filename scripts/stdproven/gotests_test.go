package main

import (
	"fmt"
	"strings"
	"testing"
)

// TestGoTableCases checks the gotests table reader on testdata/gotests:
// keyed and unkeyed elements, pointer elements, + of strings, a table
// local to a test function, numbers and time's zone names, and what is
// skipped (no literal, not UTF-8, an expression, a float with a fraction),
// and rune, bool and NAME:const inputs (task #403).
func TestGoTableCases(t *testing.T) {
	tests := []struct {
		name, fields string
		want         []string
		skipped      int
	}{
		{"keyed", "in,out", []string{"0 a|1", "1 bc|2"}, 2},
		{"keyed", "out", []string{"0 1", "1 2", "2 3", "3 4"}, 0},
		{"unkeyed", "0,1", []string{"0 x|y", "1 z|w"}, 0},
		{"nums", "year,month,z,f", []string{"0 2011|-3|Local|1000000000", "1 16|1000|UTC|2"}, 3},
		{"TestLocal.tests", "in", []string{"0 p", "1 q"}, 0},
		// a rune and a bool always; an exported constant (or a | of
		// them) only in a :const field; a lowercase name never
		{"flags", "pad,on,flags:const", []string{"0 46|true|Debug", "1 9|false|AlignRight|Debug", "2 39|true|0"}, 2},
		{"flags", "pad,flags", []string{"2 39|0"}, 4},
	}
	for _, tt := range tests {
		tab := goTable{pkg: "ex", file: "ex_test.go", name: tt.name, fields: strings.Split(tt.fields, ","), kind: "k"}
		cs, sk, err := goTableCases("scripts/stdproven/testdata/gotests", tab)
		if err != nil {
			t.Fatalf("%s %s: %v", tt.name, tt.fields, err)
		}
		var got []string
		for _, c := range cs {
			got = append(got, fmt.Sprintf("%d %s", c.index, strings.Join(c.inputs, "|")))
		}
		if strings.Join(got, ";") != strings.Join(tt.want, ";") || sk != tt.skipped {
			t.Errorf("%s %s: got %q, %d skipped; want %q, %d skipped", tt.name, tt.fields, got, sk, tt.want, tt.skipped)
		}
	}
}
