package stdtest

// A differential of std/sql's MySQL driver against go-sql-driver/mysql on
// seeded result sets from a server it does not trust (2026-11-15, std
// Proven evidence for sql: decision 0461 asks a harness of every parser
// of untrusted input).
//
// A fake server in this test (plain TCP, mysql_native_password, classic
// EOF packets) answers `SELECT cN` with case N's packets: a column count,
// column definitions of every text-protocol type (signed and unsigned
// integers, FLOAT, DOUBLE, DECIMAL, DATE, DATETIME, TIMESTAMP, TIME,
// YEAR, BIT, JSON, text and binary strings), rows of values near each
// type's edges, NULLs, and an EOF or an ERR packet in the rows. A third of
// the cases are changed after they are made: a packet cut, a byte changed,
// a row short of a value or with one too many, a column count above the
// definitions sent, a bad length-encoded integer, a NULL in a NOT NULL
// column, text that is not a number in a numeric column, or the packets
// stopped early; the server closes the connection after a changed case,
// so neither client waits for bytes that never come.
//
// Both clients are scripts/mysqldiff's (gomysql, the reference, its own Go
// module, and xo/main.xo), which print each SELECT's column types and rows
// in decision 0648 item 8's form, or the error in item 9's form ("error
// conn" for a broken connection). The outputs must be equal line by line.
//
// XO_MYSQLFUZZ_SEED (default 2), XO_MYSQLFUZZ_N (default 300) and
// XO_MYSQLFUZZ_BACKENDS (default arm64,llvm). The reference builds from the
// Go module cache (GOFLAGS=-mod=mod, GOPROXY=off): go-sql-driver/mysql
// v1.10.1 and filippo.io/edwards25519 v1.2.0 must be in it; the test fails
// (not skips) without them, so evidence never passes on nothing.

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// MySQL column types and flags the cases use.
const (
	myTiny      = 1
	myShort     = 2
	myLong      = 3
	myFloat     = 4
	myDouble    = 5
	myTimestamp = 7
	myLongLong  = 8
	myInt24     = 9
	myDate      = 10
	myTime      = 11
	myDatetime  = 12
	myYear      = 13
	myBit       = 16
	myJSON      = 245
	myDecimal   = 246
	myBlob      = 252
	myVarString = 253
	myString    = 254

	flagNotNull  = 1
	flagUnsigned = 32
	flagBinary   = 128
)

// myfCol is one column definition of a case.
type myfCol struct {
	name     string
	kind     int
	flags    int
	charset  int
	decimals int
}

// myfCase is one case: the packets the server sends for its SELECT and
// whether it closes the connection after them.
type myfCase struct {
	packets [][]byte
	close   bool
	note    string
}

func lenencInt(n int) []byte {
	switch {
	case n < 251:
		return []byte{byte(n)}
	case n < 1<<16:
		return []byte{0xfc, byte(n), byte(n >> 8)}
	case n < 1<<24:
		return []byte{0xfd, byte(n), byte(n >> 8), byte(n >> 16)}
	}
	b := make([]byte, 9)
	b[0] = 0xfe
	binary.LittleEndian.PutUint64(b[1:], uint64(n))
	return b
}

func lenencStr(s []byte) []byte { return append(lenencInt(len(s)), s...) }

func myfColumnDef(c myfCol) []byte {
	var b []byte
	for _, s := range []string{"def", "db", "t", "t", c.name, c.name} {
		b = append(b, lenencStr([]byte(s))...)
	}
	b = append(b, 0x0c)
	b = binary.LittleEndian.AppendUint16(b, uint16(c.charset))
	b = binary.LittleEndian.AppendUint32(b, 255)
	b = append(b, byte(c.kind))
	b = binary.LittleEndian.AppendUint16(b, uint16(c.flags))
	b = append(b, byte(c.decimals), 0, 0)
	return b
}

var myfEOF = []byte{0xfe, 0, 0, 2, 0}

// myfValue is a text-protocol value of column c near its type's edges.
func myfValue(r *rand.Rand, c myfCol) []byte {
	pick := func(xs ...string) []byte { return []byte(xs[r.Intn(len(xs))]) }
	unsigned := c.flags&flagUnsigned != 0
	switch c.kind {
	case myTiny:
		if unsigned {
			return pick("0", "1", "127", "128", "255")
		}
		return pick("-128", "-1", "0", "1", "127")
	case myShort, myYear:
		if c.kind == myYear {
			return pick("0000", "1901", "2024", "2155")
		}
		if unsigned {
			return pick("0", "65535", "32768")
		}
		return pick("-32768", "0", "32767")
	case myInt24, myLong:
		if unsigned {
			return pick("0", "4294967295", "16777215")
		}
		return pick("-2147483648", "-8388608", "0", "2147483647")
	case myLongLong:
		if unsigned {
			return pick("0", "9223372036854775807", "9223372036854775808", "18446744073709551615")
		}
		return pick("-9223372036854775808", "-1", "0", "9223372036854775807", strconv.FormatInt(r.Int63(), 10))
	case myFloat:
		return pick("0", "-0", "1.5", "3.4028235e38", "1e-45", "0.1", "-1.17549435e-38")
	case myDouble:
		return pick("0", "1.7976931348623157e308", "5e-324", "0.1", "-2.5", "1e21", strconv.FormatFloat(r.NormFloat64()*1e6, 'g', -1, 64))
	case myDecimal:
		return pick("0.00", "-12345.678", "99999999999999999999.99", "0.000001", "1")
	case myDate:
		return pick("2024-02-29", "1970-01-01", "0000-00-00", "1000-01-01", "9999-12-31", "1677-09-21")
	case myDatetime, myTimestamp:
		if c.decimals > 0 {
			return pick("2024-02-29 23:59:59."+strings.Repeat("9", c.decimals), "1970-01-01 00:00:00."+strings.Repeat("0", c.decimals), "0000-00-00 00:00:00."+strings.Repeat("0", c.decimals))
		}
		return pick("2024-02-29 23:59:59", "1970-01-01 00:00:00", "0000-00-00 00:00:00", "2262-04-11 23:47:16", "1677-09-21 00:12:44")
	case myTime:
		return pick("00:00:00", "-838:59:59", "838:59:59", "12:34:56.789")
	case myBit:
		n := 1 + r.Intn(8)
		b := make([]byte, n)
		r.Read(b)
		return b
	case myJSON:
		return pick(`{}`, `{"a": [1, 2.5, null, true]}`, `"é"`, `[]`)
	case myBlob, myString, myVarString:
		if c.charset == 63 {
			b := make([]byte, r.Intn(300))
			r.Read(b)
			return b
		}
		return pick("", "a", "héllo", "tab\there", "nul\x00byte", strings.Repeat("x", 251), strings.Repeat("y", 70000))
	}
	return []byte("?")
}

var myfKinds = []myfCol{
	{kind: myTiny}, {kind: myTiny, flags: flagUnsigned}, {kind: myShort}, {kind: myShort, flags: flagUnsigned},
	{kind: myInt24}, {kind: myLong}, {kind: myLong, flags: flagUnsigned}, {kind: myLongLong}, {kind: myLongLong, flags: flagUnsigned},
	{kind: myFloat, decimals: 31}, {kind: myDouble, decimals: 31}, {kind: myDecimal, decimals: 2}, {kind: myDate}, {kind: myDatetime},
	{kind: myDatetime, decimals: 3}, {kind: myTimestamp, decimals: 6}, {kind: myTime}, {kind: myYear, flags: flagUnsigned},
	{kind: myBit, flags: flagUnsigned | flagBinary, charset: 63}, {kind: myJSON, flags: flagBinary, charset: 63},
	{kind: myVarString}, {kind: myString}, {kind: myBlob}, {kind: myBlob, flags: flagBinary, charset: 63}, {kind: myVarString, flags: flagBinary, charset: 63},
}

// myfErr is an ERR packet (protocol 4.1).
func myfErr(code int, state, msg string) []byte {
	b := []byte{0xff}
	b = binary.LittleEndian.AppendUint16(b, uint16(code))
	return append(append(b, '#'), []byte(state+msg)...)
}

// myfGen makes case i: a result set, changed for a third of the cases.
func myfGen(r *rand.Rand) myfCase {
	ncol := 1 + r.Intn(4)
	cols := make([]myfCol, ncol)
	for i := range cols {
		c := myfKinds[r.Intn(len(myfKinds))]
		if c.charset == 0 {
			c.charset = 255
		}
		if r.Intn(4) == 0 {
			c.flags |= flagNotNull
		}
		c.name = fmt.Sprintf("c%d", i)
		if r.Intn(8) == 0 {
			c.name = "ñ é"
		}
		cols[i] = c
	}
	var p [][]byte
	p = append(p, lenencInt(ncol))
	for _, c := range cols {
		p = append(p, myfColumnDef(c))
	}
	p = append(p, myfEOF)
	nrow := r.Intn(5)
	for k := 0; k < nrow; k++ {
		var row []byte
		for _, c := range cols {
			if c.flags&flagNotNull == 0 && r.Intn(6) == 0 {
				row = append(row, 0xfb)
				continue
			}
			row = append(row, lenencStr(myfValue(r, c))...)
		}
		p = append(p, row)
	}
	if r.Intn(8) == 0 {
		p = append(p, myfErr(1317, "70100", "Query execution was interrupted"))
	} else {
		p = append(p, myfEOF)
	}
	cs := myfCase{packets: p}
	if r.Intn(3) != 0 {
		return cs
	}
	cs.close = true
	at := r.Intn(len(p))
	switch m := r.Intn(9); m {
	case 0: // a packet cut
		if len(p[at]) > 0 {
			p[at] = p[at][:r.Intn(len(p[at]))]
		}
		cs.note = "cut"
	case 1: // a byte changed
		if len(p[at]) > 0 {
			p[at] = append([]byte{}, p[at]...)
			p[at][r.Intn(len(p[at]))] = byte(r.Intn(256))
		}
		cs.note = "byte"
	case 2: // a row short of a value
		if nrow > 0 && ncol > 1 {
			row := 2 + ncol
			var rr []byte
			for _, c := range cols[1:] {
				rr = append(rr, lenencStr(myfValue(r, c))...)
			}
			p[row] = rr
		}
		cs.note = "short row"
	case 3: // a row with a value too many
		if nrow > 0 {
			p[2+ncol] = append(append([]byte{}, p[2+ncol]...), lenencStr([]byte("extra"))...)
		}
		cs.note = "long row"
	case 4: // a column count above the definitions sent
		p[0] = lenencInt(ncol + 1)
		cs.note = "count"
	case 5: // a bad length-encoded integer at a row's start
		if nrow > 0 {
			p[2+ncol] = append([]byte{0xfd, 0xff, 0xff}, p[2+ncol]...)
		}
		cs.note = "lenenc"
	case 6: // a NULL in a NOT NULL column
		if nrow > 0 {
			rr := []byte{0xfb}
			for _, d := range cols[1:] {
				rr = append(rr, lenencStr(myfValue(r, d))...)
			}
			cols[0].flags |= flagNotNull
			p[1] = myfColumnDef(cols[0])
			p[2+ncol] = rr
		}
		cs.note = "null"
	case 7: // text that is not a number in a numeric column
		for i, c := range cols {
			if c.kind <= myInt24 && c.kind != myTimestamp && nrow > 0 {
				var rr []byte
				for j, d := range cols {
					v := myfValue(r, d)
					if j == i {
						v = []byte("12a")
					}
					rr = append(rr, lenencStr(v)...)
				}
				p[2+ncol] = rr
				break
			}
		}
		cs.note = "nan"
	case 8: // the packets stopped early
		p = p[:at]
		cs.note = "stop"
	}
	cs.packets = p
	return cs
}

// myfServer serves the cases until l closes: any connection, any number of
// times (each client reconnects after a closed connection).
func myfServer(l net.Listener, cases []myfCase) {
	for {
		c, err := l.Accept()
		if err != nil {
			return
		}
		go myfConn(c, cases)
	}
}

func myfWrite(w io.Writer, seq int, payload []byte) error {
	h := []byte{byte(len(payload)), byte(len(payload) >> 8), byte(len(payload) >> 16), byte(seq)}
	_, err := w.Write(append(h, payload...))
	return err
}

func myfRead(r *bufio.Reader) ([]byte, error) {
	var h [4]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, err
	}
	n := int(h[0]) | int(h[1])<<8 | int(h[2])<<16
	b := make([]byte, n)
	_, err := io.ReadFull(r, b)
	return b, err
}

var myfOK = []byte{0, 0, 0, 2, 0, 0, 0}

func myfConn(c net.Conn, cases []myfCase) {
	defer c.Close()
	const caps = 1 | 2 | 4 | 8 | 0x200 | 0x2000 | 0x8000 | 0x10000 | 0x20000 | 0x40000 | 0x80000 | 0x200000
	hs := []byte{10}
	hs = append(hs, []byte("8.4.0-fuzz\x00")...)
	hs = binary.LittleEndian.AppendUint32(hs, 7)
	hs = append(hs, []byte("abcdefgh\x00")...)
	hs = binary.LittleEndian.AppendUint16(hs, uint16(caps&0xffff))
	hs = append(hs, 255)
	hs = binary.LittleEndian.AppendUint16(hs, 2)
	hs = binary.LittleEndian.AppendUint16(hs, uint16(caps>>16))
	hs = append(hs, 21)
	hs = append(hs, make([]byte, 10)...)
	hs = append(hs, []byte("ijklmnopqrst\x00")...)
	hs = append(hs, []byte("mysql_native_password\x00")...)
	r := bufio.NewReader(c)
	if myfWrite(c, 0, hs) != nil {
		return
	}
	if _, err := myfRead(r); err != nil {
		return
	}
	if myfWrite(c, 2, myfOK) != nil {
		return
	}
	for {
		cmd, err := myfRead(r)
		if err != nil || len(cmd) == 0 || cmd[0] == 1 { // COM_QUIT
			return
		}
		q := string(cmd[1:])
		if cmd[0] != 3 || !strings.HasPrefix(q, "SELECT c") {
			if myfWrite(c, 1, myfOK) != nil {
				return
			}
			continue
		}
		i, err := strconv.Atoi(strings.TrimPrefix(q, "SELECT c"))
		if err != nil || i < 0 || i >= len(cases) {
			myfWrite(c, 1, myfErr(1064, "42000", "no such case"))
			continue
		}
		var buf bytes.Buffer
		for k, p := range cases[i].packets {
			myfWrite(&buf, 1+k, p)
		}
		if _, err := c.Write(buf.Bytes()); err != nil || cases[i].close {
			return
		}
	}
}

// myfBlocks splits a client's output into its "== SELECT cN" blocks.
// myfSameButBinary reports whether two answers differ, and are equal once
// binary types and values read as their text forms (VARBINARY as VARCHAR,
// BINARY as CHAR, BLOB as TEXT, a Blob value as a Text value).
func myfSameButBinary(a, b string) bool {
	r := strings.NewReplacer(":VARBINARY:", ":VARCHAR:", ":BINARY:", ":CHAR:", ":BLOB:", ":TEXT:", `Blob(b: b"`, `Text(s: "`)
	return a != b && r.Replace(a) == r.Replace(b)
}

// myfWire is a column definition's collation, type and flags.
type myfWire struct{ collation, kind, flags int }

// myfColumns reads the column definitions among a result's packets (the
// column count, then the definitions); nil when they do not parse.
func myfColumns(packets [][]byte) []myfWire {
	if len(packets) < 2 || len(packets[0]) == 0 {
		return nil
	}
	n := int(packets[0][0])
	var out []myfWire
	for _, p := range packets[1:min(len(packets), 1+n)] {
		pos := 0
		for range 6 {
			if pos >= len(p) || p[pos] >= 251 {
				return nil
			}
			pos += 1 + int(p[pos])
		}
		if pos+10 > len(p) {
			return nil
		}
		out = append(out, myfWire{collation: int(p[pos+1]) | int(p[pos+2])<<8, kind: int(p[pos+7]), flags: int(p[pos+8]) | int(p[pos+9])<<8})
	}
	return out
}

// myfWideCollation reports whether a column definition among a result's
// packets has a collation above 255, which go-sql-driver reads as its low
// byte.
func myfWideCollation(packets [][]byte) bool {
	for _, c := range myfColumns(packets) {
		if c.collation > 255 {
			return true
		}
	}
	return false
}

var myfDecimal = regexp.MustCompile(`:DECIMAL:(\d+),`)

// myfUnsignedDecimal reports whether two answers differ only in DECIMAL
// precisions on their "types " line, each Xo's (got) one more than
// go-sql-driver's (want) exactly where the column is UNSIGNED.
func myfUnsignedDecimal(got, want string, packets [][]byte) bool {
	lg, lw := strings.Split(got, "\n"), strings.Split(want, "\n")
	if len(lg) != len(lw) || len(lg) == 0 || !strings.HasPrefix(lg[0], "types ") || strings.Join(lg[1:], "\n") != strings.Join(lw[1:], "\n") {
		return false
	}
	var dec []bool // each DECIMAL column's UNSIGNED flag, in order
	for _, c := range myfColumns(packets) {
		if c.kind == 0 || c.kind == 246 {
			dec = append(dec, c.flags&32 != 0)
		}
	}
	mg, mw := myfDecimal.FindAllStringSubmatchIndex(lg[0], -1), myfDecimal.FindAllStringSubmatchIndex(lw[0], -1)
	if len(mg) != len(dec) || len(mw) != len(dec) || myfDecimal.ReplaceAllString(lg[0], ":DECIMAL:") != myfDecimal.ReplaceAllString(lw[0], ":DECIMAL:") {
		return false
	}
	diff := false
	for k := range dec {
		pg, _ := strconv.Atoi(lg[0][mg[k][2]:mg[k][3]])
		pw, _ := strconv.Atoi(lw[0][mw[k][2]:mw[k][3]])
		switch {
		case dec[k] && pg == pw+1:
			diff = true
		case pg != pw:
			return false
		}
	}
	return diff
}

func myfBlocks(out string) map[int]string {
	m := map[int]string{}
	cur, k := -1, ""
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "== SELECT c") {
			if cur >= 0 {
				m[cur] = k
			}
			cur, _ = strconv.Atoi(strings.TrimPrefix(l, "== SELECT c"))
			k = ""
			continue
		}
		// go-sql-driver's own log lines ("[mysql] ...") go to stderr; a
		// stray one on stdout is not a value
		if cur >= 0 && l != "" && !strings.HasPrefix(l, "[mysql] ") {
			k += l + "\n"
		}
	}
	if cur >= 0 {
		m[cur] = k
	}
	return m
}

func TestMysqlFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the mysql fuzz builds go-sql-driver and the Xo client")
	}
	seed := int64(envInt(t, "XO_MYSQLFUZZ_SEED", 2))
	n := envInt(t, "XO_MYSQLFUZZ_N", 300)
	backends := envBackends("XO_MYSQLFUZZ_BACKENDS", []string{"arm64", "llvm"})
	r := rand.New(rand.NewSource(seed))
	cases := make([]myfCase, n)
	for i := range cases {
		cases[i] = myfGen(r)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go myfServer(l, cases)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-mysqlfuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	ref := filepath.Join(work, "gomysql")
	gb := exec.Command("go", "build", "-o", ref, ".")
	gb.Dir = filepath.Join(root, "scripts", "mysqldiff", "gomysql")
	gb.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off")
	if out, err := gb.CombinedOutput(); err != nil {
		t.Fatalf("building the reference (go-sql-driver/mysql v1.10.1 and filippo.io/edwards25519 v1.2.0 must be in the module cache): %v\n%s", err, out)
	}
	var in strings.Builder
	for i := range cases {
		fmt.Fprintf(&in, "SELECT c%d\n", i)
	}
	dsn := fmt.Sprintf("root@tcp(%s)/db?tls=false&parseTime=true", l.Addr())
	run := func(bin string) string {
		cmd := testtmp.Guard(exec.Command(bin, dsn))
		cmd.Stdin = strings.NewReader(in.String())
		cmd.Env = append(os.Environ(), "MY_FRESH=1")
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
	var mu sync.Mutex
	for _, b := range backends {
		bin := filepath.Join(work, "xo-"+b)
		xb := testtmp.Guard(exec.Command(xo, "build", "--backend="+b, "-o", bin, filepath.Join(root, "scripts", "mysqldiff", "xo")))
		xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
		if out, err := xb.CombinedOutput(); err != nil {
			t.Fatalf("xo build --backend=%s: %v\n%s", b, err, out)
		}
		got := myfBlocks(run(bin))
		bad := 0
		refused := map[string]int{}
		for i := 0; i < n; i++ {
			// a changed case: Xo may refuse what go-sql-driver takes (it
			// ends the rows at a length past the packet, as if they ended,
			// and panics on some definitions), but rows it returns must
			// be go-sql-driver's
			if got[i] != want[i] && want[i] == "panic go-sql-driver\n" {
				// no reference answer; Xo answered without a fault
				refused["go panic"]++
				continue
			}
			if got[i] != want[i] && myfSameButBinary(got[i], want[i]) && myfWideCollation(cases[i].packets) {
				// go-sql-driver keeps the low byte of a column's 2-byte
				// collation (fields.go charSet uint8), so 0x013f reads as
				// binary (63): BLOB and Blob values where the protocol's
				// collation says TEXT and Text. Xo reads both bytes, as the
				// protocol defines them
				refused["go collation byte"]++
				continue
			}
			if got[i] != want[i] && myfUnsignedDecimal(got[i], want[i], cases[i].packets) {
				// go-sql-driver's DECIMAL precision is length - 1 (- 2 with
				// decimals) whatever the UNSIGNED flag; an unsigned
				// DECIMAL's length has no sign byte, so Xo's precision
				// (MySQL's, as my_decimal_length_to_precision) is one more
				refused["go unsigned decimal"]++
				continue
			}
			if got[i] != want[i] && cases[i].note != "" && strings.HasPrefix(got[i], "error ") && strings.Count(got[i], "\n") == 1 {
				refused[cases[i].note]++
				continue
			}
			if got[i] != want[i] {
				mu.Lock()
				if bad < 10 {
					var pk []string
					for _, p := range cases[i].packets {
						pk = append(pk, fmt.Sprintf("%x", p))
					}
					t.Errorf("%s case %d (seed %d, %s): got\n%swant\n%spackets %s", b, i, seed, cases[i].note, got[i], want[i], strings.Join(pk, " "))
				}
				bad++
				mu.Unlock()
			}
		}
		if bad > 0 {
			t.Fatalf("%s: %d of %d cases differ (seed %d)", b, bad, n, seed)
		}
		t.Logf("%s: %d cases equal go-sql-driver's or refused by Xo (seed %d); refused changed cases by change: %v", b, n, seed, refused)
	}
}

func myfTail(s string, n int) string {
	ls := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	return strings.Join(ls, "\n")
}

// TestMysqlFuzzWideCollation: seed 2's case 3470 at 5000 cases, a column
// with collation 0x233f that go-sql-driver reads as 63 (binary), and seed
// 4's case 707 (0x383f), whose values differ in kind as well.
func TestMysqlFuzzWideCollation(t *testing.T) {
	def, _ := hex.DecodeString("036465660264620174017405c3b120c3a905c3b120c3a90c3f23ff000000fc8100000000")
	if !myfWideCollation([][]byte{{1}, def}) {
		t.Errorf("collation 0x233f: not wide")
	}
	def[25] = 0
	if myfWideCollation([][]byte{{1}, def}) {
		t.Errorf("collation 63: wide")
	}
	// seed 12's case 2175: an UNSIGNED DECIMAL of length 255, precision 254
	// in Xo (MySQL's) and 253 in go-sql-driver
	ud, _ := hex.DecodeString("03646566026462017401740263300263300cff00ff000000002000000000")
	sd, _ := hex.DecodeString("03646566026462017401740263300263300cff00ff000000000000000000")
	g, w := "types c0:DECIMAL:254,0:true\nRow{x}\n", "types c0:DECIMAL:253,0:true\nRow{x}\n"
	if !myfUnsignedDecimal(g, w, [][]byte{{1}, ud}) {
		t.Errorf("unsigned DECIMAL 254 against 253: not the divergence")
	}
	if myfUnsignedDecimal(g, w, [][]byte{{1}, sd}) {
		t.Errorf("signed DECIMAL 254 against 253: the divergence")
	}
	if myfUnsignedDecimal(g, "types c0:DECIMAL:253,0:true\nRow{y}\n", [][]byte{{1}, ud}) {
		t.Errorf("different rows: the divergence")
	}
	// seed 4's case 707: the values differ in kind too
	if !myfSameButBinary("types a:TEXT:-:true\nRow{vals: [Text(s: \"x\")]}\n", "types a:BLOB:-:true\nRow{vals: [Blob(b: b\"x\")]}\n") {
		t.Errorf("TEXT/Text against BLOB/Blob: not the same")
	}
	if myfSameButBinary("types a:TEXT:-:true\nRow{vals: [Text(s: \"x\")]}\n", "types a:BLOB:-:true\nRow{vals: [Blob(b: b\"y\")]}\n") {
		t.Errorf("different values: the same")
	}
}
