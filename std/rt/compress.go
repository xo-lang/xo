package rt

// std/compress on the Go backend (decision 0305): Go's compress/gzip,
// compress/flate, and compress/zlib over whole buffers, the decoders read
// through a limit of max_size + 1 bytes.

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
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

func Compress_deflate(_ *Task, data Bytes, level Compress_Level) Bytes {
	var b bytes.Buffer
	w, _ := flate.NewWriter(&b, compressLevel(level))
	w.Write([]byte(data))
	w.Close()
	return Bytes(b.Bytes())
}

func Compress_inflate(_ *Task, data Bytes, max int64) Result[Bytes, Compress_CompressErr] {
	out, err := compressRead(flate.NewReader(bytes.NewReader([]byte(data))), max)
	if err != nil {
		return Err[Bytes](compressFail("flate", max, err))
	}
	return Ok[Bytes, Compress_CompressErr](Bytes(out))
}

func Compress_zlib(_ *Task, data Bytes, level Compress_Level) Bytes {
	var b bytes.Buffer
	w, _ := zlib.NewWriterLevel(&b, compressLevel(level))
	w.Write([]byte(data))
	w.Close()
	return Bytes(b.Bytes())
}

func Compress_unzlib(_ *Task, data Bytes, max int64) Result[Bytes, Compress_CompressErr] {
	r, err := zlib.NewReader(bytes.NewReader([]byte(data)))
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

// Compress_compressor is `compress.compressor(format, level)`.
func Compress_compressor(_ *Task, format Compress_Format, level Compress_Level) Compress_Compressor {
	lv := compressLevel(level)
	b := &bytes.Buffer{}
	var w io.WriteCloser
	switch format.Tag {
	case 1:
		w, _ = gzip.NewWriterLevel(b, lv)
	case 2:
		w, _ = flate.NewWriter(b, lv)
	default:
		w, _ = zlib.NewWriterLevel(b, lv)
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

func (f *compressFeed) run(format int) {
	var r io.Reader
	var err error
	switch format {
	case 1:
		r, err = gzip.NewReader(f)
	case 2:
		r = flate.NewReader(f)
	default:
		r, err = zlib.NewReader(f)
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
				err = errCompressLimit
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

// Compress_decompressor is `compress.decompressor(format, max_size)`.
func Compress_decompressor(_ *Task, format Compress_Format, max int64) Compress_Decompressor {
	if max < 0 {
		max = 0
	}
	f := &compressFeed{max: max, name: map[int]string{1: "gzip", 2: "flate", 3: "zlib"}[format.Tag]}
	f.cond = sync.NewCond(&f.mu)
	go f.run(format.Tag)
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
