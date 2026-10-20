// Reference for the Postgres differential test (scripts/pg-diff.sh): each statement through
// pgx's pgconn (simple protocol), its last result printed with spec/std.md
// 10.9's mapping of text values by type OID, in the Debug form of the
// native std/sql Row, or the error's SQLSTATE and message.
package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func q(s string) string { return strconv.Quote(s) }

func elemOf(oid uint32) int64 {
	m := map[uint32]int64{1000: 16, 1001: 17, 1003: 19, 1005: 21, 1007: 23, 1016: 20, 1009: 25, 1014: 1042, 1015: 1043, 1021: 700, 1022: 701, 1028: 26, 199: 114, 3807: 3802, 1115: 1114, 1185: 1184}
	if e, ok := m[oid]; ok {
		return e
	}
	return -1
}

func xoFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "+Inf"
	case math.IsInf(f, -1):
		return "-Inf"
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eEn") {
		s += ".0"
	}
	return s
}

func iso(s string, utc bool) (time.Time, bool) {
	t := strings.Replace(s, " ", "T", 1)
	if utc {
		t += "Z"
	} else if i := strings.LastIndexAny(t, "+-"); i > 10 {
		if len(t)-i == 3 {
			t += ":00"
		}
	}
	x, err := time.Parse(time.RFC3339Nano, t)
	return x, err == nil
}

func val(oid uint32, s string) string {
	switch oid {
	case 16:
		return fmt.Sprintf("Flag(b: %v)", s == "t")
	case 20, 21, 23, 26:
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return fmt.Sprintf("Integer(n: %d)", n)
		}
	case 700, 701:
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return "Real(f: " + xoFloat(f) + ")"
		}
	case 17:
		if strings.HasPrefix(s, `\x`) {
			if b, err := hex.DecodeString(s[2:]); err == nil {
				return "Blob(b: b" + q(string(b))[0:] + ")"
			}
		}
	case 114, 3802:
		return "JsonText(s: " + q(s) + ")"
	case 1184, 1114:
		if t, ok := iso(s, oid == 1114); ok {
			return "At(t: " + t.UTC().Format(time.RFC3339Nano) + ")"
		}
	}
	if e := elemOf(oid); e >= 0 {
		if items, ok := parseArray(s, uint32(e)); ok {
			return "Array(items: [" + strings.Join(items, ", ") + "])"
		}
	}
	return "Text(s: " + q(s) + ")"
}

func parseArray(s string, elem uint32) ([]string, bool) {
	if len(s) < 2 || s[0] != '{' || s[len(s)-1] != '}' {
		return nil, false
	}
	out := []string{}
	if len(s) == 2 {
		return out, true
	}
	i := 1
	for i < len(s)-1 {
		if s[i] == '{' {
			return nil, false
		}
		var cur strings.Builder
		quoted := false
		if s[i] == '"' {
			quoted = true
			i++
			for i < len(s)-1 && s[i] != '"' {
				if s[i] == '\\' && i+1 < len(s) {
					i++
				}
				cur.WriteByte(s[i])
				i++
			}
			i++
		} else {
			for i < len(s)-1 && s[i] != ',' {
				cur.WriteByte(s[i])
				i++
			}
		}
		t := cur.String()
		if !quoted && t == "NULL" {
			out = append(out, "Null")
		} else {
			out = append(out, val(elem, t))
		}
		if i < len(s)-1 && s[i] == ',' {
			i++
		}
	}
	return out, true
}

func main() {
	ctx := context.Background()
	c, err := pgconn.Connect(ctx, os.Args[1])
	if err != nil {
		fmt.Println("connect:", err)
		os.Exit(1)
	}
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		res, err := c.Exec(ctx, line).ReadAll()
		fmt.Println("== " + line)
		var pe *pgconn.PgError
		if errors.As(err, &pe) {
			fmt.Printf("error %s %s\n", pe.Code, pe.Message)
			continue
		} else if err != nil {
			fmt.Println("error", err)
			continue
		}
		r := res[len(res)-1]
		if len(r.FieldDescriptions) == 0 {
			fmt.Printf("affected %d\n", r.CommandTag.RowsAffected())
			continue
		}
		var cols []string
		for _, f := range r.FieldDescriptions {
			cols = append(cols, q(f.Name))
		}
		for _, row := range r.Rows {
			var vals []string
			for i, v := range row {
				if v == nil {
					vals = append(vals, "Null")
				} else {
					vals = append(vals, val(r.FieldDescriptions[i].DataTypeOID, string(v)))
				}
			}
			fmt.Printf("Row{cols: [%s], vals: [%s]}\n", strings.Join(cols, ", "), strings.Join(vals, ", "))
		}
	}
}
