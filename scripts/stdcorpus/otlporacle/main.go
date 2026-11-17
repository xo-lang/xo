// The oracle of TestOtlpFuzz (internal/stdtest/otlpfuzz_test.go, task #115):
// seeded otlp.Resource values and std/trace spans (ids given, derived and
// malformed; kinds, HTTP and OpenTelemetry statuses, attributes of every
// value kind, NaN and the infinities among them, events, scopes), and the
// ExportTraceServiceRequest std/otlp's documented rules make of them, built
// as go.opentelemetry.io/proto/otlp v1.11.1 messages and written by
// protobuf-go (proto.Marshal, deterministic) and by otlptracehttp v1.47.0's
// own JSON encoder (otlpjson/, copied from its internal/otlpjson). Each case
// prints
//
//	== i
//	c RESOURCE<TAB>SPANS            (hex of JSON in std/trace's derive Json form)
//	w BINARY JSON                   (hex)
//
// A float attribute is passed as the Str "\x01f" plus its bits as a signed decimal Int
// in decimal (JSON has no NaN); the Xo side turns it back.
//
//	go run . SEED N
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"

	"stdcorpus/otlporacle/otlpjson"

	"google.golang.org/protobuf/proto"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

type attr struct {
	Key   string         `json:"key"`
	Value map[string]any `json:"value"`
	// the value the encoder sees
	v any
}

type event struct {
	Name   string `json:"name"`
	TimeNs int64  `json:"time_ns"`
	Attrs  []attr `json:"attributes"`
}

type span struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	StartMs       int64   `json:"start_ms"`
	Micros        int64   `json:"micros"`
	Status        int64   `json:"status"`
	TraceID       string  `json:"trace_id"`
	SpanID        string  `json:"span_id"`
	ParentID      string  `json:"parent_id"`
	Kind          *string `json:"kind"`
	Attrs         []attr  `json:"attributes"`
	Events        []event `json:"events"`
	StatusCode    *string `json:"status_code"`
	StatusMessage string  `json:"status_message"`
	StartNs       int64   `json:"start_ns"`
	EndNs         int64   `json:"end_ns"`
	Scope         string  `json:"scope"`
	ScopeVersion  string  `json:"scope_version"`
	SchemaURL     string  `json:"schema_url"`
}

type resource struct {
	Service    string      `json:"service"`
	Attributes [][2]string `json:"attributes"`
	Scope      string      `json:"scope"`
	Version    string      `json:"version"`
}

var strs = []string{"", "a", "GET /users/{id}", "ü", "<&>", "\"q\"\\", "line\nbreak\ttab", "  ", "\x01", "示例", "😀", "a\u007fb", "</script>"}

var kinds = []string{"Unspecified", "Internal", "Server", "Client", "Producer", "Consumer"}

var codes = []string{"Unset", "Ok", "Error"}

func pick(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

func str(r *rand.Rand) string {
	if r.Intn(4) == 0 {
		return pick(r, strs) + pick(r, strs)
	}
	return pick(r, strs)
}

func genAttr(r *rand.Rand) attr {
	key := pick(r, []string{"k", "k", "http.method", "", "ü", "a.b"})
	switch r.Intn(4) {
	case 0:
		s := str(r)
		return attr{Key: key, Value: map[string]any{"Str": map[string]any{"v": s}}, v: s}
	case 1:
		n := []int64{0, 1, -1, 42, math.MaxInt64, math.MinInt64, 1 << 53, -(1 << 31), r.Int63()}[r.Intn(9)]
		return attr{Key: key, Value: map[string]any{"Int": map[string]any{"v": n}}, v: n}
	case 2:
		f := []float64{0, math.Copysign(0, -1), 1.5, -2.25, 0.1, 1e300, 5e-324, 1e21, 1e-7, 123456789.125, math.NaN(), math.Inf(1), math.Inf(-1), r.NormFloat64() * 1e6}[r.Intn(14)]
		s := "\x01f" + strconv.FormatInt(int64(math.Float64bits(f)), 10)
		return attr{Key: key, Value: map[string]any{"Str": map[string]any{"v": s}}, v: f}
	default:
		b := r.Intn(2) == 0
		return attr{Key: key, Value: map[string]any{"Bool": map[string]any{"v": b}}, v: b}
	}
}

func genAttrs(r *rand.Rand, max int) []attr {
	out := []attr{}
	for i := r.Intn(max + 1); i > 0; i-- {
		out = append(out, genAttr(r))
	}
	return out
}

func genSpan(r *rand.Rand) span {
	s := span{
		ID:      pick(r, []string{"req-1", "req-2", "0af7651916cd43dd8448eb211c80319c", "0AF7651916CD43DD8448EB211C80319C", "zz7651916cd43dd8448eb211c80319c0", "", "ü", "x/y"}),
		Name:    pick(r, []string{"request", "request", "db.query", "GET /a", "", "ü<&>"}),
		StartMs: []int64{0, 1700000000000, r.Int63n(1 << 41)}[r.Intn(3)],
		Micros:  []int64{0, 1, 1500, r.Int63n(1e7)}[r.Intn(4)],
		Status:  []int64{0, 0, 200, 404, 499, 500, 503}[r.Intn(7)],
	}
	if r.Intn(2) == 0 {
		s.TraceID = pick(r, []string{"4bf92f3577b34da6a3ce929d0e0e4736", "4BF92F3577B34DA6A3CE929D0E0E4736", "abc", "zz", "00000000000000000000000000000000", "0102", "0102030405060708090a0b0c0d0e0f1011"})
	}
	if r.Intn(2) == 0 {
		s.SpanID = pick(r, []string{"00f067aa0ba902b7", "00F067AA0BA902B7", "0102", "xyz1", "0000000000000000", "010203040506070809"})
	}
	if r.Intn(2) == 0 {
		s.ParentID = pick(r, []string{"root", "root", "00f067aa0ba902b7", "53995c3f42cd8ad8", "q", "0000000000000000", "01"})
	}
	if r.Intn(2) == 0 {
		k := pick(r, kinds)
		s.Kind = &k
	}
	s.Attrs = genAttrs(r, 3)
	s.Events = []event{}
	for i := r.Intn(3); i > 0; i-- {
		s.Events = append(s.Events, event{Name: str(r), TimeNs: []int64{0, 1700000000123456789, r.Int63()}[r.Intn(3)], Attrs: genAttrs(r, 2)})
	}
	if r.Intn(2) == 0 {
		c := pick(r, codes)
		s.StatusCode = &c
	}
	if r.Intn(2) == 0 {
		s.StatusMessage = str(r)
	}
	if r.Intn(2) == 0 {
		s.StartNs = []int64{1700000000123456789, r.Int63n(1 << 62)}[r.Intn(2)]
	}
	if r.Intn(2) == 0 {
		s.EndNs = []int64{1700000000999999999, r.Int63n(1 << 62), 1}[r.Intn(3)]
	}
	if r.Intn(2) == 0 {
		s.Scope = pick(r, []string{"lib", "lib", "other", "std/trace"})
		s.ScopeVersion = pick(r, []string{"", "", "1.0"})
	}
	if r.Intn(3) == 0 {
		s.SchemaURL = pick(r, []string{"https://opentelemetry.io/schemas/1.26.0", "x"})
	}
	return s
}

func isHex(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

// unhex_text: whole hex pairs, either case.
func unhex(t string) ([]byte, bool) {
	if len(t)%2 != 0 {
		return nil, false
	}
	for i := 0; i < len(t); i++ {
		if !isHex(t[i]) {
			return nil, false
		}
	}
	b, _ := hex.DecodeString(t)
	if b == nil {
		b = []byte{}
	}
	return b, true
}

func sum(s string, n int) []byte { h := sha256.Sum256([]byte(s)); return h[:n] }

func traceID(id string) []byte {
	if len(id) == 32 {
		if b, ok := unhex(id); ok {
			return b
		}
	}
	return sum(id, 16)
}

func rootID(id string) []byte { return sum(id+"/request", 8) }

func anyValue(v any) *commonpb.AnyValue {
	switch x := v.(type) {
	case string:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: x}}
	case int64:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: x}}
	case float64:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: x}}
	case bool:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: x}}
	}
	panic(v)
}

func kvs(as []attr) []*commonpb.KeyValue {
	var out []*commonpb.KeyValue
	for _, a := range as {
		out = append(out, &commonpb.KeyValue{Key: a.Key, Value: anyValue(a.v)})
	}
	return out
}

func index(xs []string, x string) int {
	for i, y := range xs {
		if y == x {
			return i
		}
	}
	panic(x)
}

func spanMsg(s span) *tracepb.Span {
	root := s.Name == "request" && s.ParentID == ""
	tid := traceID(s.ID)
	if s.TraceID != "" {
		if b, ok := unhex(s.TraceID); ok {
			tid = b
		}
	}
	var sid []byte
	if s.SpanID != "" {
		if b, ok := unhex(s.SpanID); ok {
			sid = b
		}
	}
	if sid == nil {
		if root {
			sid = rootID(s.ID)
		} else {
			sid = sum(s.ID+"/"+s.Name+"/"+strconv.FormatInt(s.StartMs, 10)+"/"+strconv.FormatInt(s.Micros, 10), 8)
		}
	}
	var parent []byte
	switch {
	case s.ParentID == "root":
	case s.ParentID == "":
		if !root {
			parent = rootID(s.ID)
		}
	default:
		if b, ok := unhex(s.ParentID); ok {
			parent = b
		} else {
			parent = rootID(s.ID)
		}
	}
	kind := 1
	if s.Kind != nil {
		if k := index(kinds, *s.Kind); k != 0 {
			kind = k
		}
	} else if root {
		kind = 2
	}
	start := s.StartMs * 1000000
	if s.StartNs > 0 {
		start = s.StartNs
	}
	end := start + s.Micros*1000
	if s.EndNs > 0 {
		end = s.EndNs
	}
	httpStatus := s.Status > 0 && s.Name == "request" && s.ParentID == ""
	var attrs []*commonpb.KeyValue
	if httpStatus {
		attrs = append(attrs, &commonpb.KeyValue{Key: "http.response.status_code", Value: anyValue(s.Status)})
	}
	attrs = append(attrs, kvs(s.Attrs)...)
	var events []*tracepb.Span_Event
	for _, e := range s.Events {
		events = append(events, &tracepb.Span_Event{TimeUnixNano: uint64(e.TimeNs), Name: e.Name, Attributes: kvs(e.Attrs)})
	}
	var st *tracepb.Status
	if s.StatusCode != nil {
		c := index(codes, *s.StatusCode)
		st = &tracepb.Status{Code: tracepb.Status_StatusCode(c)}
		if c == 2 {
			st.Message = s.StatusMessage
		}
	} else if httpStatus {
		st = &tracepb.Status{Code: tracepb.Status_STATUS_CODE_OK}
		if s.Status >= 500 {
			st.Code = tracepb.Status_STATUS_CODE_ERROR
		}
	}
	otel := s.SpanID != ""
	var flags uint32
	if otel {
		flags = 257
		if st == nil {
			st = &tracepb.Status{}
		}
	}
	return &tracepb.Span{TraceId: tid, SpanId: sid, ParentSpanId: parent, Name: s.Name, Kind: tracepb.Span_SpanKind(kind),
		StartTimeUnixNano: uint64(start), EndTimeUnixNano: uint64(end), Attributes: attrs, Events: events, Status: st, Flags: flags}
}

func request(res resource, spans []span) *coltracepb.ExportTraceServiceRequest {
	rattrs := []*commonpb.KeyValue{}
	for _, kv := range append([][2]string{{"service.name", res.Service}}, res.Attributes...) {
		v := &commonpb.AnyValue{}
		if kv[1] != "" {
			v.Value = &commonpb.AnyValue_StringValue{StringValue: kv[1]}
		}
		rattrs = append(rattrs, &commonpb.KeyValue{Key: kv[0], Value: v})
	}
	var groups []*tracepb.ScopeSpans
	if len(spans) == 0 {
		groups = append(groups, &tracepb.ScopeSpans{Scope: &commonpb.InstrumentationScope{Name: res.Scope, Version: res.Version}})
	}
	for _, s := range spans {
		n, v := res.Scope, res.Version
		if s.Scope != "" {
			n, v = s.Scope, s.ScopeVersion
		}
		at := -1
		for i, g := range groups {
			if g.Scope.Name == n && g.Scope.Version == v && g.SchemaUrl == s.SchemaURL {
				at = i
				break
			}
		}
		if at < 0 {
			groups = append(groups, &tracepb.ScopeSpans{Scope: &commonpb.InstrumentationScope{Name: n, Version: v}, SchemaUrl: s.SchemaURL})
			at = len(groups) - 1
		}
		groups[at].Spans = append(groups[at].Spans, spanMsg(s))
	}
	return &coltracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{Resource: &resourcepb.Resource{Attributes: rattrs}, ScopeSpans: groups}}}
}

func main() {
	seed, _ := strconv.ParseInt(os.Args[1], 10, 64)
	n, _ := strconv.Atoi(os.Args[2])
	r := rand.New(rand.NewSource(seed))
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	for i := 0; i < n; i++ {
		res := resource{Service: str(r), Scope: pick(r, []string{"std/trace", "std/trace", "app", ""}), Version: pick(r, []string{"", "", "0.1"}), Attributes: [][2]string{}}
		for j := r.Intn(3); j > 0; j-- {
			res.Attributes = append(res.Attributes, [2]string{pick(r, []string{"host.name", "env", ""}), str(r)})
		}
		spans := []span{}
		for j := r.Intn(5); j > 0; j-- {
			spans = append(spans, genSpan(r))
		}
		rj, err := json.Marshal(res)
		if err != nil {
			panic(err)
		}
		sj, err := json.Marshal(spans)
		if err != nil {
			panic(err)
		}
		req := request(res, spans)
		bin, err := proto.MarshalOptions{Deterministic: true}.Marshal(req)
		if err != nil {
			panic(err)
		}
		js, err := otlpjson.MarshalExportTraceServiceRequest(req)
		if err != nil {
			panic(err)
		}
		fmt.Fprintf(w, "== %d\nc %s\t%s\nw %s %s\n", i, hex.EncodeToString(rj), hex.EncodeToString(sj), hex.EncodeToString(bin), hex.EncodeToString(js))
	}
}
