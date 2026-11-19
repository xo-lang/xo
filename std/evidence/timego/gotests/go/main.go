// Command go is the Go side of std/timego's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package time's result as text;
// std/evidence/timego/gotests/main.xo prints std/timego's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"math"
	"os"
	"strings"
	"time"
)

var layouts = map[string]string{
	"Layout": time.Layout, "ANSIC": time.ANSIC, "UnixDate": time.UnixDate,
	"RubyDate": time.RubyDate, "RFC822": time.RFC822, "RFC822Z": time.RFC822Z,
	"RFC850": time.RFC850, "RFC1123": time.RFC1123, "RFC1123Z": time.RFC1123Z,
	"RFC3339": time.RFC3339, "RFC3339Nano": time.RFC3339Nano, "Kitchen": time.Kitchen,
	"Stamp": time.Stamp, "StampMilli": time.StampMilli, "StampMicro": time.StampMicro,
	"StampNano": time.StampNano, "DateTime": time.DateTime, "DateOnly": time.DateOnly,
	"TimeOnly": time.TimeOnly,
}

// layoutOf is a layout constant's text, or the layout as written.
func layoutOf(s string) string {
	if l, ok := layouts[s]; ok {
		return l
	}
	return s
}

func trun(kind string, in []string) string {
	// Local in Go's time tests (ForceUSPacificForTesting)
	pacific, _ := time.LoadLocation("America/Los_Angeles")
	time.Local = pacific
	var layout, value string
	switch kind {
	case "format":
		// TestFormat's time: Thu Feb  4 21:00:57.012345600 PST 2009
		return time.Unix(0, 1233810057012345600).In(pacific).Format(layoutOf(in[0]))
	case "parse":
		layout, value = layoutOf(in[0]), in[1]
	case "ansic":
		layout, value = time.ANSIC, in[0]
	case "month":
		layout, value = "01-02", in[0]
	case "rfc3339":
		layout, value = time.RFC3339, in[0]
	}
	// timego.parse is ParseInLocation: Local for a text without an offset
	t, err := time.ParseInLocation(layout, value, pacific)
	if err != nil {
		return "err " + err.Error()
	}
	// a Time holds 1677 to 2262 (UnixNano's range): Go's year 0 wraps,
	// timego's is a range error
	if t.Before(time.Unix(0, math.MinInt64)) || t.After(time.Unix(0, math.MaxInt64)) {
		return "err range"
	}
	return fmt.Sprint(t.UnixNano())
}

func run(kind string, in []string) string {
	switch kind {
	case "format", "parse", "ansic", "month", "rfc3339":
		return trun(kind, in)
	}
	return "unknown kind " + kind
}

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<24)
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		in := make([]string, 0, len(f)-1)
		for _, s := range f[1:] {
			b, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			in = append(in, string(b))
		}
		fmt.Fprintln(w, base64.StdEncoding.EncodeToString([]byte(run(f[0], in))))
	}
}
