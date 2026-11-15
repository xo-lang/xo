package rt

// std/compress on the Go backend (decision 0305): Go's compress/gzip,
// compress/flate, and compress/zlib over whole buffers, the decoders read
// through a limit of max_size + 1 bytes.

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/lzw"
	"compress/zlib"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// M_display is the error's text.
func (e Compress_CompressErr) M_display(*Task) string { return e.F_msg }

// compressLevel is Go's level of l: a fault outside -2 to 9.
func compressLevel(l Compress_Level) int {
	switch l.Tag {
	case 1:
		return flate.NoCompression
	case 2:
		return flate.BestSpeed
	case 3:
		return flate.DefaultCompression
	case 4:
		return flate.BestCompression
	case 5:
		return flate.HuffmanOnly
	}
	n := l.P.(Compress_Level_Of).F_n
	if n < -2 || n > 9 {
		FaultHere("compress: invalid compression level %d: want value in range [-2, 9]", n)
	}
	return int(n)
}

// compressDict is dict as Go's writers and readers take it: nil when
// empty (Go's zlib writers set FDICT for any non-nil dictionary).
func compressDict(dict Bytes) []byte {
	if len(dict) == 0 {
		return nil
	}
	return []byte(dict)
}

func compressNoGzipDict(format Compress_Format, dict Bytes) {
	if format.Tag == 1 && len(dict) > 0 {
		FaultHere("compress: gzip has no preset dictionary")
	}
}

func compressFail(format string, max int64, err error) Compress_CompressErr {
	if err == errCompressLimit {
		return Compress_CompressErr{F_msg: format + ": output exceeds max_size (" + strconv.FormatInt(max, 10) + " bytes)"}
	}
	return Compress_CompressErr{F_msg: err.Error()}
}

var errCompressLimit = fmt.Errorf("compress: limit")

// compressRead reads r to the end, at most max bytes.
func compressRead(r io.Reader, max int64) ([]byte, error) {
	if max < 0 {
		max = 0
	}
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, errCompressLimit
	}
	return b, nil
}

func Compress_gzip(_ *Task, data Bytes, level Compress_Level) Bytes {
	var b bytes.Buffer
	w, _ := gzip.NewWriterLevel(&b, compressLevel(level))
	w.Write([]byte(data))
	w.Close()
	return Bytes(b.Bytes())
}

func Compress_gunzip(_ *Task, data Bytes, max int64) Result[Bytes, Compress_CompressErr] {
	r, err := gzip.NewReader(bytes.NewReader([]byte(data)))
	if err != nil {
		return Err[Bytes](compressFail("gzip", max, err))
	}
	out, err := compressRead(r, max)
	if err != nil {
		return Err[Bytes](compressFail("gzip", max, err))
	}
	return Ok[Bytes, Compress_CompressErr](Bytes(out))
}

// Compress_gzip_with is gzip.Writer with its Header set.
func Compress_gzip_with(_ *Task, data Bytes, h Compress_GzipHeader, level Compress_Level) Result[Bytes, Compress_CompressErr] {
	var b bytes.Buffer
	w, _ := gzip.NewWriterLevel(&b, compressLevel(level))
	w.Name = h.F_name
	w.Comment = h.F_comment
	if len(h.F_extra) > 0 {
		w.Extra = []byte(h.F_extra)
	}
	if h.F_mtime > 0 {
		w.ModTime = time.Unix(h.F_mtime, 0)
	}
	w.OS = byte(h.F_os)
	_, err := w.Write([]byte(data))
	if err == nil {
		err = w.Close()
	}
	if err != nil {
		return Err[Bytes](Compress_CompressErr{F_msg: err.Error()})
	}
	return Ok[Bytes, Compress_CompressErr](Bytes(b.Bytes()))
}

// Compress_gzip_header is gzip.NewReader's Header.
func Compress_gzip_header(_ *Task, data Bytes) Result[Compress_GzipHeader, Compress_CompressErr] {
	r, err := gzip.NewReader(bytes.NewReader([]byte(data)))
	if err != nil {
		return Err[Compress_GzipHeader](Compress_CompressErr{F_msg: err.Error()})
	}
	h := Compress_GzipHeader{F_name: r.Name, F_comment: r.Comment, F_extra: Bytes(r.Extra), F_os: int64(r.OS)}
	if !r.ModTime.IsZero() {
		h.F_mtime = r.ModTime.Unix()
	}
	return Ok[Compress_GzipHeader, Compress_CompressErr](h)
}

func Compress_deflate(_ *Task, data Bytes, level Compress_Level, dict Bytes) Bytes {
	var b bytes.Buffer
	w, _ := flate.NewWriterDict(&b, compressLevel(level), compressDict(dict))
	w.Write([]byte(data))
	w.Close()
	return Bytes(b.Bytes())
}

func Compress_inflate(_ *Task, data Bytes, max int64, dict Bytes) Result[Bytes, Compress_CompressErr] {
	out, err := compressRead(flate.NewReaderDict(bytes.NewReader([]byte(data)), compressDict(dict)), max)
	if err != nil {
		return Err[Bytes](compressFail("flate", max, err))
	}
	return Ok[Bytes, Compress_CompressErr](Bytes(out))
}

func Compress_zlib(_ *Task, data Bytes, level Compress_Level, dict Bytes) Bytes {
	var b bytes.Buffer
	w, _ := zlib.NewWriterLevelDict(&b, compressLevel(level), compressDict(dict))
	w.Write([]byte(data))
	w.Close()
	return Bytes(b.Bytes())
}

func Compress_unzlib(_ *Task, data Bytes, max int64, dict Bytes) Result[Bytes, Compress_CompressErr] {
	r, err := zlib.NewReaderDict(bytes.NewReader([]byte(data)), compressDict(dict))
	if err != nil {
		return Err[Bytes](compressFail("zlib", max, err))
	}
	out, err := compressRead(r, max)
	if err != nil {
		return Err[Bytes](compressFail("zlib", max, err))
	}
	return Ok[Bytes, Compress_CompressErr](Bytes(out))
}

// Compress_Compressor is `compress.Compressor` (move only, so its writer
// is never shared): Go's gzip, flate, or zlib Writer over a buffer the
// methods drain.
type Compress_Compressor struct {
	w   io.WriteCloser
	buf *bytes.Buffer
}

func (Compress_Compressor) XoDebug(b *strings.Builder) { b.WriteString("<compressor>") }
func (x Compress_Compressor) M_debug(*Task) string     { return Debug(x) }

// Compress_compressor is `compress.compressor(format, level, dict)`.
func Compress_compressor(_ *Task, format Compress_Format, level Compress_Level, dict Bytes) Compress_Compressor {
	compressNoGzipDict(format, dict)
	lv := compressLevel(level)
	d := compressDict(dict)
	b := &bytes.Buffer{}
	var w io.WriteCloser
	switch format.Tag {
	case 1:
		w, _ = gzip.NewWriterLevel(b, lv)
	case 2:
		w, _ = flate.NewWriterDict(b, lv, d)
	default:
		w, _ = zlib.NewWriterLevelDict(b, lv, d)
	}
	return Compress_Compressor{w: w, buf: b}
}

// the writers keep writing to the buffer they were made with: drain by
// copying its bytes out and resetting it in place
func (x *Compress_Compressor) take() Bytes {
	out := Bytes(append([]byte(nil), x.buf.Bytes()...))
	x.buf.Reset()
	return out
}

func (x *Compress_Compressor) M_write(_ *Task, data Bytes) Bytes {
	x.w.Write([]byte(data))
	return x.take()
}

type compressFlusher interface{ Flush() error }

func (x *Compress_Compressor) M_flush(_ *Task) Bytes {
	if f, ok := x.w.(compressFlusher); ok {
		f.Flush()
	}
	return x.take()
}

func (x *Compress_Compressor) M_finish(_ *Task) Bytes {
	x.w.Close()
	return x.take()
}

// compressFeed is the input of a Decompressor's decoding goroutine: it
// reports when the decoder has taken all the input given (starved), so
// write returns what the input so far decodes to.
type compressFeed struct {
	mu      sync.Mutex
	cond    *sync.Cond
	in      []byte
	closed  bool
	starved bool
	done    bool
	out     []byte
	total   int64
	max     int64
	name    string
	err     error
}

func (f *compressFeed) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for len(f.in) == 0 && !f.closed {
		f.starved = true
		f.cond.Broadcast()
		f.cond.Wait()
	}
	f.starved = false
	if len(f.in) == 0 {
		return 0, io.EOF
	}
	n := copy(p, f.in)
	f.in = f.in[n:]
	return n, nil
}

func (f *compressFeed) run(format int, dict []byte) {
	var r io.Reader
	var err error
	switch format {
	case 1:
		r, err = gzip.NewReader(f)
	case 2:
		r = flate.NewReaderDict(f, dict)
	default:
		r, err = zlib.NewReaderDict(f, dict)
	}
	if err != nil {
		// the constructor's error, io.EOF for an empty gzip stream included
		f.mu.Lock()
		f.err = err
		f.done = true
		f.cond.Broadcast()
		f.mu.Unlock()
		return
	}
	buf := make([]byte, 32768)
	for err == nil {
		var n int
		n, err = r.Read(buf)
		f.mu.Lock()
		if n > 0 {
			f.total += int64(n)
			if f.total > f.max {
				// a checksum or corruption error found in the same read wins,
				// as compressRead's one shot (the limit is max_size + 1 there)
				if err == nil || err == io.EOF {
					err = errCompressLimit
				}
			} else {
				f.out = append(f.out, buf[:n]...)
			}
		}
		f.mu.Unlock()
	}
	f.mu.Lock()
	if err != io.EOF {
		f.err = err
	}
	f.done = true
	f.cond.Broadcast()
	f.mu.Unlock()
}

// Compress_Decompressor is `compress.Decompressor`: Go's reader on a
// goroutine fed through compressFeed.
type Compress_Decompressor struct {
	f *compressFeed
}

func (Compress_Decompressor) XoDebug(b *strings.Builder) { b.WriteString("<decompressor>") }
func (x Compress_Decompressor) M_debug(*Task) string     { return Debug(x) }

// Compress_decompressor is `compress.decompressor(format, max_size, dict)`.
func Compress_decompressor(_ *Task, format Compress_Format, max int64, dict Bytes) Compress_Decompressor {
	compressNoGzipDict(format, dict)
	if max < 0 {
		max = 0
	}
	f := &compressFeed{max: max, name: map[int]string{1: "gzip", 2: "flate", 3: "zlib"}[format.Tag]}
	f.cond = sync.NewCond(&f.mu)
	go f.run(format.Tag, compressDict(dict))
	return Compress_Decompressor{f: f}
}

// wait returns the output so far once the decoder took all the input
// (or ended).
func (f *compressFeed) wait() ([]byte, error) {
	for !f.done && !(f.starved && len(f.in) == 0) {
		f.cond.Wait()
	}
	out := f.out
	f.out = nil
	err := f.err
	return out, err
}

func (x *Compress_Decompressor) M_write(_ *Task, data Bytes) Result[Bytes, Compress_CompressErr] {
	f := x.f
	f.mu.Lock()
	defer f.mu.Unlock()
	f.in = append(f.in, data...)
	f.starved = false
	f.cond.Broadcast()
	out, err := f.wait()
	if err != nil {
		return Err[Bytes](compressFail(f.name, f.max, err))
	}
	return Ok[Bytes, Compress_CompressErr](Bytes(out))
}

func (x *Compress_Decompressor) M_finish(_ *Task) Result[Bytes, Compress_CompressErr] {
	f := x.f
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	f.cond.Broadcast()
	for !f.done {
		f.cond.Wait()
	}
	if f.err != nil {
		return Err[Bytes](compressFail(f.name, f.max, f.err))
	}
	out := f.out
	f.out = nil
	return Ok[Bytes, Compress_CompressErr](Bytes(out))
}

// lzwOrder is Go's order for a LzwOrder (variant tags start at 1: LSB is 1).
func lzwOrder(o Compress_LzwOrder) lzw.Order {
	if o.Tag == 1 {
		return lzw.LSB
	}
	return lzw.MSB
}

// Compress_lzw_encode is Go's compress/lzw writer over the whole buffer.
func Compress_lzw_encode(_ *Task, data Bytes, order Compress_LzwOrder, litWidth int64) Result[Bytes, Compress_CompressErr] {
	var b bytes.Buffer
	w := lzw.NewWriter(&b, lzwOrder(order), int(litWidth))
	if _, err := w.Write([]byte(data)); err != nil {
		return Err[Bytes](Compress_CompressErr{F_msg: err.Error()})
	}
	if err := w.Close(); err != nil {
		return Err[Bytes](Compress_CompressErr{F_msg: err.Error()})
	}
	return Ok[Bytes, Compress_CompressErr](Bytes(b.Bytes()))
}

// Compress_lzw_decode is Go's compress/lzw reader over the whole buffer,
// through a limit of max_size + 1 bytes.
func Compress_lzw_decode(_ *Task, data Bytes, order Compress_LzwOrder, litWidth int64, max int64) Result[Bytes, Compress_CompressErr] {
	r := lzw.NewReader(bytes.NewReader([]byte(data)), lzwOrder(order), int(litWidth))
	out, err := compressRead(r, max)
	if err != nil {
		return Err[Bytes](compressFail("lzw", max, err))
	}
	return Ok[Bytes, Compress_CompressErr](Bytes(out))
}
