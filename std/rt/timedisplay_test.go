package rt

import "testing"

// TestTimeErrDisplay pins the display text of the std/time errors
// (std.md 13), including ZoneErr.NoLocal, which no golden program can
// reach on a machine with a zone.
func TestTimeErrDisplay(t *testing.T) {
	cases := []struct {
		got  string
		want string
	}{
		{Time_ZoneErr{Tag: 1, P: Time_ZoneErr_Unknown{F_name: "Mars/Olympus"}}.M_display(nil), `unknown time zone "Mars/Olympus"`},
		{Time_ZoneErr{Tag: 2, P: Time_ZoneErr_NoLocal{F_msg: "the system zone is not set"}}.M_display(nil), "cannot determine the local time zone: the system zone is not set"},
		{civilErr(1, "month 13 out of range 1 to 12").M_display(nil), "invalid civil time: month 13 out of range 1 to 12"},
		{civilErr(2, "2026-03-08T02:30:00 America/New_York").M_display(nil), "2026-03-08T02:30:00 America/New_York does not exist (skipped by a daylight saving change)"},
		{civilErr(3, "2026-11-01T01:30:00 America/New_York").M_display(nil), "2026-11-01T01:30:00 America/New_York happens twice (repeated by a daylight saving change)"},
		{Time_ParseErr{F_input: "2026-1x-09", F_layout: "{year}-{month}-{day}", F_at: 5, F_msg: "expected 2 digit month"}.M_display(nil), `at byte 5 of "2026-1x-09": expected 2 digit month`},
		{Time_LayoutErr{F_layout: "{yr}", F_msg: "unknown field {yr}"}.M_display(nil), `bad layout "{yr}": unknown field {yr}`},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("display %q, want %q", c.got, c.want)
		}
	}
}
