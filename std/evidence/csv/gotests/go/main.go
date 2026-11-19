// Command go is the Go side of std/csv's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package csv's result as text;
// std/evidence/csv/gotests/main.xo prints std/csv's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func parseCase(in []string) string {
	text := strings.NewReplacer("\u00a7", "", "\u00b6", "", "\u2211", "").Replace(in[0])
	r := csv.NewReader(strings.NewReader(text))
	if c, _ := strconv.Atoi(in[1]); c != 0 {
		r.Comma = rune(c)
	}
	c, _ := strconv.Atoi(in[2])
	r.Comment = rune(c)
	r.FieldsPerRecord = -1
	if in[3] == "true" {
		r.FieldsPerRecord, _ = strconv.Atoi(in[4])
	}
	r.LazyQuotes = in[5] == "true"
	r.TrimLeadingSpace = in[6] == "true"
	rows, err := r.ReadAll()
	if err != nil {
		var pe *csv.ParseError
		if errors.As(err, &pe) {
			return fmt.Sprintf("err %d %d %d %s", pe.StartLine, pe.Line, pe.Column, pe.Err)
		}
		return "err " + err.Error()
	}
	var b strings.Builder
	for _, rec := range rows {
		b.WriteString("R")
		for _, f := range rec {
			fmt.Fprintf(&b, "|%d:%s", len(f), f)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func run(kind string, in []string) string {
	switch kind {
	case "read":
		return parseCase(in)
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
