package stdtest

// A differential of std/sql's Postgres driver against pgx's pgconn on
// seeded result sets from a server it does not trust (task #99 follow-up,
// std Proven evidence for sql: decision 0461 asks a harness of every
// parser of untrusted input).
//
// A fake server in this test (plain TCP, no TLS, trust authentication,
// the simple query protocol) answers `SELECT cN` with case N's messages:
// a RowDescription of 1 to 4 columns of the types std/sql maps (bool,
// integers, oid, floats, numeric, text, varchar, bpchar, bytea, json and
// jsonb, date, time, timestamp, timestamptz, interval, uuid, int4[] and
// text[]), DataRows of values near each type's edges and text that is not
// the type's at all, NULLs, NoticeResponses between rows, and an
// ErrorResponse in place of or after rows; then CommandComplete and
// ReadyForQuery. A third of the cases are changed after they are made: a
// message cut, a byte changed, a DataRow with a value too few or too many,
// a value length past its message or below -1, a RowDescription count
// above the fields sent, a message length below 4 or far past the data,
// a DataRow before any RowDescription, or the messages stopped early; the
// server closes the connection after a changed case.
//
// Both clients are scripts/pgdiff's (gopgx, the reference: pgconn for the
// wire and spec/std.md 10.9's mapping of text values; and xo/main.xo), run
// with PG_FRESH=1 (a connection per statement). Their outputs must be
// equal line by line; on a changed case Xo may refuse with an error where
// pgconn answers, but rows it returns must be pgconn's.
//
// XO_PGFUZZ_SEED (default 2), XO_PGFUZZ_N (default 300) and
// XO_PGFUZZ_BACKENDS (default arm64,llvm). The reference builds from the
// Go module cache (GOFLAGS=-mod=mod, GOPROXY=off): github.com/jackc/pgx/v5
// v5.11.0 and its requirements must be in it; the test fails (not skips)
// without them, so evidence never passes on nothing.

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/hostile"
	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

type pgfKind struct {
	name   string
	oid    uint32
	typlen int16
	typmod int32
	vals   []string
}

var pgfKinds = []pgfKind{
	{"bool", 16, 1, -1, []string{"t", "f", "true", "x", ""}},
	{"int2", 21, 2, -1, []string{"0", "-32768", "32767", "32768", "-1", "+5", " 5", "1.5", "abc"}},
	{"int4", 23, 4, -1, []string{"0", "-2147483648", "2147483647", "2147483648", "42", "-0", "1e3"}},
	{"int8", 20, 8, -1, []string{"0", "-9223372036854775808", "9223372036854775807", "9223372036854775808", "-9223372036854775809", "7", "00012", ""}},
	{"oid", 26, 4, -1, []string{"0", "4294967295", "4294967296", "12"}},
	{"float4", 700, 4, -1, []string{"1.5", "-0", "NaN", "Infinity", "-Infinity", "3.4028235e+38", "0.1", "1e-45", "abc"}},
	{"float8", 701, 8, -1, []string{"1.5", "-0", "NaN", "Infinity", "-Infinity", "1e308", "1e309", "5e-324", "1e-400", "0x10", " 1"}},
	{"numeric", 1700, -1, (10<<16 | 4) + 4, []string{"123.4500", "NaN", "-0.001", "0", "99999999999999999999999.5", "Infinity", "-Infinity", "1e5"}},
	{"text", 25, -1, -1, []string{"", "plain", "quote \" and \\ back", "日本語", "line\nbreak", "tab\tx", "nul\x00byte", "bad \xff utf8", "{1,2}", "NULL"}},
	{"varchar", 1043, -1, 12 + 4, []string{"", "abc", "twelve chars", "é", "longer than twelve"}},
	{"bpchar", 1042, -1, 5 + 4, []string{"ab   ", "abcde", ""}},
	{"bytea", 17, -1, -1, []string{`\x`, `\x00ff10`, `\xABcd`, `\x0`, `\xzz`, `abc\000`, "plain"}},
	{"json", 114, -1, -1, []string{`{"a":1}`, `[1,2]`, `"s"`, `null`, `not json`, `{"a":`}},
	{"jsonb", 3802, -1, -1, []string{`{"a": 1}`, `[]`, `1.5e3`, `{"k": "é"}`}},
	{"date", 1082, 4, -1, []string{"2024-01-02", "infinity", "-infinity", "0001-01-01 BC", "2024-02-30", "5874897-12-31"}},
	{"time", 1083, 8, -1, []string{"12:34:56", "12:34:56.789", "24:00:00", "00:00:00.000001"}},
	{"timestamp", 1114, 8, -1, []string{"2024-01-02 03:04:05", "2024-01-02 03:04:05.123456", "infinity", "-infinity", "294276-12-31 23:59:59", "0001-01-01 00:00:00 BC", "2024-13-01 00:00:00", "1970-01-01 00:00:00"}},
	{"timestamptz", 1184, 8, -1, []string{"2024-01-02 03:04:05+00", "2024-01-02 03:04:05.5+05:30", "2024-01-02 03:04:05-08", "1969-12-31 23:59:59.999999+00", "infinity", "2024-01-02 03:04:05+00:00:12", "2024-01-02T03:04:05Z"}},
	{"interval", 1186, 16, -1, []string{"1 day 02:03:04", "-00:00:01", "1 year 2 mons", "00:00:00"}},
	{"uuid", 2950, 16, -1, []string{"a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "00000000-0000-0000-0000-000000000000", "not-a-uuid"}},
	{"int4[]", 1007, -1, -1, []string{"{1,2,NULL}", "{}", "{{1,2},{3,4}}", `{"1"}`, "{1,x}", "{", "{2147483648}"}},
	{"text[]", 1009, -1, -1, []string{`{a,"b c","d\"e",NULL,"NULL"}`, "{}", `{"x\\y"}`, `{""}`, "{a,b"}},
}

type pgfCase struct {
	msgs [][]byte // whole messages, type byte and length included
	note string   // the change made, "" for a case left as made
}

func pgfMsg(typ byte, body []byte) []byte {
	m := []byte{typ, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(m[1:], uint32(len(body)+4))
	return append(m, body...)
}

func pgfCstr(b []byte, s string) []byte { return append(append(b, s...), 0) }

func pgfI16(b []byte, v int16) []byte { return binary.BigEndian.AppendUint16(b, uint16(v)) }
func pgfI32(b []byte, v int32) []byte { return binary.BigEndian.AppendUint32(b, uint32(v)) }

func pgfRowDesc(names []string, kinds []pgfKind, count int) []byte {
	b := pgfI16(nil, int16(count))
	for i, k := range kinds {
		b = pgfCstr(b, names[i])
		b = pgfI32(b, 0)
		b = pgfI16(b, 0)
		b = pgfI32(b, int32(k.oid))
		b = pgfI16(b, k.typlen)
		b = pgfI32(b, k.typmod)
		b = pgfI16(b, 0)
	}
	return pgfMsg('T', b)
}

func pgfDataRow(vals []*string) []byte {
	b := pgfI16(nil, int16(len(vals)))
	for _, v := range vals {
		if v == nil {
			b = pgfI32(b, -1)
			continue
		}
		b = pgfI32(b, int32(len(*v)))
		b = append(b, *v...)
	}
	return pgfMsg('D', b)
}

func pgfNotice(typ byte, code, msg string) []byte {
	b := pgfCstr([]byte{'S'}, "ERROR")
	b = pgfCstr(append(b, 'V'), "ERROR")
	b = pgfCstr(append(b, 'C'), code)
	b = pgfCstr(append(b, 'M'), msg)
	return pgfMsg(typ, append(b, 0))
}

var pgfReady = pgfMsg('Z', []byte{'I'})

func pgfGen(r *rand.Rand) pgfCase {
	ncol := 1 + r.Intn(4)
	kinds := make([]pgfKind, ncol)
	names := make([]string, ncol)
	for i := range kinds {
		kinds[i] = pgfKinds[r.Intn(len(pgfKinds))]
		// distinct names: the Xo client reads a Row's values by name
		names[i] = fmt.Sprintf("c%d", i)
	}
	var msgs [][]byte
	if r.Intn(12) == 0 {
		// an error in place of rows
		msgs = append(msgs, pgfNotice('E', "42703", "column \"x\" does not exist"), pgfReady)
		return pgfCase{msgs: msgs}
	}
	msgs = append(msgs, pgfRowDesc(names, kinds, ncol))
	nrow := r.Intn(5)
	for k := 0; k < nrow; k++ {
		vals := make([]*string, ncol)
		for i, kd := range kinds {
			if r.Intn(8) != 0 {
				v := kd.vals[r.Intn(len(kd.vals))]
				vals[i] = &v
			}
		}
		msgs = append(msgs, pgfDataRow(vals))
		if r.Intn(10) == 0 {
			msgs = append(msgs, pgfNotice('N', "01000", "a notice"))
		}
	}
	if r.Intn(12) == 0 {
		msgs = append(msgs, pgfNotice('E', "22012", "division by zero"), pgfReady)
		return pgfCase{msgs: msgs}
	}
	msgs = append(msgs, pgfMsg('C', pgfCstr(nil, fmt.Sprintf("SELECT %d", nrow))), pgfReady)
	c := pgfCase{msgs: msgs}
	if r.Intn(3) == 0 {
		pgfChange(r, &c, names, kinds)
	}
	return c
}

// pgfChange changes a made case as a hostile server could.
func pgfChange(r *rand.Rand, c *pgfCase, names []string, kinds []pgfKind) {
	var rows []int // the DataRows, by index into msgs
	for i, m := range c.msgs {
		if m[0] == 'D' {
			rows = append(rows, i)
		}
	}
	v := "1"
	switch k := r.Intn(9); {
	case k == 0:
		i := r.Intn(len(c.msgs))
		c.msgs = append(c.msgs[:i:i], c.msgs[i][:r.Intn(len(c.msgs[i]))])
		c.note = "cut"
	case k == 1:
		i := r.Intn(len(c.msgs))
		m := append([]byte(nil), c.msgs[i]...)
		m[r.Intn(len(m))] ^= byte(1 << r.Intn(8))
		c.msgs[i] = m
		c.note = "byte"
	case k == 2 && len(rows) > 0:
		i := rows[r.Intn(len(rows))]
		vals := make([]*string, len(kinds)+pgfPick(r, -1, 1))
		for j := range vals {
			vals[j] = &v
		}
		c.msgs[i] = pgfDataRow(vals)
		c.note = "values"
	case k == 3 && len(rows) > 0:
		i := rows[r.Intn(len(rows))]
		m := append([]byte(nil), c.msgs[i]...)
		// the first value's length: past the message, or below -1
		binary.BigEndian.PutUint32(m[7:], uint32(pgfPick(r, int32(len(m)), 1<<30, -2, -100)))
		if r.Intn(2) == 0 {
			// a hostile int32 (task #352): top bit, all ones, int32's max
			binary.BigEndian.PutUint32(m[7:], uint32(hostile.Pick(r, hostile.Lengths(32))))
		}
		c.msgs[i] = m
		c.note = "value length"
	case k == 4 && r.Intn(2) == 0:
		// a RowDescription's or DataRow's count a hostile uint16 (task
		// #352; pgproto3 reads both with Uint16, 54e535150)
		i := 0
		if len(rows) > 0 && r.Intn(2) == 0 {
			i = rows[r.Intn(len(rows))]
		}
		m := append([]byte(nil), c.msgs[i]...)
		binary.BigEndian.PutUint16(m[5:], uint16(hostile.Pick(r, hostile.Lengths(16))))
		c.msgs[i] = m
		c.note = "hostile count"
	case k == 4:
		c.msgs[0] = pgfRowDesc(names, kinds, len(kinds)+1+r.Intn(3))
		c.note = "column count"
	case k == 5:
		i := r.Intn(len(c.msgs))
		m := append([]byte(nil), c.msgs[i]...)
		binary.BigEndian.PutUint32(m[1:], uint32(pgfPick(r, 0, 3, int32(len(m)+50), 1<<30)))
		if r.Intn(2) == 0 {
			binary.BigEndian.PutUint32(m[1:], uint32(hostile.Pick(r, hostile.Lengths(32))))
		}
		c.msgs[i] = m
		c.note = "message length"
	case k == 6:
		c.msgs = append([][]byte{pgfDataRow([]*string{&v})}, c.msgs...)
		c.note = "row first"
	default:
		c.msgs = c.msgs[:1+r.Intn(len(c.msgs)-1)]
		c.note = "stopped"
	}
}

func pgfPick[T any](r *rand.Rand, xs ...T) T { return xs[r.Intn(len(xs))] }

func pgfServer(l net.Listener, cases []pgfCase) {
	for {
		c, err := l.Accept()
		if err != nil {
			return
		}
		go pgfConn(c, cases)
	}
}

func pgfConn(c net.Conn, cases []pgfCase) {
	defer c.Close()
	br := bufio.NewReader(c)
	for {
		var hdr [8]byte
		if _, err := io.ReadFull(br, hdr[:]); err != nil {
			return
		}
		n := int(binary.BigEndian.Uint32(hdr[:4]))
		code := binary.BigEndian.Uint32(hdr[4:])
		if n < 8 || n > 1<<16 {
			return
		}
		if _, err := io.CopyN(io.Discard, br, int64(n-8)); err != nil {
			return
		}
		if code == 80877103 || code == 80877104 { // SSLRequest, GSSENCRequest
			if _, err := c.Write([]byte{'N'}); err != nil {
				return
			}
			continue
		}
		if code != 196608 { // protocol 3.0
			return
		}
		break
	}
	var w bytes.Buffer
	w.Write(pgfMsg('R', pgfI32(nil, 0)))
	for _, kv := range [][2]string{{"server_version", "17.0"}, {"client_encoding", "UTF8"}, {"server_encoding", "UTF8"}, {"DateStyle", "ISO, MDY"}, {"integer_datetimes", "on"}, {"standard_conforming_strings", "on"}, {"TimeZone", "UTC"}} {
		w.Write(pgfMsg('S', pgfCstr(pgfCstr(nil, kv[0]), kv[1])))
	}
	w.Write(pgfMsg('K', pgfI32(pgfI32(nil, 1), 2)))
	w.Write(pgfReady)
	if _, err := c.Write(w.Bytes()); err != nil {
		return
	}
	for {
		typ, err := br.ReadByte()
		if err != nil {
			return
		}
		var lb [4]byte
		if _, err := io.ReadFull(br, lb[:]); err != nil {
			return
		}
		n := int(binary.BigEndian.Uint32(lb[:]))
		if n < 4 || n > 1<<20 {
			return
		}
		body := make([]byte, n-4)
		if _, err := io.ReadFull(br, body); err != nil {
			return
		}
		switch typ {
		case 'X':
			return
		case 'Q':
			q := strings.TrimRight(string(body), "\x00")
			if strings.HasPrefix(q, "--") {
				// a ping (pgconn's and std/sql's "-- ping"): a comment only
				if _, err := c.Write(append(pgfMsg('I', nil), pgfReady...)); err != nil {
					return
				}
				continue
			}
			i, err := strconv.Atoi(strings.TrimPrefix(q, "SELECT c"))
			if err != nil || i < 0 || i >= len(cases) {
				c.Write(append(pgfNotice('E', "42601", "unknown statement"), pgfReady...))
				continue
			}
			var out []byte
			for _, m := range cases[i].msgs {
				out = append(out, m...)
			}
			if _, err := c.Write(out); err != nil {
				return
			}
			if cases[i].note != "" {
				return // after a changed case the connection ends
			}
		}
	}
}

func TestPgFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the pg fuzz builds pgx and the Xo client")
	}
	seed := int64(envInt(t, "XO_PGFUZZ_SEED", 2))
	n := envInt(t, "XO_PGFUZZ_N", 300)
	backends := envBackends("XO_PGFUZZ_BACKENDS", []string{"arm64", "llvm"})
	r := rand.New(rand.NewSource(seed))
	cases := make([]pgfCase, n)
	for i := range cases {
		cases[i] = pgfGen(r)
	}
	if d := os.Getenv("XO_PGFUZZ_DUMP"); d != "" {
		// one case's messages as hex, to replay it alone
		i, _ := strconv.Atoi(d)
		for _, m := range cases[i].msgs {
			fmt.Printf("%x\n", m)
		}
		t.Logf("case %d: %s", i, cases[i].note)
		return
	}
	pgfDiff(t, cases, backends, seed)
}

// TestPgInvalidName: a column name that is not UTF-8 (`c0\x80`) is
// U+FFFD in both clients' types lines, as in their Row lines; the
// reference printed the raw byte (TestPgFuzz seed 3 case 1528, task #152).
func TestPgInvalidName(t *testing.T) {
	if testing.Short() {
		t.Skip("builds pgx and the Xo client")
	}
	one := "1"
	kinds := []pgfKind{pgfKinds[2]} // int4
	desc := pgfRowDesc([]string{"c0\x80"}, kinds, 1)
	done := [][]byte{pgfMsg('C', pgfCstr(nil, "SELECT 1")), pgfReady}
	cases := []pgfCase{{msgs: append([][]byte{desc, pgfDataRow([]*string{&one})}, done...)}}
	pgfDiff(t, cases, []string{"arm64"}, 0)
}

// TestPgEmptyError: an ErrorResponse with no fields (TestPgFuzz seed 6
// case 909: a DataRow's type byte changed to 'E') is "error  " in both
// clients; scripts/pgdiff's Xo client took the first line of an empty
// message and faulted (task #152).
func TestPgEmptyError(t *testing.T) {
	if testing.Short() {
		t.Skip("builds pgx and the Xo client")
	}
	one := "1"
	kinds := []pgfKind{pgfKinds[2]} // int4
	desc := pgfRowDesc([]string{"c0"}, kinds, 1)
	cases := []pgfCase{{msgs: [][]byte{desc, pgfDataRow([]*string{&one}), pgfMsg('E', []byte{0}), pgfReady}}}
	pgfDiff(t, cases, []string{"arm64"}, 0)
}

// TestPgSelectNoColumns: a SELECT answered with no RowDescription, here
// a ParameterDescription in its place (TestPgFuzz seed 38 case 2618: the
// 'T' byte changed to 't'), is a result of no columns in both clients: the
// reference printed "affected 0" and the Xo client an empty types line.
func TestPgSelectNoColumns(t *testing.T) {
	if testing.Short() {
		t.Skip("builds pgx and the Xo client")
	}
	pd := pgfMsg('t', []byte{0, 4, 0, 0, 0, 23, 0, 0, 0, 25, 0, 0})
	done := [][]byte{pgfMsg('C', pgfCstr(nil, "SELECT 0")), pgfReady}
	cases := []pgfCase{{msgs: append([][]byte{pd}, done...)}, {msgs: done}}
	pgfDiff(t, cases, []string{"arm64"}, 0)
}

// TestPgTimePastTime: a timestamp or timestamptz outside Xo's Time
// (int64 nanoseconds from 1970: about 1677-09-21 to 2262-04-11) is Text in
// std/sql, the server's text; the reference read any year as At (TestPgFuzz
// seed 43 case 472: a byte changed 2024 to 2824, sql evidence, 2026-11-18).
func TestPgTimePastTime(t *testing.T) {
	if testing.Short() {
		t.Skip("builds pgx and the Xo client")
	}
	kinds := []pgfKind{pgfKinds[16], pgfKinds[17]} // timestamp, timestamptz
	desc := pgfRowDesc([]string{"c0", "c1"}, kinds, 2)
	msgs := [][]byte{desc}
	for _, v := range [][2]string{
		{"2824-01-02 03:04:05", "2824-01-02 03:04:05-08"},
		{"2262-04-11 00:00:00", "2262-04-11 00:00:00+00"},
		{"2262-04-12 00:00:00", "2262-04-12 00:00:00+00"},
		{"1677-09-22 00:00:00", "1677-09-22 00:00:00+00"},
		{"1677-09-21 00:00:00", "1677-09-21 00:00:00+00"},
		{"0001-01-01 00:00:00", "9999-12-31 23:59:59.999999+00"},
	} {
		a, b := v[0], v[1]
		msgs = append(msgs, pgfDataRow([]*string{&a, &b}))
	}
	msgs = append(msgs, pgfMsg('C', pgfCstr(nil, "SELECT 6")), pgfReady)
	pgfDiff(t, []pgfCase{{msgs: msgs}}, []string{"arm64"}, 0)
}

// TestPgFracComma: a timestamp or timestamptz with a comma before its
// fraction of a second is At, as pgx reads it (Go's time.Parse takes a
// comma or a period); std/sql gave Text (TestPgFuzz seed 127 case 3956: a
// byte changed '.' to ',', task #152). A comma with no digit after it is
// Text in both.
func TestPgFracComma(t *testing.T) {
	if testing.Short() {
		t.Skip("builds pgx and the Xo client")
	}
	kinds := []pgfKind{pgfKinds[16], pgfKinds[17]} // timestamp, timestamptz
	desc := pgfRowDesc([]string{"c0", "c1"}, kinds, 2)
	msgs := [][]byte{desc}
	for _, v := range [][2]string{
		{"2024-01-02 03:04:05,123456", "2024-01-02 03:04:05,5+05:30"},
		{"2024-01-02 03:04:05,1", "1969-12-31 23:59:59,999999+00"},
		{"2024-01-02 03:04:05,", "2024-01-02 03:04:05,+00"},
	} {
		a, b := v[0], v[1]
		msgs = append(msgs, pgfDataRow([]*string{&a, &b}))
	}
	msgs = append(msgs, pgfMsg('C', pgfCstr(nil, "SELECT 3")), pgfReady)
	pgfDiff(t, []pgfCase{{msgs: msgs}}, []string{"arm64"}, 0)
}

// TestPgDuplicateColumn: a row whose two columns share a name (SELECT 1
// AS c, 2 AS c) shows each column's own value, as pgx's Values does; the
// Xo side read both by name and showed the first twice (TestPgFuzz seed 7
// case 700 on linux/arm64 host 2, 2026-11-18, task #152).
func TestPgDuplicateColumn(t *testing.T) {
	if testing.Short() {
		t.Skip("builds pgx and the Xo client")
	}
	one, two := "1", "2"
	kinds := []pgfKind{pgfKinds[2], pgfKinds[2]} // int4, int4
	desc := pgfRowDesc([]string{"c1", "c1"}, kinds, 2)
	cases := []pgfCase{{msgs: [][]byte{desc, pgfDataRow([]*string{&one, &two}), pgfMsg('C', pgfCstr(nil, "SELECT 1")), pgfReady}}}
	pgfDiff(t, cases, []string{"arm64"}, 0)
}

// pgfDiff serves cases to the reference and to the Xo client on each
// backend and fails on any case whose answers differ (a changed case's
// refusal by Xo aside).
func pgfDiff(t *testing.T, cases []pgfCase, backends []string, seed int64) {
	t.Helper()
	n := len(cases)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go pgfServer(l, cases)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-pgfuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	ref := filepath.Join(work, "gopgx")
	gb := exec.Command("go", "build", "-o", ref, ".")
	gb.Dir = filepath.Join(root, "scripts", "pgdiff", "gopgx")
	gb.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off")
	if out, err := gb.CombinedOutput(); err != nil {
		t.Fatalf("building the reference (github.com/jackc/pgx/v5 v5.11.0 must be in the module cache): %v\n%s", err, out)
	}
	var in strings.Builder
	for i := range cases {
		fmt.Fprintf(&in, "SELECT c%d\n", i)
	}
	host, port, _ := net.SplitHostPort(l.Addr().String())
	dsn := fmt.Sprintf("postgres://u@%s:%s/db?sslmode=disable", host, port)
	run := func(bin string) string {
		cmd := testtmp.Guard(exec.Command(bin, dsn))
		cmd.Stdin = strings.NewReader(in.String())
		cmd.Env = append(os.Environ(), "PG_FRESH=1")
		var out, errb bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v\n%s\nstderr:\n%s", filepath.Base(bin), err, myfTail(out.String(), 20), myfTail(errb.String(), 30))
		}
		return out.String()
	}
	want := myfBlocks(run(ref))
	if len(want) != n {
		t.Fatalf("the reference answered %d of %d cases", len(want), n)
	}
	for _, b := range backends {
		bin := filepath.Join(work, "xo-"+b)
		xb := testtmp.Guard(exec.Command(xo, "build", "--backend="+b, "-o", bin, filepath.Join(root, "scripts", "pgdiff", "xo")))
		xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
		if out, err := xb.CombinedOutput(); err != nil {
			t.Fatalf("xo build --backend=%s: %v\n%s", b, err, out)
		}
		got := myfBlocks(run(bin))
		bad := 0
		refused := map[string]int{}
		for i := 0; i < n; i++ {
			if got[i] == want[i] {
				continue
			}
			if want[i] == "width pgconn\n" && strings.HasPrefix(got[i], "error ") {
				// pgconn keeps a row of another width; Xo refused it
				refused["pgconn row width"]++
				continue
			}
			if cases[i].note != "" && strings.HasPrefix(got[i], "error ") && strings.Count(got[i], "\n") == 1 {
				refused[cases[i].note]++
				continue
			}
			if bad < 10 {
				var pk []string
				for _, p := range cases[i].msgs {
					pk = append(pk, fmt.Sprintf("%x", p))
				}
				t.Errorf("%s case %d (seed %d, %s): got\n%swant\n%smessages %s", b, i, seed, cases[i].note, got[i], want[i], strings.Join(pk, " "))
			}
			bad++
		}
		if bad > 0 {
			t.Fatalf("%s: %d of %d cases differ (seed %d)", b, bad, n, seed)
		}
		t.Logf("%s: %d cases equal pgconn's or refused by Xo (seed %d); refused changed cases by change: %v", b, n, seed, refused)
	}
}

// TestPgRowShape: std/sql's Postgres driver refuses a DataRow of another
// width than its RowDescription, and one before any RowDescription, with
// an error on a broken connection, and a second RowDescription before the
// statement's CommandComplete (it decoded the rows after it by the first
// one's types, TestPgFuzz seed 10 case 3488). It faulted on the first (a mutex
// poisoned in PgConn.next_msg, found by TestPgFuzz seed 2 case 45) and
// returned the second as a row. Needs no reference build.
func TestPgRowShape(t *testing.T) {
	one, two := "1", "2"
	kinds := []pgfKind{pgfKinds[2], pgfKinds[8]} // int4, text
	desc := pgfRowDesc([]string{"c0", "c1"}, kinds, 2)
	done := [][]byte{pgfMsg('C', pgfCstr(nil, "SELECT 1")), pgfReady}
	cases := []pgfCase{
		{msgs: append([][]byte{desc, pgfDataRow([]*string{&one})}, done...), note: "short row"},
		{msgs: append([][]byte{desc, pgfDataRow([]*string{&one, &two, &two})}, done...), note: "long row"},
		{msgs: append([][]byte{pgfDataRow([]*string{&one, &two}), desc}, done...), note: "row first"},
		{msgs: append([][]byte{desc, pgfRowDesc([]string{"d0", "d1"}, []pgfKind{pgfKinds[8], pgfKinds[8]}, 2), pgfDataRow([]*string{&one, &two})}, done...), note: "second RowDescription"},
		{msgs: append([][]byte{desc, pgfDataRow([]*string{&one, &two})}, done...)},
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go pgfServer(l, cases)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-pgshape-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	bin := filepath.Join(work, "xo-pg")
	xb := testtmp.Guard(exec.Command(xo, "build", "-o", bin, filepath.Join(root, "scripts", "pgdiff", "xo")))
	xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
	if out, err := xb.CombinedOutput(); err != nil {
		t.Fatalf("xo build: %v\n%s", err, out)
	}
	host, port, _ := net.SplitHostPort(l.Addr().String())
	cmd := testtmp.Guard(exec.Command(bin, fmt.Sprintf("postgres://u@%s:%s/db?sslmode=disable", host, port)))
	cmd.Stdin = strings.NewReader("SELECT c0\nSELECT c1\nSELECT c2\nSELECT c3\nSELECT c4\n")
	cmd.Env = append(os.Environ(), "PG_FRESH=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("xo client: %v\n%s", err, out)
	}
	got := myfBlocks(string(out))
	if len(got) != len(cases) {
		t.Fatalf("%d answers, want %d:\n%s", len(got), len(cases), out)
	}
	for i, c := range cases[:4] {
		if !strings.HasPrefix(got[i], "error ") {
			t.Errorf("%s: got\n%swant an error", c.note, got[i])
		}
	}
	if !strings.Contains(got[4], `Integer(n: 1), Text(s: "2")`) {
		t.Errorf("a well formed row: got\n%s", got[4])
	}
}
