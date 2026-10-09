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
