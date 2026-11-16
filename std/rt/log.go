package rt

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// std/log (std.md 6): ambient structured logging.

const (
	levelDebug = 1
	levelInfo  = 2
	levelWarn  = 3
	levelError = 4
)

type logField struct {
	k   string
	v   string
	raw any // the value for JSON records, when it is a number or a boolean
	val Log_Value
	// group: a group's own entry (JSON writes it nested, text not at
	// all); member: one of its members under the dotted key (text and
	// Record.fields write it, JSON not; decision 0678)
	group, member bool
}

// logValue is a field's log.Value (decision 0617 C), as std/native/log.xo
// value_of makes it: numbers and booleans as themselves, a Str as Str,
// every other type (Time and Duration too, until the native side reads
// the static type's kind) as Any of its text.
func logValue(s string, x any) Log_Value {
	switch v := x.(type) {
	case int64:
		return Log_Value{Tag: 2, P: Log_Value_Int{F_n: v}}
	case int32:
		return Log_Value{Tag: 2, P: Log_Value_Int{F_n: int64(v)}}
	case int16:
		return Log_Value{Tag: 2, P: Log_Value_Int{F_n: int64(v)}}
	case int8:
		return Log_Value{Tag: 2, P: Log_Value_Int{F_n: int64(v)}}
	case uint8:
		return Log_Value{Tag: 2, P: Log_Value_Int{F_n: int64(v)}}
	case uint16:
		return Log_Value{Tag: 2, P: Log_Value_Int{F_n: int64(v)}}
	case uint32:
		return Log_Value{Tag: 2, P: Log_Value_Int{F_n: int64(v)}}
	case uint64:
		if v <= 1<<63-1 {
			return Log_Value{Tag: 2, P: Log_Value_Int{F_n: int64(v)}}
		}
		return Log_Value{Tag: 3, P: Log_Value_UInt{F_n: v}}
	case float64:
		return Log_Value{Tag: 4, P: Log_Value_Float{F_f: v}}
	case float32:
		return Log_Value{Tag: 4, P: Log_Value_Float{F_f: float64(v)}}
	case bool:
		return Log_Value{Tag: 5, P: Log_Value_Bool{F_b: v}}
	case string:
		return Log_Value{Tag: 1, P: Log_Value_Str{F_s: v}}
	}
	return Log_Value{Tag: 8, P: Log_Value_Any{F_text: s}}
}

type logSink struct {
	mu    sync.Mutex
	out   Stdio // nil: standard error, or the test's buffer when capturing
	level int
	json  bool
	// capture is set for a test (std.md 6, decision 0057): every record
	// written is kept for testing.logs(), and records with no configured
	// output go to the test's buffer instead of standard error.
	capture *logCapture
	// handler is log.configure_handler's (decision 0617 C): every record
	// goes to it instead of out; level is then the lowest level it takes
	handler Log_Handler
}

// logCapture holds the records of one test.
type logCapture struct {
	recs  []Log_Record
	lines []string // records that had no configured output, as text
}

// newTestLogSink is the log sink of a test world.
func newTestLogSink() *logSink {
	return &logSink{level: levelInfo, capture: &logCapture{}}
}

// testLogLines returns the buffered output of a test world's log sink.
func testLogLines(w *World) []string {
	if w == nil || w.log == nil || w.log.capture == nil {
		return nil
	}
	w.log.mu.Lock()
	defer w.log.mu.Unlock()
	return append([]string(nil), w.log.capture.lines...)
}

// Testing_logs is `testing.logs()` (std.md 8.6): the records logged so far
// in the current test, oldest first; empty outside a test.
func Testing_logs(tk *Task) List[Log_Record] {
	if tk == nil || tk.w == nil || tk.w.log == nil || tk.w.log.capture == nil {
		return List[Log_Record]{}
	}
	s := tk.w.log
	s.mu.Lock()
	defer s.mu.Unlock()
	return ListOf(append([]Log_Record(nil), s.capture.recs...)...)
}

var logger = &logSink{level: levelInfo}

var levelNames = map[int]string{levelDebug: "DEBUG", levelInfo: "INFO", levelWarn: "WARN", levelError: "ERROR"}

// logRecord writes one record if its level is enabled. from is the
// dependency that wrote it ("<module path>@<version>", decision 0173), ""
// for the main module's records: text records carry it in brackets after
// the level, JSON records as the key "module" (which wins over a field of
// that name, so a dependency cannot attribute its record to another).
func logRecord(tk *Task, level int, msg string, fields []logField, from string) {
	s := logger
	if tk != nil && tk.w != nil && tk.w.log != nil {
		s = tk.w.log
	}
	s.mu.Lock()
	out, min, asJSON, capture, h := s.out, s.level, s.json, s.capture, s.handler
	s.mu.Unlock()
	if level < min {
		return
	}
	var now time.Time
	if tk != nil && tk.w != nil {
		now = time.Unix(0, int64(tk.w.Now())).UTC()
	} else {
		// an effect under record and replay (core 11.3)
		now = fxNowTime(tk).UTC()
	}
	if h != nil {
		if !h.M_enabled(tk, Log_Level{Tag: level}) {
			return
		}
		var fm Map[string, string]
		var vm Map[string, Log_Value]
		for _, f := range fields {
			if !f.group {
				Map_put(nil, &fm, f.k, f.v)
			}
			if !f.member {
				Map_put(nil, &vm, f.k, f.val)
			}
		}
		if capture != nil {
			s.mu.Lock()
			capture.recs = append(capture.recs, Log_Record{F_level: Log_Level{Tag: level}, F_msg: msg, F_fields: fm, F_module: from})
			s.mu.Unlock()
		}
		h.M_handle(tk, Log_Record{F_level: Log_Level{Tag: level}, F_msg: msg, F_fields: fm, F_module: from, F_time: Time(now.UnixNano()), F_values: vm})
		return
	}
	line := logLine(level, msg, fields, from, now, asJSON)
	if capture != nil {
		var fm Map[string, string]
		for _, f := range fields {
			if !f.group {
				Map_put(nil, &fm, f.k, f.v)
			}
		}
		s.mu.Lock()
		capture.recs = append(capture.recs, Log_Record{F_level: Log_Level{Tag: level}, F_msg: msg, F_fields: fm, F_module: from})
		if out == nil {
			capture.lines = append(capture.lines, line)
		}
		s.mu.Unlock()
		if out == nil {
			return
		}
	}
	if out == nil {
		outWrite(stderrFile(), line+"\n")
		return
	}
	out.M_eprintln(tk, line)
}

func logFields(tk *Task, m Map[string, IDebug]) []logField {
	return logExpand(tk, logFieldsRaw(tk, m))
}

// logAsGroup is v as a log.Group, if it is one.
func logAsGroup(v IDebug) (Log_Group, bool) {
	if g, ok := v.(Log_Group); ok {
		return g, true
	}
	if d, ok := v.(Dyn); ok {
		if g, ok := d.V.(Log_Group); ok {
			return g, true
		}
	}
	return Log_Group{}, false
}

// logFieldsRaw is m's fields with their values, a group as its Group
// value (not expanded).
func logFieldsRaw(tk *Task, m Map[string, IDebug]) []logField {
	var out []logField
	for k, v := range Map_All(m) {
		if g, ok := logAsGroup(v); ok {
			val := Log_Value{Tag: 9, P: Log_Value_Group{F_fields: g.vals}}
			out = append(out, logField{k: k, v: logDotted(tk, g.vals), val: val, group: true})
			continue
		}
		s := ""
		var raw any
		if v != nil {
			if d, ok := v.(Dyn); ok {
				s = Display(tk, d.V)
				switch x := d.V.(type) {
				case int64, int32, int16, int8, uint8, uint16, uint32, uint64, float64, float32, bool:
					raw = x
				}
			} else if dv, ok := v.(IDisplay); ok {
				s = dv.M_display(tk)
			} else {
				s = v.M_debug(tk)
			}
		}
		var x any
		if d, ok := v.(Dyn); ok {
			x = d.V
		}
		out = append(out, logField{k: k, v: s, raw: raw, val: logValue(s, x)})
	}
	return out
}

// Log_debug is `log.debug(msg, fields)`.
func Log_debug(tk *Task, msg string, fields Map[string, IDebug]) {
	logRecord(tk, levelDebug, msg, logFields(tk, fields), "")
}

// Log_debugFrom is `log.debug(msg, fields)` called in the code of the
// dependency from (decision 0173).
func Log_debugFrom(tk *Task, from, msg string, fields Map[string, IDebug]) {
	logRecord(tk, levelDebug, msg, logFields(tk, fields), from)
}

// Log_info is `log.info(msg, fields)`.
func Log_info(tk *Task, msg string, fields Map[string, IDebug]) {
	logRecord(tk, levelInfo, msg, logFields(tk, fields), "")
}

// Log_infoFrom is `log.info(msg, fields)` called in the code of the
// dependency from (decision 0173).
func Log_infoFrom(tk *Task, from, msg string, fields Map[string, IDebug]) {
	logRecord(tk, levelInfo, msg, logFields(tk, fields), from)
}

// Log_warn is `log.warn(msg, fields)`.
func Log_warn(tk *Task, msg string, fields Map[string, IDebug]) {
	logRecord(tk, levelWarn, msg, logFields(tk, fields), "")
}

// Log_warnFrom is `log.warn(msg, fields)` called in the code of the
// dependency from (decision 0173).
func Log_warnFrom(tk *Task, from, msg string, fields Map[string, IDebug]) {
	logRecord(tk, levelWarn, msg, logFields(tk, fields), from)
}

// Log_error is `log.error(msg, fields)`.
func Log_error(tk *Task, msg string, fields Map[string, IDebug]) {
	logRecord(tk, levelError, msg, logFields(tk, fields), "")
}

// Log_errorFrom is `log.error(msg, fields)` called in the code of the
// dependency from (decision 0173).
func Log_errorFrom(tk *Task, from, msg string, fields Map[string, IDebug]) {
	logRecord(tk, levelError, msg, logFields(tk, fields), from)
}

// Log_enabled is `log.enabled(level)`: whether logRecord would write a
// record of level under the logger's configured level (decision 0558).
func Log_enabled(tk *Task, level Log_Level) bool {
	s := logger
	if tk != nil && tk.w != nil && tk.w.log != nil {
		s = tk.w.log
	}
	s.mu.Lock()
	min := s.level
	s.mu.Unlock()
	return level.Tag >= min
}

// LogEnabledN is Log_enabled for a level written as its number (1 debug
// to 4 error), as the generated code guards a log call (decision 0558).
func LogEnabledN(tk *Task, level int) bool {
	s := logger
	if tk != nil && tk.w != nil && tk.w.log != nil {
		s = tk.w.log
	}
	s.mu.Lock()
	min := s.level
	s.mu.Unlock()
	return level >= int(min)
}

// Log_configure is `log.configure(out, level, format)`: records go to the
// standard error of out.
func Log_configure(tk *Task, out Stdio, level Log_Level, format Log_Format) {
	s := logger
	if tk != nil && tk.w != nil {
		if tk.w.log == nil {
			tk.w.log = &logSink{level: levelInfo}
		}
		s = tk.w.log
	}
	s.mu.Lock()
	s.out = out
	s.level = level.Tag
	s.json = format.Tag == 2
	s.handler = nil
	s.mu.Unlock()
}

// Log_configure_handler is `log.configure_handler(h)` (decision 0617 C):
// every record goes to h; the call sites' guard lets through the lowest
// level h takes, and each record asks h.enabled again.
func Log_configure_handler(tk *Task, h Log_Handler) {
	s := logger
	if tk != nil && tk.w != nil {
		if tk.w.log == nil {
			tk.w.log = &logSink{level: levelInfo}
		}
		s = tk.w.log
	}
	min := 5
	for n := 4; n >= 1; n-- {
		if h.M_enabled(tk, Log_Level{Tag: n}) {
			min = n
		}
	}
	s.mu.Lock()
	s.handler = h
	s.level = min
	s.mu.Unlock()
}

var _ = sort.Strings

// Log_Logger is `log.Logger` (decision 0413): fields every record gets
// and the dependency the logger was made in (decision 0173).
type Log_Logger struct {
	fields Map[string, IDebug]
	from   string
	groups []logLevelGroup // with_group's levels, outermost first
}

type logLevelGroup struct {
	name   string
	fields Map[string, IDebug]
}

// M_with_group is `l.with_group(name)` (decision 0678).
func (l Log_Logger) M_with_group(_ *Task, name string) Log_Logger {
	if name == "" {
		return l
	}
	l.groups = append(append([]logLevelGroup(nil), l.groups...), logLevelGroup{name: name})
	return l
}

// all is l's fields with a record's, the record's under the innermost
// group.
func (l Log_Logger) all(tk *Task, fields Map[string, IDebug]) Map[string, IDebug] {
	if len(l.groups) == 0 {
		return logMerged(tk, l.fields, fields)
	}
	i := len(l.groups) - 1
	acc := logMerged(tk, l.groups[i].fields, fields)
	for ; i >= 0; i-- {
		parent := l.fields
		if i > 0 {
			parent = l.groups[i-1].fields
		}
		if Map_len(tk, acc) > 0 {
			var g Map[string, IDebug]
			Map_put(nil, &g, l.groups[i].name, IDebug(Log_group(tk, acc)))
			parent = logMerged(tk, parent, g)
		}
		acc = parent
	}
	return acc
}

// XoDebug implements Debug.
func (Log_Logger) XoDebug(b *strings.Builder) { b.WriteString("<logger>") }

// Log_Group is `log.Group` (decision 0678): its members' values in the
// order given.
type Log_Group struct {
	vals Map[string, Log_Value]
}

// Log_group is `log.group(fields)`.
func Log_group(tk *Task, fields Map[string, IDebug]) Log_Group {
	var vs Map[string, Log_Value]
	for _, f := range logFieldsRaw(tk, fields) {
		Map_put(nil, &vs, f.k, f.val)
	}
	return Log_Group{vals: vs}
}

// M_debug is the dotted form.
func (g Log_Group) M_debug(tk *Task) string { return logDotted(tk, g.vals) }

// XoDebug implements Debug.
func (g Log_Group) XoDebug(b *strings.Builder) { b.WriteString(logDotted(nil, g.vals)) }

// logGroupMark and M_display: std/native/log.xo's encoding of a group
// (GROUP_MARK, group_enc), the same on every backend.
const logGroupMark = "\x1exo.log.group\x1e"

func (g Log_Group) M_display(tk *Task) string { return logGroupMark + logGroupEnc(tk, g.vals) }

func logGroupEnc(tk *Task, fs Map[string, Log_Value]) string {
	var b strings.Builder
	lenp := func(s string) { b.WriteString(strconv.Itoa(len(s)) + ":" + s) }
	for k, v := range Map_All(fs) {
		tag, p := "a", logValueText(tk, v)
		switch v.Tag {
		case 1:
			tag = "s"
		case 2:
			tag = "i"
		case 3:
			tag, p = "u", strconv.FormatInt(int64(v.P.(Log_Value_UInt).F_n), 10)
		case 4:
			tag, p = "f", strconv.FormatInt(int64(math.Float64bits(v.P.(Log_Value_Float).F_f)), 10)
		case 5:
			tag, p = "b", "0"
			if v.P.(Log_Value_Bool).F_b {
				p = "1"
			}
		case 9:
			tag, p = "g", logGroupEnc(tk, v.P.(Log_Value_Group).F_fields)
		}
		lenp(k)
		b.WriteString(tag)
		lenp(p)
	}
	return b.String()
}

// logValueText is a value's text as the text format writes it.
func logValueText(tk *Task, v Log_Value) string {
	switch v.Tag {
	case 1:
		return v.P.(Log_Value_Str).F_s
	case 2:
		return Display(tk, v.P.(Log_Value_Int).F_n)
	case 3:
		return Display(tk, v.P.(Log_Value_UInt).F_n)
	case 4:
		return Display(tk, v.P.(Log_Value_Float).F_f)
	case 5:
		return Display(tk, v.P.(Log_Value_Bool).F_b)
	case 6:
		return Display(tk, v.P.(Log_Value_Time).F_t)
	case 7:
		return Display(tk, v.P.(Log_Value_Dur).F_d)
	case 8:
		return v.P.(Log_Value_Any).F_text
	case 9:
		return logDotted(tk, v.P.(Log_Value_Group).F_fields)
	}
	return ""
}

func logJoinKey(prefix, k string) string {
	if prefix == "" {
		return k
	}
	return prefix + "." + k
}

// logFlat is fs's members under prefix as dotted keys and texts.
func logFlat(tk *Task, prefix string, fs Map[string, Log_Value], out []logField) []logField {
	for k, v := range Map_All(fs) {
		if v.Tag == 9 {
			p := prefix
			if k != "" {
				p = logJoinKey(prefix, k)
			}
			out = logFlat(tk, p, v.P.(Log_Value_Group).F_fields, out)
			continue
		}
		out = append(out, logField{k: logJoinKey(prefix, k), v: logValueText(tk, v), raw: logRaw(v), val: v, member: true})
	}
	return out
}

func logDotted(tk *Task, fs Map[string, Log_Value]) string {
	var parts []string
	for _, f := range logFlat(tk, "", fs, nil) {
		parts = append(parts, f.k+"="+f.v)
	}
	return strings.Join(parts, " ")
}

// logRaw is a value's JSON form when it is a number or a boolean.
func logRaw(v Log_Value) any {
	switch v.Tag {
	case 2:
		return v.P.(Log_Value_Int).F_n
	case 3:
		return v.P.(Log_Value_UInt).F_n
	case 4:
		return v.P.(Log_Value_Float).F_f
	case 5:
		return v.P.(Log_Value_Bool).F_b
	}
	return nil
}

// logNorm applies Go's group rules: an empty key's group inlined, an
// empty group left out.
func logNorm(tk *Task, fs Map[string, Log_Value]) Map[string, Log_Value] {
	var out Map[string, Log_Value]
	for k, v := range Map_All(fs) {
		if v.Tag != 9 {
			Map_put(nil, &out, k, v)
			continue
		}
		n := logNorm(tk, v.P.(Log_Value_Group).F_fields)
		if Map_len(tk, n) == 0 {
			continue
		}
		if k == "" {
			for k2, v2 := range Map_All(n) {
				Map_put(nil, &out, k2, v2)
			}
			continue
		}
		Map_put(nil, &out, k, Log_Value{Tag: 9, P: Log_Value_Group{F_fields: n}})
	}
	return out
}

// logExpand replaces each group field by its entry and its members (by
// logNorm's rules).
func logExpand(tk *Task, fields []logField) []logField {
	var out []logField
	for _, f := range fields {
		if !f.group {
			out = append(out, f)
			continue
		}
		var one Map[string, Log_Value]
		Map_put(nil, &one, f.k, f.val)
		for k, v := range Map_All(logNorm(tk, one)) {
			if v.Tag == 9 {
				out = append(out, logField{k: k, v: logValueText(tk, v), val: v, group: true})
				out = logFlat(tk, k, v.P.(Log_Value_Group).F_fields, out)
				continue
			}
			out = append(out, logField{k: k, v: logValueText(tk, v), raw: logRaw(v), val: v})
		}
	}
	return out
}

// logJSON is a group's members as a JSON object's value.
func logJSON(tk *Task, fs Map[string, Log_Value]) map[string]any {
	m := map[string]any{}
	for k, v := range Map_All(fs) {
		switch {
		case v.Tag == 9:
			m[k] = logJSON(tk, v.P.(Log_Value_Group).F_fields)
		case logRaw(v) != nil:
			m[k] = logRaw(v)
		default:
			m[k] = logValueText(tk, v)
		}
	}
	return m
}

// Log_with is `log.with(fields)`.
func Log_with(_ *Task, fields Map[string, IDebug]) Log_Logger {
	return Log_Logger{fields: fields}
}

// Log_withFrom is `log.with(fields)` called in the code of the dependency
// from (decision 0173).
func Log_withFrom(_ *Task, from string, fields Map[string, IDebug]) Log_Logger {
	return Log_Logger{fields: fields, from: from}
}

// logMerged is a's fields then b's, b's value kept for a name in both.
func logMerged(tk *Task, a, b Map[string, IDebug]) Map[string, IDebug] {
	if Map_len(tk, b) == 0 {
		return a
	}
	m := a
	m.XoMark() // a value copy: the put must not reach a
	for k, v := range Map_All(b) {
		Map_put(tk, &m, k, v)
	}
	return m
}

// M_with is `l.with(fields)`.
func (l Log_Logger) M_with(tk *Task, fields Map[string, IDebug]) Log_Logger {
	if n := len(l.groups); n > 0 {
		gs := append([]logLevelGroup(nil), l.groups...)
		gs[n-1].fields = logMerged(tk, gs[n-1].fields, fields)
		l.groups = gs
		return l
	}
	l.fields = logMerged(tk, l.fields, fields)
	return l
}

// M_debug is `l.debug(msg, fields)`.
func (l Log_Logger) M_debug(tk *Task, msg string, fields Map[string, IDebug]) {
	logRecord(tk, levelDebug, msg, logFields(tk, l.all(tk, fields)), l.from)
}

// M_info is `l.info(msg, fields)`.
func (l Log_Logger) M_info(tk *Task, msg string, fields Map[string, IDebug]) {
	logRecord(tk, levelInfo, msg, logFields(tk, l.all(tk, fields)), l.from)
}

// M_warn is `l.warn(msg, fields)`.
func (l Log_Logger) M_warn(tk *Task, msg string, fields Map[string, IDebug]) {
	logRecord(tk, levelWarn, msg, logFields(tk, l.all(tk, fields)), l.from)
}

// M_error is `l.error(msg, fields)`.
func (l Log_Logger) M_error(tk *Task, msg string, fields Map[string, IDebug]) {
	logRecord(tk, levelError, msg, logFields(tk, l.all(tk, fields)), l.from)
}

// logLine is a record's line as configure writes it: text, or a JSON
// object (encoding/json of a map: keys sorted).
func logLine(level int, msg string, fields []logField, from string, now time.Time, asJSON bool) string {
	var line string
	if asJSON {
		m := map[string]any{"time": now.Format(time.RFC3339Nano), "level": levelNames[level], "msg": msg}
		for _, f := range fields {
			switch {
			case f.member:
			case f.group:
				m[f.k] = logJSON(nil, f.val.P.(Log_Value_Group).F_fields)
			case f.raw != nil:
				m[f.k] = f.raw
			default:
				m[f.k] = f.v
			}
		}
		if from != "" {
			m["module"] = from
		}
		data, _ := json.Marshal(m)
		line = string(data)
	} else {
		var b strings.Builder
		b.WriteString(now.Format(time.RFC3339))
		b.WriteString(" ")
		b.WriteString(levelNames[level])
		b.WriteString(" ")
		if from != "" {
			b.WriteString("[")
			b.WriteString(from)
			b.WriteString("] ")
		}
		b.WriteString(msg)
		for _, f := range fields {
			if f.group {
				continue
			}
			b.WriteString(" ")
			b.WriteString(f.k)
			b.WriteString("=")
			b.WriteString(f.v)
		}
		line = b.String()
	}
	return line
}

// The built-in sinks and handlers (decision 0617 C), as
// std/native/log.xo's.

var logDropped atomic.Int64

type logStdSink struct {
	out Stdio
	err bool
}

func (s logStdSink) M_write_line(tk *Task, line string) Result[Unit, string] {
	if s.err {
		s.out.M_eprintln(tk, line)
	} else {
		s.out.M_println(tk, line)
	}
	return Ok[Unit, string](Unit{})
}

type logFileSink struct{ f File }

func (s logFileSink) M_write_line(tk *Task, line string) Result[Unit, string] {
	if r := s.f.M_write(tk, Bytes(line+"\n")); !r.Ok {
		return Err[Unit, string](Display(tk, r.E))
	}
	return Ok[Unit, string](Unit{})
}

type logConnSink struct{ c Conn }

func (s logConnSink) M_write_line(tk *Task, line string) Result[Unit, string] {
	if r := s.c.M_write(tk, Bytes(line+"\n")); !r.Ok {
		return Err[Unit, string](Display(tk, r.E))
	}
	return Ok[Unit, string](Unit{})
}

type logFuncSink struct {
	f func(*Task, string) Result[Unit, Never]
}

func (s logFuncSink) M_write_line(tk *Task, line string) Result[Unit, string] {
	s.f(tk, line)
	return Ok[Unit, string](Unit{})
}

func Log_stderr_sink(_ *Task, out Stdio) Log_Sink { return logStdSink{out: out, err: true} }
func Log_stdout_sink(_ *Task, out Stdio) Log_Sink { return logStdSink{out: out} }
func Log_file_sink(_ *Task, f File) Log_Sink      { return logFileSink{f: f} }
func Log_conn_sink(_ *Task, c Conn) Log_Sink      { return logConnSink{c: c} }
func Log_func_sink(_ *Task, f func(*Task, string) Result[Unit, Never]) Log_Sink {
	return logFuncSink{f: f}
}

type logLineHandler struct {
	out  Log_Sink
	min  int
	json bool
}

func (h logLineHandler) M_enabled(_ *Task, level Log_Level) bool { return level.Tag >= h.min }

func (h logLineHandler) M_handle(tk *Task, r Log_Record) {
	var fields []logField
	for k, v := range Map_All(r.F_fields) {
		f := logField{k: k, v: v}
		if i := strings.IndexByte(k, '.'); i > 0 {
			// a group's member: the group's value writes it in JSON
			if o := Map_get(tk, r.F_values, k[:i]); o.Some && o.V.Tag == 9 {
				f.member = true
			}
		}
		if o := Map_get(tk, r.F_values, k); o.Some {
			x := o.V
			switch x.Tag {
			case 2:
				f.raw = x.P.(Log_Value_Int).F_n
			case 3:
				f.raw = x.P.(Log_Value_UInt).F_n
			case 4:
				f.raw = x.P.(Log_Value_Float).F_f
			case 5:
				f.raw = x.P.(Log_Value_Bool).F_b
			}
		}
		fields = append(fields, f)
	}
	for k, v := range Map_All(r.F_values) {
		if v.Tag == 9 {
			fields = append(fields, logField{k: k, val: v, group: true})
		}
	}
	line := logLine(r.F_level.Tag, r.F_msg, fields, r.F_module, time.Unix(0, int64(r.F_time)).UTC(), h.json)
	if res := h.out.M_write_line(tk, line); !res.Ok {
		logDropped.Add(1)
	}
}

func Log_text_handler(_ *Task, out Log_Sink, level Log_Level) Log_Handler {
	return logLineHandler{out: out, min: level.Tag}
}

func Log_json_handler(_ *Task, out Log_Sink, level Log_Level) Log_Handler {
	return logLineHandler{out: out, min: level.Tag, json: true}
}

type logMulti struct{ hs List[Log_Handler] }

func (m logMulti) M_enabled(tk *Task, level Log_Level) bool {
	for h := range List_All(m.hs) {
		if h.M_enabled(tk, level) {
			return true
		}
	}
	return false
}

func (m logMulti) M_handle(tk *Task, r Log_Record) {
	for h := range List_All(m.hs) {
		if h.M_enabled(tk, r.F_level) {
			h.M_handle(tk, r)
		}
	}
}

func Log_multi(_ *Task, hs List[Log_Handler]) Log_Handler { return logMulti{hs: hs} }

type logRange struct {
	h        Log_Handler
	min, max int
}

func (g logRange) M_enabled(tk *Task, level Log_Level) bool {
	return level.Tag >= g.min && level.Tag <= g.max && g.h.M_enabled(tk, level)
}

func (g logRange) M_handle(tk *Task, r Log_Record) {
	if r.F_level.Tag >= g.min && r.F_level.Tag <= g.max {
		g.h.M_handle(tk, r)
	}
}

func Log_level_range(_ *Task, h Log_Handler, min, max Log_Level) Log_Handler {
	return logRange{h: h, min: min.Tag, max: max.Tag}
}

func Log_dropped(_ *Task) int64 { return logDropped.Load() }
