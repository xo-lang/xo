package rt

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
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
	out, min, asJSON, capture := s.out, s.level, s.json, s.capture
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
		out = append(out, logField{k: k, v: s, raw: raw})
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
