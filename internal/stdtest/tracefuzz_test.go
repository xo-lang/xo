package stdtest

// TestTraceFuzz (task #115): std/trace against a Go model of its
// documented behaviour, on seeded scripts. std/trace has no Go
// counterpart, so the model is its doc comments and OpenTelemetry Go's
// rules: a Ring keeps the last cap spans (cap at least 1) and Ring.spans
// gives them newest first, at most n (all for n <= 0) at least min long;
// a Recorder's span takes its trace and span ids from the random source
// (16 and 8 bytes from one draw for a new trace, 8 for a child; an id
// that is all zero is drawn again and is never all zero), its times from
// the clock (end never before start); set_attributes replaces a key in
// place (last value wins); set_status follows OpenTelemetry Go (Unset
// changes nothing, Ok is final and drops the message, Error keeps its
// message until an Ok); end records the span once. Every span's JSON must
// decode in Go to the same span (the optional fields present exactly when
// set) and back in Xo to an equal one. The scripts use a scripted clock
// and random source (scripts/tracediff/xo), with draws all zero, zero at
// the ends, short and long, clocks that go back, negative and zero
// durations, caps from -1 to 7, ends twice and changes after an end.
//
//   XO_TRACEFUZZ_SEED (default 2), XO_TRACEFUZZ_N (3000 scripts),
//   XO_TRACEFUZZ_BACKENDS (arm64,llvm)

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

type trfAttr struct {
	Key  string
	Kind byte // s i b f
	S    string
	I    int64
	B    bool
	F    float64
}

type trfEvent struct {
	Name string
	NS   int64
}

type trfSpan struct {
	ID, Name                   string
	StartMS, Micros            int64
	Status                     int64
	TraceID, SpanID, ParentID  string
	Kind                       int // -1 none
	Attrs                      []trfAttr
	Events                     []trfEvent
	Code                       int // -1 none
	Msg                        string
	StartNS, EndNS             int64
	Scope, ScopeVer, SchemaURL string
}

func (s trfSpan) show() string {
	var attrs, evs string
	for _, a := range s.Attrs {
		var v string
		switch a.Kind {
		case 's':
			v = "s" + strconv.Quote(a.S)
		case 'i':
			v = "i" + strconv.FormatInt(a.I, 10)
		case 'b':
			v = "b" + strconv.FormatBool(a.B)
		default:
			v = "f" + strconv.FormatFloat(a.F, 'f', 6, 64)
		}
		attrs += " " + strconv.Quote(a.Key) + "=" + v
	}
	for _, e := range s.Events {
		evs += fmt.Sprintf(" %s@%d", strconv.Quote(e.Name), e.NS)
	}
	kind, code := "-", "-"
	if s.Kind >= 0 {
		kind = strconv.Itoa(s.Kind)
	}
	if s.Code >= 0 {
		code = strconv.Itoa(s.Code)
	}
	return fmt.Sprintf("span %s %s ms %d us %d status %d trace %s sid %s parent %s kind %s code %s %s ns %d..%d attrs%s events%s",
		strconv.Quote(s.ID), strconv.Quote(s.Name), s.StartMS, s.Micros, s.Status, s.TraceID, s.SpanID, s.ParentID, kind, code, strconv.Quote(s.Msg), s.StartNS, s.EndNS, attrs, evs)
}

type trfActive struct {
	span trfSpan
	done bool
}

// trfModel: std/trace as documented, with the scripted clock and draws.
type trfModel struct {
	cap    int
	buf    []trfSpan
	clock  int64
	draws  [][]byte
	active map[string]*trfActive
	out    strings.Builder
}

func (m *trfModel) push(s trfSpan) {
	if len(m.buf) >= 2*m.cap {
		m.buf = append([]trfSpan(nil), m.buf[len(m.buf)-m.cap+1:]...)
	}
	m.buf = append(m.buf, s)
}

func (m *trfModel) spans(minNS int64, n int) []trfSpan {
	var out []trfSpan
	first := 0
	if len(m.buf) > m.cap {
		first = len(m.buf) - m.cap
	}
	for i := len(m.buf) - 1; i >= first; i-- {
		if m.buf[i].Micros*1000 >= minNS {
			out = append(out, m.buf[i])
			if n > 0 && len(out) >= n {
				break
			}
		}
	}
	return out
}

func (m *trfModel) draw(n int) []byte {
	var b []byte
	if len(m.draws) > 0 {
		b = m.draws[0]
		m.draws = m.draws[1:]
	}
	out := make([]byte, n)
	copy(out, b)
	return out
}

// hexID: b as an id; all zero is drawn again, and a second draw with a
// zero first and last byte gives 0...01 (never all zero).
func (m *trfModel) hexID(b []byte) string {
	zero := true
	for _, c := range b {
		if c != 0 {
			zero = false
		}
	}
	if zero {
		n := len(b)
		b = m.draw(n)
		if b[0] == 0 && b[n-1] == 0 {
			return strings.Repeat("0", 2*n-1) + "1"
		}
	}
	return hex.EncodeToString(b)
}

func trfText(h string) string {
	b, err := hex.DecodeString(strings.TrimPrefix(h, "x"))
	if err != nil {
		panic(h)
	}
	return string(b)
}

func trfAtoi(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func (m *trfModel) step(line string) {
	w := strings.Split(line, " ")
	fmt.Fprintf(&m.out, "== %s\n", line)
	switch w[0] {
	case "ring":
		m.cap = int(trfAtoi(w[1]))
		if m.cap < 1 {
			m.cap = 1
		}
		m.buf = nil
		m.active = map[string]*trfActive{}
		m.draws = nil
	case "rec":
		us := trfAtoi(w[4])
		m.push(trfSpan{ID: trfText(w[1]), Name: trfText(w[2]), StartMS: trfAtoi(w[3]), Micros: us, Kind: -1, Code: -1})
	case "spans":
		for _, s := range m.spans(trfAtoi(w[1])*1000, int(trfAtoi(w[2]))) {
			m.out.WriteString(s.show() + "\n")
		}
	case "clock":
		m.clock = trfAtoi(w[1])
	case "rand":
		b, _ := hex.DecodeString(strings.TrimPrefix(w[1], "x"))
		m.draws = append(m.draws, b)
	case "start":
		ns := m.clock
		var tid, sid, pid string
		if w[3] == "-" {
			b := m.draw(24)
			tid = m.hexID(b[:16])
			sid = m.hexID(b[16:])
			pid = "root"
		} else {
			tid, pid = trfText(w[4]), trfText(w[5])
			sid = m.hexID(m.draw(8))
		}
		k := int(trfAtoi(w[6]))
		if k > 5 {
			k = 5
		}
		m.active[w[1]] = &trfActive{span: trfSpan{ID: tid, Name: trfText(w[2]), StartMS: ns / 1000000, TraceID: tid, SpanID: sid, ParentID: pid, Kind: k, Code: -1, StartNS: ns, EndNS: ns}}
		fmt.Fprintf(&m.out, "ids %s %s\n", tid, sid)
	case "attr":
		a := trfAttr{Key: trfText(w[2]), Kind: w[3][0]}
		switch w[3] {
		case "s":
			a.S = trfText(w[4])
		case "i":
			a.I = trfAtoi(w[4])
		case "b":
			a.B = w[4] == "1"
		default:
			a.Kind = 'f'
			a.F, _ = strconv.ParseFloat(w[4], 64)
		}
		s := &m.active[w[1]].span
		at := -1
		for i, o := range s.Attrs {
			if o.Key == a.Key {
				at = i
			}
		}
		if at >= 0 {
			attrs := append([]trfAttr(nil), s.Attrs...)
			attrs[at] = a
			s.Attrs = attrs
		} else {
			s.Attrs = append(append([]trfAttr(nil), s.Attrs...), a)
		}
	case "event":
		s := &m.active[w[1]].span
		s.Events = append(append([]trfEvent(nil), s.Events...), trfEvent{Name: trfText(w[2]), NS: m.clock})
	case "status":
		s := &m.active[w[1]].span
		if s.Code == 1 {
			break
		}
		switch w[2] {
		case "1":
			s.Code, s.Msg = 1, ""
		case "2":
			s.Code, s.Msg = 2, trfText(w[3])
		}
	case "end":
		a := m.active[w[1]]
		if a.done {
			break
		}
		a.done = true
		end := m.clock
		if end < a.span.StartNS {
			end = a.span.StartNS
		}
		a.span.EndNS = end
		a.span.Micros = (end - a.span.StartNS) / 1000
		s := a.span
		s.Attrs = append([]trfAttr(nil), s.Attrs...)
		s.Events = append([]trfEvent(nil), s.Events...)
		m.push(s)
	case "json":
		// checked against the JSON the Xo client prints (trfCheckJSON)
	}
}

// trfJSONSpan: std/trace's Span as its derived JSON writes it.
type trfJSONSpan struct {
	ID         *string `json:"id"`
	Name       *string `json:"name"`
	StartMS    *int64  `json:"start_ms"`
	Micros     *int64  `json:"micros"`
	Status     *int64  `json:"status"`
	TraceID    *string `json:"trace_id"`
	SpanID     *string `json:"span_id"`
	ParentID   *string `json:"parent_id"`
	Kind       *string `json:"kind"`
	Attributes []struct {
		Key   string                     `json:"key"`
		Value map[string]json.RawMessage `json:"value"`
	} `json:"attributes"`
	Events []struct {
		Name       string            `json:"name"`
		TimeNS     int64             `json:"time_ns"`
		Attributes []json.RawMessage `json:"attributes"`
	} `json:"events"`
	StatusCode    *string `json:"status_code"`
	StatusMessage *string `json:"status_message"`
	StartNS       *int64  `json:"start_ns"`
	EndNS         *int64  `json:"end_ns"`
	Scope         *string `json:"scope"`
	ScopeVersion  *string `json:"scope_version"`
	SchemaURL     *string `json:"schema_url"`
}

var trfKinds = []string{"Unspecified", "Internal", "Server", "Client", "Producer", "Consumer"}
var trfCodes = []string{"Unset", "Ok", "Error"}

// trfFromJSON: the span the JSON text holds, or why it is not one as
// documented (a required field missing, an empty optional field present).
func trfFromJSON(text string) (trfSpan, error) {
	var j trfJSONSpan
	dec := json.NewDecoder(strings.NewReader(text))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&j); err != nil {
		return trfSpan{}, err
	}
	if j.ID == nil || j.Name == nil || j.StartMS == nil || j.Micros == nil || j.Status == nil {
		return trfSpan{}, fmt.Errorf("a required field is missing")
	}
	s := trfSpan{ID: *j.ID, Name: *j.Name, StartMS: *j.StartMS, Micros: *j.Micros, Status: *j.Status, Kind: -1, Code: -1}
	str := func(name string, p *string, dst *string) error {
		if p != nil {
			if *p == "" {
				return fmt.Errorf("%s is present but empty", name)
			}
			*dst = *p
		}
		return nil
	}
	num := func(name string, p *int64, dst *int64) error {
		if p != nil {
			if *p == 0 {
				return fmt.Errorf("%s is present but 0", name)
			}
			*dst = *p
		}
		return nil
	}
	for _, e := range []error{
		str("trace_id", j.TraceID, &s.TraceID), str("span_id", j.SpanID, &s.SpanID), str("parent_id", j.ParentID, &s.ParentID),
		str("status_message", j.StatusMessage, &s.Msg), str("scope", j.Scope, &s.Scope), str("scope_version", j.ScopeVersion, &s.ScopeVer),
		str("schema_url", j.SchemaURL, &s.SchemaURL), num("start_ns", j.StartNS, &s.StartNS), num("end_ns", j.EndNS, &s.EndNS),
	} {
		if e != nil {
			return s, e
		}
	}
	idx := func(xs []string, v string) int {
		for i, x := range xs {
			if x == v {
				return i
			}
		}
		return -2
	}
	if j.Kind != nil {
		if s.Kind = idx(trfKinds, *j.Kind); s.Kind < 0 {
			return s, fmt.Errorf("kind %q", *j.Kind)
		}
	}
	if j.StatusCode != nil {
		if s.Code = idx(trfCodes, *j.StatusCode); s.Code < 0 {
			return s, fmt.Errorf("status_code %q", *j.StatusCode)
		}
	}
	if j.Attributes != nil && len(j.Attributes) == 0 || j.Events != nil && len(j.Events) == 0 {
		return s, fmt.Errorf("an empty list is present")
	}
	for _, a := range j.Attributes {
		if len(a.Value) != 1 {
			return s, fmt.Errorf("attribute %q: value %v", a.Key, a.Value)
		}
		x := trfAttr{Key: a.Key}
		for k, raw := range a.Value {
			var v struct {
				V json.RawMessage `json:"v"`
			}
			if err := json.Unmarshal(raw, &v); err != nil {
				return s, err
			}
			var err error
			switch k {
			case "Str":
				x.Kind = 's'
				err = json.Unmarshal(v.V, &x.S)
			case "Int":
				x.Kind = 'i'
				err = json.Unmarshal(v.V, &x.I)
			case "Bool":
				x.Kind = 'b'
				err = json.Unmarshal(v.V, &x.B)
			case "Float":
				x.Kind = 'f'
				err = json.Unmarshal(v.V, &x.F)
			default:
				err = fmt.Errorf("attribute kind %q", k)
			}
			if err != nil {
				return s, err
			}
		}
		s.Attrs = append(s.Attrs, x)
	}
	for _, e := range j.Events {
		if e.Attributes != nil {
			return s, fmt.Errorf("event attributes present but empty")
		}
		s.Events = append(s.Events, trfEvent{Name: e.Name, NS: e.TimeNS})
	}
	return s, nil
}

// trfExpectJSON: what the model wants of a "json" step: for each span of
// spans(0, 0), the span decoded from Xo's JSON and Xo's decode back equal.
func (m *trfModel) expectJSON() []string {
	var out []string
	for _, s := range m.spans(0, 0) {
		out = append(out, s.show())
	}
	return out
}

var trfNames = []string{"", "request", "db", "x y", "caf\u00e9", "\"q\"", "a\\b", "\u2603", "<&>", "\t", "long-" + strings.Repeat("n", 40)}

func trfHex(s string) string { return fmt.Sprintf("x%x", s) }

func trfDraw(r *rand.Rand) string {
	n := []int{0, 1, 4, 8, 16, 24, 30}[r.Intn(7)]
	b := make([]byte, n)
	switch r.Intn(5) {
	case 0: // all zero
	case 1: // zero at the ends
		r.Read(b)
		if n > 0 {
			b[0], b[n-1] = 0, 0
		}
	default:
		r.Read(b)
	}
	return fmt.Sprintf("x%x", b)
}

// trfGen: one script.
func trfGen(r *rand.Rand) []string {
	cp := r.Intn(9) - 1
	lines := []string{fmt.Sprintf("ring %d", cp)}
	clock := int64(r.Intn(3)) * 1_700_000_000_000_000_000 / 2
	lines = append(lines, fmt.Sprintf("clock %d", clock))
	var labels []string
	for i, n := 0, 5+r.Intn(60); i < n; i++ {
		switch k := r.Intn(20); {
		case k < 3:
			us := int64(r.Intn(3000))
			if r.Intn(8) == 0 {
				us = -int64(r.Intn(20))
			}
			lines = append(lines, fmt.Sprintf("rec %s %s %d %d", trfHex(trfPick(r)), trfHex(trfPick(r)), r.Int63n(1<<41), us))
		case k < 5:
			lines = append(lines, fmt.Sprintf("spans %d %d", r.Intn(3000)-5, r.Intn(10)-1))
		case k < 7:
			d := r.Int63n(5_000_000)
			if r.Intn(6) == 0 {
				d = -r.Int63n(1_000_000)
			}
			clock += d
			lines = append(lines, fmt.Sprintf("clock %d", clock))
		case k < 9:
			lines = append(lines, "rand "+trfDraw(r))
		case k < 12 || len(labels) == 0:
			l := string(rune('A' + len(labels)%26))
			labels = append(labels, l)
			if r.Intn(2) == 0 {
				lines = append(lines, fmt.Sprintf("start %s %s - x x %d", l, trfHex(trfPick(r)), r.Intn(6)))
			} else {
				lines = append(lines, fmt.Sprintf("start %s %s p %s %s %d", l, trfHex(trfPick(r)), trfHex(fmt.Sprintf("%032x", r.Uint64())), trfHex(fmt.Sprintf("%016x", r.Uint64())), r.Intn(6)))
			}
		case k < 15:
			key := []string{"k", "http.method", "a", "\u00e9", ""}[r.Intn(5)]
			var v string
			switch r.Intn(4) {
			case 0:
				v = "s " + trfHex(trfPick(r))
			case 1:
				v = fmt.Sprintf("i %d", r.Int63()-r.Int63())
			case 2:
				v = fmt.Sprintf("b %d", r.Intn(2))
			default:
				v = fmt.Sprintf("f %d.%03d", r.Intn(2000)-1000, r.Intn(1000))
			}
			lines = append(lines, fmt.Sprintf("attr %s %s %s", labels[r.Intn(len(labels))], trfHex(key), v))
		case k < 16:
			lines = append(lines, fmt.Sprintf("event %s %s", labels[r.Intn(len(labels))], trfHex(trfPick(r))))
		case k < 18:
			lines = append(lines, fmt.Sprintf("status %s %d %s", labels[r.Intn(len(labels))], r.Intn(3), trfHex(trfPick(r))))
		default:
			lines = append(lines, "end "+labels[r.Intn(len(labels))])
		}
	}
	lines = append(lines, "spans 0 0", "json")
	return lines
}

func trfPick(r *rand.Rand) string { return trfNames[r.Intn(len(trfNames))] }

// trfCases: Xo's output split by script (each starts with "== ring").
func trfCases(out string) []string {
	var cs []string
	for _, l := range strings.SplitAfter(out, "\n") {
		if strings.HasPrefix(l, "== ring ") {
			cs = append(cs, "")
		}
		if len(cs) > 0 {
			cs[len(cs)-1] += l
		}
	}
	return cs
}

// trfCheck: the model's output for a script against Xo's: every line but
// the "json"/"back" ones must be equal; each json line must decode to the
// model's span and each back line must say equal.
func trfCheck(script []string, got string) string {
	m := &trfModel{active: map[string]*trfActive{}, cap: 1000}
	for _, l := range script {
		m.step(l)
	}
	want := m.out.String()
	gl := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	var plain []string
	var jsons []string
	for _, l := range gl {
		switch {
		case strings.HasPrefix(l, "json "):
			jsons = append(jsons, strings.TrimPrefix(l, "json "))
		case strings.HasPrefix(l, "back "):
			if l != "back equal" {
				return "Xo's JSON did not decode back equal: " + l
			}
		default:
			plain = append(plain, l)
		}
	}
	if g := strings.Join(plain, "\n") + "\n"; g != want {
		return trfDiff(g, want)
	}
	exp := m.expectJSON()
	if len(jsons) != len(exp) {
		return fmt.Sprintf("%d json lines, want %d", len(jsons), len(exp))
	}
	for i, j := range jsons {
		s, err := trfFromJSON(j)
		if err != nil {
			return fmt.Sprintf("json %d: %v: %s", i, err, j)
		}
		if s.show() != exp[i] {
			return fmt.Sprintf("json %d decodes to\n%s\nwant\n%s\n(%s)", i, s.show(), exp[i], j)
		}
	}
	return ""
}

func trfDiff(got, want string) string {
	g := strings.Split(got, "\n")
	w := strings.Split(want, "\n")
	for i := 0; i < len(g) || i < len(w); i++ {
		var a, b string
		if i < len(g) {
			a = g[i]
		}
		if i < len(w) {
			b = w[i]
		}
		if a != b {
			lo := i - 6
			if lo < 0 {
				lo = 0
			}
			ctx := strings.Join(w[lo:i], "\n")
			return fmt.Sprintf("line %d:\n%s\ngot  %s\nwant %s", i, ctx, a, b)
		}
	}
	return ""
}

func TestTraceFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the trace fuzz builds the Xo client")
	}
	seed := int64(envInt(t, "XO_TRACEFUZZ_SEED", 2))
	n := envInt(t, "XO_TRACEFUZZ_N", 3000)
	backends := envBackends("XO_TRACEFUZZ_BACKENDS", []string{"arm64", "llvm"})
	r := rand.New(rand.NewSource(seed))
	scripts := make([][]string, n)
	var all []string
	for i := range scripts {
		scripts[i] = trfGen(r)
		all = append(all, scripts[i]...)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-tracefuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	for _, b := range backends {
		bin := filepath.Join(work, "xo-trace-"+b)
		xb := testtmp.Guard(exec.Command(xo, "build", "--backend="+b, "-o", bin, filepath.Join(root, "scripts", "tracediff", "xo")))
		xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
		if out, err := xb.CombinedOutput(); err != nil {
			t.Fatalf("xo build --backend=%s: %v\n%s", b, err, out)
		}
		cmd := testtmp.Guard(exec.Command(bin))
		cmd.Stdin = strings.NewReader(strings.Join(all, "\n") + "\n")
		var out, errb bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v\n%s\nstderr:\n%s", bin, err, myfTail(out.String(), 20), myfTail(errb.String(), 30))
		}
		got := trfCases(out.String())
		if len(got) != n {
			t.Fatalf("%s: %d scripts ran of %d", b, len(got), n)
		}
		bad := 0
		for i := range scripts {
			if msg := trfCheck(scripts[i], got[i]); msg != "" {
				if bad < 5 {
					t.Errorf("%s script %d (seed %d):\n%s\n%s", b, i, seed, strings.Join(scripts[i], "\n"), msg)
				}
				bad++
			}
		}
		if bad > 0 {
			t.Fatalf("%s: %d of %d scripts differ (seed %d)", b, bad, n, seed)
		}
		t.Logf("%s: %d scripts (%d steps) as documented (seed %d)", b, n, len(all), seed)
	}
}
