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
