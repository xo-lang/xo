package build

import (
	"slices"
	"testing"
)

// TestIOFlagsTextC checks that code calling the C path of the number and
// time text compiles the runtime with -DXO_TEXT_C (decision 0193), and
// that the default (Xo) path, which calls rs_push_word and the other
// primitives, does not.
func TestIOFlagsTextC(t *testing.T) {
	for _, c := range []struct {
		code string
		want bool
	}{
		{"bl _xo_rs_append_duration\n", true},
		{"call ptr @xo_rs_append_quote(ptr %1, i64 %2, ptr %3, i64 %4, ptr %5)\n", true},
		{"bl _xo_int_parse_ok\n", true},
		{"bl _xo_time_parse\n", true},
		// a string literal naming it (xo1's own io_flags table) is no call
		{"\t.ascii \"xo_int_parse\"\n", false},
		{"@s = private constant [12 x i8] c\"xo_int_parse\"\n", false},
		{"bl _xo_rs_push_word\nbl _xo_rs_append_bytes\nbl _xo_bytes_span\nbl _xo_int_wrap\n", false},
	} {
		got := slices.Contains(IOFlags([]byte(c.code)), "-DXO_TEXT_C")
		if got != c.want {
			t.Errorf("IOFlags(%q) has -DXO_TEXT_C: %v, want %v", c.code, got, c.want)
		}
	}
}

// TestIOFlagsStep5 checks the C paths of decision 0193 step 5: the leaf
// text of Debug (XO_LLRT_C=show), the report of a property test's case
// (report), and the zone rules (tz) compile their C only into programs
// that call it; the primitives the Xo paths call (xo_tz_data,
// xo_gen_choices, xo_rs_append_rune) turn on nothing.
func TestIOFlagsStep5(t *testing.T) {
	for _, c := range []struct {
		code, flag string
		want       bool
	}{
		{"bl _xo_rs_append_u64\n", "-DXO_SHOW_C", true},
		{"call ptr @xo_rs_append_rune_quote(ptr %1, i64 %2)\n", "-DXO_SHOW_C", true},
		{"bl _xo_rs_append_rune\nbl _xo_rs_push_word\n", "-DXO_SHOW_C", false},
		{"bl _xo_prop_report\n", "-DXO_REPORT_C", true},
		{"bl _xo_gen_choices\nbl _xo_test_write\nbl _xo_prop_best\n", "-DXO_REPORT_C", false},
		{"bl _xo_tz_load_host\n", "-DXO_TZ_C", true},
		{"bl _xo_tz_offset\n", "-DXO_TZ_C", true},
		{"bl _xo_tz_data\nbl _xo_tz_find\nbl _xo_tz_keep\nbl _xo_tz_now\nbl _xo_tz_local_name\n", "-DXO_TZ_C", false},
		{".ascii \"xo_tz_offset\"\n", "-DXO_TZ_C", false},
	} {
		got := slices.Contains(IOFlags([]byte(c.code)), c.flag)
		if got != c.want {
			t.Errorf("IOFlags(%q) has %s: %v, want %v", c.code, c.flag, got, c.want)
		}
	}
}

// TestIOFlagsDeletedCPaths checks that the runtime symbols of the C
// paths decision 0198 deleted (unicode, strings, num, jsontext) turn on
// no flag: their -DXO_*_C sections are gone from the runtime.
func TestIOFlagsDeletedCPaths(t *testing.T) {
	for _, code := range []string{
		"bl _xo_rune_to_case\nbl _xo_str_lower\nbl _xo_unicode_ranges\n",
		"bl _xo_str_trim\nbl _xo_str_replace\n",
		"bl _xo_float_parse_ok\nbl _xo_b64_decode\nbl _xo_bytes_hex_ok\n",
		"bl _xo_json_parse\nbl _xo_rs_append_json_float\nbl _xo_rs_append_b64_quoted\n",
	} {
		if got := IOFlags([]byte(code)); len(got) != 0 {
			t.Errorf("IOFlags(%q) = %v, want none", code, got)
		}
	}
}
