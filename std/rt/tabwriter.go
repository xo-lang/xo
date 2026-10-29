package rt

import (
	"bytes"
	"strings"
	"text/tabwriter"
)

// Tabwriter_TabWriter is `tabwriter.TabWriter` (move only): Go's own
// text/tabwriter Writer over a buffer `flush` drains (decision 0410; the
// native backends run the Xo port, which scripts/stdcorpus checks against
// Go's test table).
type Tabwriter_TabWriter struct {
	w   *tabwriter.Writer
	buf *bytes.Buffer
}

func (Tabwriter_TabWriter) XoDebug(b *strings.Builder) { b.WriteString("<tabwriter>") }
func (x Tabwriter_TabWriter) M_debug(*Task) string     { return Debug(x) }

// Tabwriter_writer is `tabwriter.writer(min_width, tab_width, padding,
// pad_char, flags)`.
func Tabwriter_writer(_ *Task, minWidth, tabWidth, padding, padChar, flags int64) Tabwriter_TabWriter {
	if minWidth < 0 || tabWidth < 0 || padding < 0 {
		FaultHere("tabwriter: negative minwidth, tabwidth, or padding")
	}
	b := &bytes.Buffer{}
	w := tabwriter.NewWriter(b, int(minWidth), int(tabWidth), int(padding), byte(padChar), uint(flags))
	return Tabwriter_TabWriter{w: w, buf: b}
}

func (x *Tabwriter_TabWriter) M_write(_ *Task, data Bytes) {
	x.w.Write([]byte(data))
}

func (x *Tabwriter_TabWriter) M_flush(_ *Task) Bytes {
	x.w.Flush()
	out := Bytes(append([]byte(nil), x.buf.Bytes()...))
	x.buf.Reset()
	return out
}
