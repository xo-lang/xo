package stdtest

// A differential of std/sse against the WHATWG event stream interpretation
// (task #115): the HTML Living Standard's algorithm (9.2.6) transcribed
// here, sseRef, is the oracle; Go's standard library has no event stream
// parser. Random streams (a BOM, CRLF, LF and lone CR line ends, comments,
// fields with and without a colon and a space, repeated and empty data
// lines, ids with NUL, retry values that are and are not digits, bytes that
// are not UTF-8, unknown fields, an unfinished last event or line) are fed
// to a Parser in random chunk sizes, so a CRLF, a BOM or a UTF-8 sequence
// is split across chunks; the events, the retry and the last event ID must
// equal the oracle's whatever the chunking. A line longer than the
// parser's limit (std/sse's TooLong, from Go's bufio.Scanner limit, not
// the standard's) must be an error, after a prefix of the oracle's
// events. And encode's text, read by the oracle, must give back the event
// it was given (its data's CR and CRLF read as LF).
//
// Where Xo's choices are kept: a retry past Int's range is ignored (Go's
// strconv.ParseInt failing, as Go readers do); the standard has no limit.
//
//   XO_SSEFUZZ_SEED (2), _N (20000), _BACKENDS (arm64,llvm)

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/rand"
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

type sseEv struct{ event, data, id string }

// sseDecode is the WHATWG UTF-8 decode (one U+FFFD per maximal subpart):
// Go's strings.ToValidUTF8 replaces a run with one, so it goes byte by
// byte with utf8 rules instead.
func sseDecode(b []byte) string {
	var o strings.Builder
	for i := 0; i < len(b); {
		c := b[i]
		if c < 0x80 {
			o.WriteByte(c)
			i++
			continue
		}
		need, lo, hi := 0, byte(0x80), byte(0xBF)
		switch {
		case c >= 0xC2 && c <= 0xDF:
			need = 1
		case c >= 0xE0 && c <= 0xEF:
			need = 2
			if c == 0xE0 {
				lo = 0xA0
			}
			if c == 0xED {
				hi = 0x9F
			}
		case c >= 0xF0 && c <= 0xF4:
			need = 3
			if c == 0xF0 {
				lo = 0x90
			}
			if c == 0xF4 {
				hi = 0x8F
			}
		}
		if need == 0 {
			o.WriteString("�")
			i++
			continue
		}
		j := i + 1
		ok := true
		for j-i <= need {
			if j >= len(b) || b[j] < lo || b[j] > hi {
				ok = false
				break
			}
			lo, hi = 0x80, 0xBF
			j++
		}
		if ok {
			o.Write(b[i:j])
		} else {
			o.WriteString("�")
		}
		i = j
	}
	return o.String()
}

// sseRef is the standard's algorithm over the whole stream: decode (one
// leading BOM stripped), split into lines at CRLF, LF and CR, process each
// complete line; what follows the last line end is discarded, as is an
// event without its blank line. It returns the events, the retry (-1 for
// none) and the last event ID, and the length of the longest line (an
// unterminated tail counts, as std/sse's buffer does).
func sseRef(stream []byte) ([]sseEv, int64, string, int) {
	b := stream
	if bytes.HasPrefix(b, []byte("\xEF\xBB\xBF")) {
		b = b[3:]
	}
	var evs []sseEv
	retry := int64(-1)
	lastID, evType := "", ""
	var data strings.Builder
	haveData := false
	longest := 0
	line := func(l []byte) {
		if len(l) > longest {
			longest = len(l)
		}
		if len(l) == 0 {
			if !haveData {
				evType = ""
				return
			}
			d := data.String()
			d = strings.TrimSuffix(d, "\n")
			t := evType
			if t == "" {
				t = "message"
			}
			evs = append(evs, sseEv{t, d, lastID})
			data.Reset()
			haveData = false
			evType = ""
			return
		}
		if l[0] == ':' {
			return
		}
		name, value := l, []byte(nil)
		if k := bytes.IndexByte(l, ':'); k >= 0 {
			name, value = l[:k], l[k+1:]
			if len(value) > 0 && value[0] == ' ' {
				value = value[1:]
			}
		}
		switch sseDecode(name) {
		case "event":
			evType = sseDecode(value)
		case "data":
			data.WriteString(sseDecode(value))
			data.WriteByte('\n')
			haveData = true
		case "id":
			if bytes.IndexByte(value, 0) < 0 {
				lastID = sseDecode(value)
			}
		case "retry":
			if len(value) > 0 && strings.Trim(string(value), "0123456789") == "" {
				if n, err := strconv.ParseInt(string(value), 10, 64); err == nil {
					retry = n
				}
			}
		}
	}
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' || b[i] == '\r' {
			line(b[start:i])
			if b[i] == '\r' && i+1 < len(b) && b[i+1] == '\n' {
				i++
			}
			start = i + 1
		}
	}
	if len(b)-start > longest {
		longest = len(b) - start
	}
	return evs, retry, lastID, longest
}

func ssePick(r *rand.Rand, xs ...string) string { return xs[r.Intn(len(xs))] }

func sseEnd(r *rand.Rand) string { return ssePick(r, "\n", "\n", "\r\n", "\r") }

func sseValue(r *rand.Rand) string {
	if r.Intn(6) == 0 {
		// the hostile dictionary (task #352): digit runs past Int, Int's
		// edges, and UTF-8 cut inside a rune
		switch r.Intn(3) {
		case 0:
			return hostile.Pick(r, hostile.Digits())
		case 1:
			return strconv.FormatInt(hostile.Pick(r, hostile.Ints()), 10)
		default:
			return hostile.Pick(r, append(hostile.UTF8(), hostile.CutUTF8("é日😀")...))
		}
	}
	return ssePick(r, "", "x", " lead", "  two", "a:b", "日本", "\xff", "\xe2\x80", "\xf0\x9f\x98", "\xef\xbb\xbf", "a\x00b", "123",
		"007", "99999999999999999999999", "-5", "1 2", "😀", "\xc3", "\xed\xa0\x80", "\xf4\x90\x80\x80")
}

func sseField(r *rand.Rand) string {
	switch r.Intn(12) {
	case 0:
		return ":" + sseValue(r)
	case 1:
		return ssePick(r, "data", "event", "id", "retry", "dat", "Data", "data ", "")
	case 2:
		return ssePick(r, "data", "event", "id", "retry", "foo", "da\xffta") + ":" + sseValue(r)
	case 3, 4, 5:
		return "data" + ssePick(r, ":", ": ", ":  ") + sseValue(r)
	case 6:
		return "event" + ssePick(r, ":", ": ") + sseValue(r)
	case 7:
		return "id" + ssePick(r, ":", ": ") + sseValue(r)
	case 8:
		return "retry" + ssePick(r, ":", ": ") + ssePick(r, "1000", "0", "x", "", "18446744073709551616", "9223372036854775807", hostile.Pick(r, hostile.Digits()), strconv.FormatUint(hostile.Pick(r, hostile.Uints()), 10), sseValue(r))
	default:
		return "data: " + strings.Repeat(ssePick(r, "ab", "é", "\xe2"), r.Intn(20))
	}
}

func sseStream(r *rand.Rand) []byte {
	var b strings.Builder
	if r.Intn(5) == 0 {
		b.WriteString("\xEF\xBB\xBF")
	}
	if r.Intn(20) == 0 {
		b.WriteString("\xEF\xBB\xBF")
	}
	for i := r.Intn(14); i > 0; i-- {
		for j := r.Intn(4); j >= 0; j-- {
			b.WriteString(sseField(r))
			b.WriteString(sseEnd(r))
		}
		if r.Intn(6) != 0 {
			b.WriteString(sseEnd(r))
		}
	}
	if r.Intn(3) == 0 {
		b.WriteString(sseField(r))
	}
	return []byte(b.String())
}

func sseHex(s string) string { return hex.EncodeToString([]byte(s)) }

func sseWant(stream []byte, max int) string {
	evs, retry, last, longest := sseRef(stream)
	var o strings.Builder
	if longest > max {
		return "toolong"
	}
	for _, e := range evs {
		fmt.Fprintf(&o, "ev %s %s %s\n", sseHex(e.event), sseHex(e.data), sseHex(e.id))
	}
	r := "none"
	if retry >= 0 {
		r = strconv.FormatInt(retry, 10)
	}
	fmt.Fprintf(&o, "end %s %s\n", r, sseHex(last))
	return o.String()
}

type sseCase struct {
	line, want string
	stream     []byte
	max        int
	enc        *sseEv
	encRetry   int64
}

func sseGen(r *rand.Rand) sseCase {
	if r.Intn(6) == 0 {
		// encode: an event without line breaks in its name or id
		ev := sseEv{ssePick(r, "", "message", "update", "é x", " sp"), "", ssePick(r, "", "1", "abc", " x", "-")}
		var d strings.Builder
		for i := r.Intn(5); i > 0; i-- {
			d.WriteString(ssePick(r, "a", "", " b", "日本", ":c", "data: x"))
			if r.Intn(3) > 0 {
				d.WriteString(ssePick(r, "\n", "\r\n", "\r"))
			}
		}
		ev.data = d.String()
		idArg := "x" + sseHex(ev.id)
		if ev.id == "-" {
			idArg = "-"
		}
		rt := int64(-1)
		retryArg := "-"
		if r.Intn(2) == 0 {
			rt = r.Int63n(100000)
			retryArg = strconv.FormatInt(rt, 10)
		}
		return sseCase{line: fmt.Sprintf("e x%s x%s %s %s", sseHex(ev.data), sseHex(ev.event), idArg, retryArg), enc: &ev, encRetry: rt}
	}
	s := sseStream(r)
	var sizes []string
	for i := 1 + r.Intn(4); i > 0; i-- {
		sizes = append(sizes, strconv.Itoa(1+r.Intn(1+r.Intn(40))))
	}
	max := 1048576
	if r.Intn(5) == 0 {
		max = 1 + r.Intn(30)
	}
	return sseCase{line: fmt.Sprintf("p x%s %s %d", hex.EncodeToString(s), strings.Join(sizes, ","), max), want: sseWant(s, max), stream: s, max: max}
}

// sseCheckEncode: the oracle's reading of encode's text is the event.
func sseCheckEncode(c sseCase, got string) string {
	b, err := hex.DecodeString(strings.TrimSpace(got))
	if err != nil {
		return "not hex: " + got
	}
	evs, retry, last, _ := sseRef(b)
	want := c.enc
	event := want.event
	if event == "" {
		event = "message"
	}
	data := strings.ReplaceAll(strings.ReplaceAll(want.data, "\r\n", "\n"), "\r", "\n")
	id := want.id
	if id == "-" {
		id = ""
	}
	if len(evs) != 1 || evs[0] != (sseEv{event, data, id}) || last != id || retry != c.encRetry {
		return fmt.Sprintf("encode gave %q: read as %q retry %d last %q, want %q retry %d", b, evs, retry, last, sseEv{event, data, id}, c.encRetry)
	}
	return ""
}

func sseBuild(t *testing.T, work, backend string) string {
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(work, "xo-sse-"+backend)
	args := []string{"build"}
	if backend != "" {
		args = append(args, "--backend="+backend)
	}
	args = append(args, "-o", bin, filepath.Join(root, "scripts", "ssediff", "xo"))
	xb := testtmp.Guard(exec.Command(xo, args...))
	xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
	if out, err := xb.CombinedOutput(); err != nil {
		t.Fatalf("xo build --backend=%s: %v\n%s", backend, err, out)
	}
	return bin
}

func sseRunXo(t *testing.T, bin string, lines []string) []string {
	cmd := testtmp.Guard(exec.Command(bin))
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s: %v\n%s\nstderr:\n%s", bin, err, myfTail(out.String(), 20), myfTail(errb.String(), 30))
	}
	var bs []string
	for _, l := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(l, "== ") {
			bs = append(bs, "")
			continue
		}
		if len(bs) > 0 && l != "" {
			bs[len(bs)-1] += l + "\n"
		}
	}
	return bs
}

func TestSseFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the sse fuzz builds the Xo client")
	}
	seed := int64(envInt(t, "XO_SSEFUZZ_SEED", 2))
	n := envInt(t, "XO_SSEFUZZ_N", 20000)
	backends := envBackends("XO_SSEFUZZ_BACKENDS", []string{"arm64", "llvm"})
	r := rand.New(rand.NewSource(seed))
	cases := make([]sseCase, n)
	lines := make([]string, n)
	long := 0
	for i := range cases {
		cases[i] = sseGen(r)
		lines[i] = cases[i].line
		if cases[i].want == "toolong" {
			long++
		}
	}
	t.Logf("seed %d: %d cases, %d with a line past the limit", seed, n, long)
	work, err := testtmp.Dir("xo-ssefuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	for _, b := range backends {
		got := sseRunXo(t, sseBuild(t, work, b), lines)
		if len(got) != n {
			t.Fatalf("%s: %d results for %d cases", b, len(got), n)
		}
		bad := 0
		for i, c := range cases {
			msg := ""
			switch {
			case c.enc != nil:
				msg = sseCheckEncode(c, got[i])
			case c.want == "toolong":
				// an error, after a prefix of the oracle's events
				evs, _, _, _ := sseRef(c.stream)
				var pre strings.Builder
				for _, e := range evs {
					fmt.Fprintf(&pre, "ev %s %s %s\n", sseHex(e.event), sseHex(e.data), sseHex(e.id))
				}
				body := strings.TrimSuffix(got[i], "err toolong\n")
				if !strings.HasSuffix(got[i], "err toolong\n") || !strings.HasPrefix(pre.String(), body) {
					msg = fmt.Sprintf("got\n%swant a prefix of\n%sthen err toolong", got[i], pre.String())
				}
			case got[i] != c.want:
				msg = fmt.Sprintf("got\n%swant\n%s", got[i], c.want)
			}
			if msg == "" {
				continue
			}
			if bad < 10 {
				t.Errorf("%s case %d (seed %d): %s\n%s", b, i, seed, c.line, msg)
			}
			bad++
		}
		if bad > 0 {
			t.Fatalf("%s: %d of %d cases differ (seed %d)", b, bad, n, seed)
		}
		t.Logf("%s: %d cases equal the WHATWG algorithm (seed %d)", b, n, seed)
	}
}
