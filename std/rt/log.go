package rt

import (
	"encoding/json"
	"sort"
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
			Map_put(nil, &fm, f.k, f.v)
			Map_put(nil, &vm, f.k, f.val)
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
			Map_put(nil, &fm, f.k, f.v)
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
	var out []logField
	for k, v := range Map_All(m) {
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
}

// XoDebug implements Debug.
func (Log_Logger) XoDebug(b *strings.Builder) { b.WriteString("<logger>") }

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
	l.fields = logMerged(tk, l.fields, fields)
	return l
}

// M_debug is `l.debug(msg, fields)`.
func (l Log_Logger) M_debug(tk *Task, msg string, fields Map[string, IDebug]) {
	logRecord(tk, levelDebug, msg, logFields(tk, logMerged(tk, l.fields, fields)), l.from)
}

// M_info is `l.info(msg, fields)`.
func (l Log_Logger) M_info(tk *Task, msg string, fields Map[string, IDebug]) {
	logRecord(tk, levelInfo, msg, logFields(tk, logMerged(tk, l.fields, fields)), l.from)
}

// M_warn is `l.warn(msg, fields)`.
func (l Log_Logger) M_warn(tk *Task, msg string, fields Map[string, IDebug]) {
	logRecord(tk, levelWarn, msg, logFields(tk, logMerged(tk, l.fields, fields)), l.from)
}

// M_error is `l.error(msg, fields)`.
func (l Log_Logger) M_error(tk *Task, msg string, fields Map[string, IDebug]) {
	logRecord(tk, levelError, msg, logFields(tk, logMerged(tk, l.fields, fields)), l.from)
}

// logLine is a record's line as configure writes it: text, or a JSON
// object (encoding/json of a map: keys sorted).
func logLine(level int, msg string, fields []logField, from string, now time.Time, asJSON bool) string {
	var line string
	if asJSON {
		m := map[string]any{"time": now.Format(time.RFC3339Nano), "level": levelNames[level], "msg": msg}
		for _, f := range fields {
			if f.raw != nil {
				m[f.k] = f.raw
			} else {
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
