package check

import (
	"testing"

	"github.com/xo-lang/xo/std/rt"
)

// TestTimeLayoutAgrees keeps the compile time layout check equal to the
// runtime's layout parser (std/rt/timefmt.go): the same layouts are
// rejected with the same message at the same offset.
func TestTimeLayoutAgrees(t *testing.T) {
	layouts := []string{
		"", "plain text", rt.Time_RFC3339, rt.Time_RFC1123, rt.Time_DATE, rt.Time_TIME,
		"{year}-{month}-{day}", "{yaer}", "{year", "year}", "{}", "{:}", "{month:}", "{month:x}",
		"{month:1}{month:short}{month:name}{day:1}", "{weekday}", "{weekday:short}{weekday:name}",
		"{hour12}", "{hour12}{ampm}", "{ampm}", "{frac}{frac:1}{frac:9}", "{frac:0}", "{frac:10}",
		"{offset}{offset:compact}{offset:long}", "{zone}{zone:x}", "{lbrace}{rbrace}", "{lbrace:x}",
		"{year}{", "{{year}}", "}{", "{yyyy}-{MM}", "%Y-%m-%d", "{hour12} {hour12}",
	}
	for _, l := range layouts {
		_, rmsg, rat := rt.TimeLayoutParse(l)
		cmsg, cat := TimeLayoutError(l)
		if rmsg != cmsg || (rmsg != "" && rat != cat) {
			t.Errorf("layout %q: runtime says %q at %d, checker %q at %d", l, rmsg, rat, cmsg, cat)
		}
	}
}

// TestTimeLayoutRules: constant std/time layouts (XO0420, XO0177) and the
// layouts the checker leaves to run time.
func TestTimeLayoutRules(t *testing.T) {
	pre := "use std/time\n\nfn f(t: Time, s: Str) -> Str {\n  "
	runCases(t, []ruleCase{
		{name: "unknown field", src: pre + "time.format(t, time.utc(), \"{yaer}\")\n}\n", want: []string{"XO0420"}},
		{name: "bad field form", src: pre + "time.format(t, time.utc(), \"{month:long}\")\n}\n", want: []string{"XO0420"}},
		{name: "stray brace", src: pre + "time.format(t, time.utc(), \"{year} }\")\n}\n", want: []string{"XO0420"}},
		{name: "hour12 without ampm", src: pre + "time.format(t, time.utc(), \"{hour12}\")\n}\n", want: []string{"XO0420"}},
		{name: "bad const layout", src: "use std/time\n\nconst L: Str = \"{dya}\"\n\nfn f(t: Time) -> Str {\n  time.format(t, time.utc(), L)\n}\n", want: []string{"XO0420"}},
		{name: "check_layout constant", src: pre + "if time.check_layout(\"{frac:0}\").is_ok() { s } else { \"\" }\n}\n", want: []string{"XO0420"}},
		{name: "letter habit", src: pre + "time.format(t, time.utc(), \"{yyyy}-{MM}\")\n}\n", want: []string{"XO0177"}},
		{name: "strftime habit", src: pre + "time.format(t, time.utc(), \"%Y-%m-%d\")\n}\n", want: []string{"XO0177"}},
		{name: "go reference habit", src: pre + "time.format(t, time.utc(), \"2006-01-02\")\n}\n", want: []string{"XO0177"}},
		{name: "valid layout", src: pre + "time.format(t, time.utc(), \"{year}-{month}-{day} {lbrace}{rbrace}\")\n}\n"},
		{name: "std constant", src: pre + "time.format(t, time.utc(), time.RFC1123)\n}\n"},
		{name: "run time layout", src: pre + "time.format(t, time.utc(), \"{\" + s + \"}\")\n}\n"},
		{name: "plain text layout", src: pre + "time.format(t, time.utc(), \"Date: {year}\")\n}\n"},
	})
}
