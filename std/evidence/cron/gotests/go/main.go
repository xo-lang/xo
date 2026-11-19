// Command go is the Go side of std/cron's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package cron's result as text;
// std/evidence/cron/gotests/main.xo prints std/cron's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"github.com/robfig/cron/v3"
	"os"
	"strings"
	"time"
)

var secondParser = cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.DowOptional | cron.Descriptor)

const iso = "2006-01-02T15:04:05-0700"

// getTime is the test's getTime and getTimeTZ with Local set to UTC.
func getTime(value string) (time.Time, bool) {
	location := time.UTC
	if strings.HasPrefix(value, "TZ=") {
		parts := strings.Fields(value)
		loc, err := time.LoadLocation(parts[0][len("TZ="):])
		if err != nil {
			return time.Time{}, false
		}
		location = loc
		value = parts[1]
	}
	for _, layout := range []string{"Mon Jan 2 15:04 2006", "Mon Jan 2 15:04:05 2006", iso} {
		if t, err := time.ParseInLocation(layout, value, location); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func next(p cron.Parser, tv, spec string, back bool) string {
	t, ok := getTime(tv)
	if !ok {
		return "bad time"
	}
	s, err := p.Parse(spec)
	if err != nil {
		return "err " + err.Error()
	}
	from := t
	if back {
		from = t.Add(-time.Second)
	}
	n := s.Next(from)
	if n.IsZero() {
		return "zero"
	}
	return n.In(t.Location()).Format(iso)
}

func perr(spec string) string {
	if _, err := secondParser.Parse(spec); err != nil {
		return "err " + err.Error()
	}
	return "ok"
}

func run(kind string, in []string) string {
	switch kind {
	case "act":
		return next(cron.NewParser(cron.Minute|cron.Hour|cron.Dom|cron.Month|cron.Dow|cron.Descriptor), in[0], in[1], true)
	case "next":
		return next(secondParser, in[0], in[1], false)
	case "nextstd":
		return next(cron.NewParser(cron.Minute|cron.Hour|cron.Dom|cron.Month|cron.Dow|cron.Descriptor), in[0], in[1], false)
	case "perr":
		return perr(in[0])
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
