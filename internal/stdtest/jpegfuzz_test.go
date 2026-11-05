// A fuzz harness for std/jpeg against Go's image/jpeg (2026-11-03, decision
// 0461: a parser of untrusted input is Early on testing until it is fuzzed).
//
// The seeds are Go's own test images (every chroma subsampling ratio,
// baseline and progressive, restart intervals, CMYK, YCCK, RGB, gray,
// separate DC progression) and images Go encodes. Each case changes one to
// three things in a seed, at the level of its segments: bits flipped, the
// file cut, a segment dropped, repeated or swapped, a byte of a header
// changed (the frame's sampling factors and component ids, a quantization
// or Huffman table, the scan's spectral selection and successive
// approximation), a restart interval inserted, restart markers dropped into
// the entropy coded data, a segment length changed, garbage between segments.
//
// Each case is checked as Go answers it: Decode gives the image type, size,
// planes (CRC-32 and length) and RGBA conversion, or an error of the same
// kind and text; DecodeConfig gives the size and image type or the same
// error. A file Go's decoder panics on is not a case.
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/jpegfuzz/jpegfuzz_test.xo, which TestStdCorpora runs on
// every corpus backend; TestJpegFuzzCorpusCurrent checks it is what this
// generator writes (XO_JPEGFUZZ_WRITE=1 rewrites it). TestJpegFuzz runs a
// fresh seed (XO_JPEGFUZZ_SEED, default 2; XO_JPEGFUZZ_N cases, default 1000)
// on XO_JPEGFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

type jxCase struct {
	data []byte
	want string
	cfg  string
}

func jxCrop(pix []byte, stride, w, h, bpp int) []byte {
	var out []byte
	for y := 0; y < h; y++ {
		out = append(out, pix[y*stride:y*stride+w*bpp]...)
	}
	return out
}

func jxErr(err error) string {
	var fe jpeg.FormatError
	var ue jpeg.UnsupportedError
	switch {
	case errors.As(err, &fe):
		return "format " + err.Error()
	case errors.As(err, &ue):
		return "unsupported " + err.Error()
	case err == io.ErrUnexpectedEOF:
		return "truncated"
	}
	return "other " + err.Error()
}

var jxRatios = map[image.YCbCrSubsampleRatio]string{
	image.YCbCrSubsampleRatio444: "S444", image.YCbCrSubsampleRatio440: "S440", image.YCbCrSubsampleRatio422: "S422",
	image.YCbCrSubsampleRatio420: "S420", image.YCbCrSubsampleRatio411: "S411", image.YCbCrSubsampleRatio410: "S410",
}

func jxDescribe(data []byte) (desc, cfg string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		desc = "err " + jxErr(err)
	} else {
		b := img.Bounds()
		w, h := b.Dx(), b.Dy()
		rgba := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
		rc := crc32.ChecksumIEEE(rgba.Pix)
		switch m := img.(type) {
		case *image.Gray:
			p := jxCrop(m.Pix, m.Stride, w, h, 1)
			desc = fmt.Sprintf("ok Gray %dx%d S444 y=0/0 cb=0 cr=0 pix=%d/%d rgba=%d", w, h, crc32.ChecksumIEEE(p), len(p), rc)
		case *image.YCbCr:
			cw, ch := w, h
			switch m.SubsampleRatio {
			case image.YCbCrSubsampleRatio440:
				ch = (h + 1) / 2
			case image.YCbCrSubsampleRatio422:
				cw = (w + 1) / 2
			case image.YCbCrSubsampleRatio420:
				cw, ch = (w+1)/2, (h+1)/2
			case image.YCbCrSubsampleRatio411:
				cw = (w + 3) / 4
			case image.YCbCrSubsampleRatio410:
				cw, ch = (w+3)/4, (h+1)/2
			}
			yp := jxCrop(m.Y, m.YStride, w, h, 1)
			cb := jxCrop(m.Cb, m.CStride, cw, ch, 1)
			cr := jxCrop(m.Cr, m.CStride, cw, ch, 1)
			desc = fmt.Sprintf("ok YCbCr %dx%d %s y=%d/%d cb=%d cr=%d pix=0/0 rgba=%d", w, h, jxRatios[m.SubsampleRatio], crc32.ChecksumIEEE(yp), len(yp), crc32.ChecksumIEEE(cb), crc32.ChecksumIEEE(cr), rc)
		case *image.CMYK:
			p := jxCrop(m.Pix, m.Stride, w, h, 4)
			desc = fmt.Sprintf("ok CMYK %dx%d S444 y=0/0 cb=0 cr=0 pix=%d/%d rgba=%d", w, h, crc32.ChecksumIEEE(p), len(p), rc)
		case *image.RGBA:
			p := jxCrop(m.Pix, m.Stride, w, h, 4)
			desc = fmt.Sprintf("ok RGBA %dx%d S444 y=0/0 cb=0 cr=0 pix=%d/%d rgba=%d", w, h, crc32.ChecksumIEEE(p), len(p), rc)
		default:
			return "", "", false
		}
	}
	c, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		cfg = "err " + jxErr(err)
	} else {
		m := "YCbCr"
		switch c.ColorModel {
		case color.GrayModel:
			m = "Gray"
		case color.CMYKModel:
			m = "CMYK"
		case color.RGBAModel:
			m = "RGBA"
		}
		cfg = fmt.Sprintf("ok %s %dx%d", m, c.Width, c.Height)
	}
	return desc, cfg, true
}

// jxSeg is a segment of a JPEG file: a marker segment (ff xx, length,
// payload), or the entropy coded data after a scan header (marker 0).
type jxSeg struct {
	marker byte
	data   []byte
}

func jxParse(b []byte) []jxSeg {
	var segs []jxSeg
	if len(b) < 2 {
		return nil
	}
	segs = append(segs, jxSeg{0xd8, b[:2]})
	i := 2
	for i+1 < len(b) {
		if b[i] != 0xff {
			// not at a marker: the rest is one chunk
			segs = append(segs, jxSeg{0, b[i:]})
			return segs
		}
		m := b[i+1]
		if m == 0xd9 || (m >= 0xd0 && m <= 0xd7) || m == 0x01 || m == 0xff || m == 0 {
			segs = append(segs, jxSeg{m, b[i : i+2]})
			i += 2
			continue
		}
		if i+3 >= len(b) {
			segs = append(segs, jxSeg{0, b[i:]})
			return segs
		}
		n := int(b[i+2])<<8 | int(b[i+3])
		end := i + 2 + n
		if end > len(b) || n < 2 {
			segs = append(segs, jxSeg{0, b[i:]})
			return segs
		}
		segs = append(segs, jxSeg{m, b[i:end]})
		i = end
		if m == 0xda {
			j := i
			for j+1 < len(b) {
				if b[j] == 0xff && b[j+1] != 0 && !(b[j+1] >= 0xd0 && b[j+1] <= 0xd7) {
					break
				}
				j++
			}
			if j+1 >= len(b) {
				j = len(b)
			}
			segs = append(segs, jxSeg{0, b[i:j]})
			i = j
		}
	}
	if i < len(b) {
		segs = append(segs, jxSeg{0, b[i:]})
	}
	return segs
}

func jxJoin(segs []jxSeg) []byte {
	var out []byte
	for _, s := range segs {
		out = append(out, s.data...)
	}
	return out
}

func jxClone(segs []jxSeg) []jxSeg {
	out := make([]jxSeg, len(segs))
	for i, s := range segs {
		out[i] = jxSeg{s.marker, append([]byte(nil), s.data...)}
	}
	return out
}

func jxFind(segs []jxSeg, m byte) []int {
	var r []int
	for i, s := range segs {
		if s.marker == m {
			r = append(r, i)
		}
	}
	return r
}

// jxMutateOnce changes the file in one way.
func jxMutateOnce(r *rand.Rand, b []byte) []byte {
	segs := jxClone(jxParse(b))
	if len(segs) < 3 {
		if len(b) > 0 {
			c := append([]byte(nil), b...)
			c[r.Intn(len(c))] ^= byte(1 << r.Intn(8))
			return c
		}
		return b
	}
	pick := func(m byte) int {
		ix := jxFind(segs, m)
		if len(ix) == 0 {
			return -1
		}
		return ix[r.Intn(len(ix))]
	}
	switch k := r.Intn(16); k {
	case 0, 1: // flip a bit anywhere
		out := jxJoin(segs)
		out[r.Intn(len(out))] ^= byte(1 << r.Intn(8))
		return out
	case 2: // cut
		out := jxJoin(segs)
		return out[:r.Intn(len(out))]
	case 3: // drop a segment
		i := 1 + r.Intn(len(segs)-1)
		return jxJoin(append(segs[:i:i], segs[i+1:]...))
	case 4: // repeat a segment
		i := 1 + r.Intn(len(segs)-1)
		out := append(segs[:i+1:i+1], segs[i:]...)
		return jxJoin(out)
	case 5: // swap neighbours
		i := 1 + r.Intn(len(segs)-2)
		segs[i], segs[i+1] = segs[i+1], segs[i]
		return jxJoin(segs)
	case 6: // a byte of some header
		i := 1 + r.Intn(len(segs)-1)
		if len(segs[i].data) > 4 && segs[i].marker != 0 {
			segs[i].data[4+r.Intn(min(len(segs[i].data)-4, 24))] = byte(r.Intn(256))
		}
		return jxJoin(segs)
	case 7: // the sampling factors, ids or table selectors of the frame
		i := pick(0xc0)
		if i < 0 {
			i = pick(0xc2)
		}
		if i < 0 {
			i = pick(0xc1)
		}
		if i >= 0 && len(segs[i].data) > 10 {
			d := segs[i].data
			nc := int(d[9])
			if nc > 0 && 10+3*nc <= len(d) {
				c := r.Intn(nc)
				switch r.Intn(3) {
				case 0:
					d[11+3*c] = []byte{0x11, 0x12, 0x21, 0x22, 0x41, 0x42, 0x14, 0x24, 0x31, 0x13, 0x33, 0x44, 0x00, 0x51}[r.Intn(14)]
				case 1:
					d[10+3*c] = byte(r.Intn(6))
				default:
					d[12+3*c] = byte(r.Intn(5))
				}
			}
		}
		return jxJoin(segs)
	case 8: // the scan header
		i := pick(0xda)
		if i >= 0 && len(segs[i].data) > 6 {
			d := segs[i].data
			switch r.Intn(3) {
			case 0:
				d[len(d)-3] = byte(r.Intn(70)) // Ss
			case 1:
				d[len(d)-2] = byte(r.Intn(70)) // Se
			default:
				d[len(d)-1] = byte(r.Intn(256)) // Ah, Al
			}
			if r.Intn(4) == 0 && len(d) > 7 {
				d[5+r.Intn(len(d)-8)] = byte(r.Intn(256))
			}
		}
		return jxJoin(segs)
	case 9: // a restart interval before the first scan
		i := pick(0xda)
		if i >= 0 {
			dri := jxSeg{0xdd, []byte{0xff, 0xdd, 0, 4, 0, byte(r.Intn(6))}}
			out := append(segs[:i:i], append([]jxSeg{dri}, segs[i:]...)...)
			return jxJoin(out)
		}
		return jxJoin(segs)
	case 10: // restart markers in the entropy coded data
		var ecs []int
		for i, s := range segs {
			if s.marker == 0 && i > 0 && len(s.data) > 4 && s.data[0] != 0xff {
				ecs = append(ecs, i)
			}
		}
		if len(ecs) > 0 {
			i := ecs[r.Intn(len(ecs))]
			d := segs[i].data
			at := 1 + r.Intn(len(d)-1)
			rst := []byte{0xff, byte(0xd0 + r.Intn(8))}
			segs[i].data = append(append(d[:at:at], rst...), d[at:]...)
		}
		return jxJoin(segs)
	case 11: // a segment length
		i := 1 + r.Intn(len(segs)-1)
		if len(segs[i].data) > 4 && segs[i].marker != 0 {
			d := segs[i].data
			n := int(d[2])<<8 | int(d[3])
			n += []int{-3, -2, -1, 1, 2, 3, 100}[r.Intn(7)]
			if n < 0 {
				n = 0
			}
			d[2], d[3] = byte(n>>8), byte(n)
		}
		return jxJoin(segs)
	case 12: // a Huffman table's counts
		i := pick(0xc4)
		if i >= 0 && len(segs[i].data) > 22 {
			segs[i].data[5+r.Intn(16)] = byte(r.Intn(8))
		}
		return jxJoin(segs)
	case 13: // garbage or an unknown segment between segments
		i := 1 + r.Intn(len(segs)-1)
		var extra []byte
		switch r.Intn(4) {
		case 0:
			extra = []byte{byte(r.Intn(256)), byte(r.Intn(256))}
		case 1:
			extra = []byte{0xff, byte(0xe0 + r.Intn(16)), 0, 4, 1, 2}
		case 2:
			extra = []byte{0xff, byte(0x01 + r.Intn(0xff)), 0, 3, 7}
		default:
			extra = []byte{0xff, 0xff, 0xff, 0xfe, 0, 5, 'a', 'b', 'c'}
		}
		out := append(segs[:i:i], append([]jxSeg{{0, extra}}, segs[i:]...)...)
		return jxJoin(out)
	case 14: // a quantization table entry
		i := pick(0xdb)
		if i >= 0 && len(segs[i].data) > 6 {
			segs[i].data[5+r.Intn(len(segs[i].data)-5)] = byte(r.Intn(256))
		}
		return jxJoin(segs)
	default: // an Adobe or JFIF marker's bytes
		i := pick(0xee)
		if i < 0 {
			i = pick(0xe0)
		}
		if i >= 0 && len(segs[i].data) > 6 {
			segs[i].data[4+r.Intn(len(segs[i].data)-4)] = byte(r.Intn(256))
		}
		return jxJoin(segs)
	}
}

func jxSeeds() (all [][]byte) {
	dir := filepath.Join(runtime.GOROOT(), "src", "image", "testdata")
	paths, _ := filepath.Glob(filepath.Join(dir, "*.jpeg"))
	sort.Strings(paths)
	for _, p := range paths {
		if b, err := os.ReadFile(p); err == nil {
			all = append(all, b)
		}
	}
	return
}

func jxEncoded(r *rand.Rand) []byte {
	w, h := 1+r.Intn(48), 1+r.Intn(40)
	var img image.Image
	if r.Intn(4) == 0 {
		m := image.NewGray(image.Rect(0, 0, w, h))
		for i := range m.Pix {
			m.Pix[i] = byte(i*3 + r.Intn(40))
		}
		img = m
	} else {
		m := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i := m.PixOffset(x, y)
				m.Pix[i], m.Pix[i+1], m.Pix[i+2], m.Pix[i+3] = byte(x*5+r.Intn(30)), byte(y*7+r.Intn(30)), byte((x+y)*3), 255
			}
		}
		img = m
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 1 + r.Intn(100)})
	return buf.Bytes()
}

func jxCases(r *rand.Rand, n int) []jxCase {
	seeds := jxSeeds()
	var small [][]byte
	for _, s := range seeds {
		if len(s) <= 8000 {
			small = append(small, s)
		}
	}
	var cases []jxCase
	for len(cases) < n {
		var base []byte
		switch k := r.Intn(50); {
		case k < 29:
			base = small[r.Intn(len(small))]
		case k < 49:
			base = jxEncoded(r)
		default:
			base = seeds[r.Intn(len(seeds))]
		}
		d := base
		for i, m := 0, 1+r.Intn(3); i < m; i++ {
			d = jxMutateOnce(r, d)
		}
		desc, cfg, ok := jxDescribe(d)
		if !ok {
			continue
		}
		cases = append(cases, jxCase{d, desc, cfg})
	}
	return cases
}

const jxRenderXo = `use std/crc32
use std/jpeg

fn jx_describe(data: Bytes) -> Str {
  match jpeg.decode(data) {
    Ok(img) => {
      if img.model == jpeg.Model.YCbCr && img.cb.len() != img.chroma_width() * img.chroma_height() {
        fault("chroma plane size")
      }
      let rgba = img.to_rgba()
      "ok ${img.model} ${img.width}x${img.height} ${img.subsample} y=${crc32.checksum(img.y)}/${img.y.len()} cb=${crc32.checksum(img.cb)} cr=${crc32.checksum(img.cr)} pix=${crc32.checksum(img.pix)}/${img.pix.len()} rgba=${crc32.checksum(rgba)}"
    }
    Err(e) => jx_kind(e)
  }
}

fn jx_kind(e: jpeg.JpegErr) -> Str {
  match e {
    jpeg.JpegErr.Format(_) => "err format ${e.display()}"
    jpeg.JpegErr.Unsupported(_) => "err unsupported ${e.display()}"
    jpeg.JpegErr.Truncated => "err truncated"
    jpeg.JpegErr.Encode(_) => "err encode ${e.display()}"
  }
}

fn jx_config(data: Bytes) -> Str {
  match jpeg.decode_config(data) {
    Ok(c) => "ok ${c.model} ${c.width}x${c.height}"
    Err(e) => jx_kind(e)
  }
}

fn jx_check(what: Str, got: Str, want: Str) {
  if got != want && !got.ends_with("image too large") {
    fault(what + ": got " + got)
  }
}

`

func jxXoFile(seed int64, cases []jxCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/jpegfuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// image/jpeg; do not edit: XO_JPEGFUZZ_WRITE=1 go test ./internal/stdtest\n// -run TestJpegFuzzCorpusCurrent rewrites it.\n")
	b.WriteString(jxRenderXo)
	for i, c := range cases {
		fmt.Fprintf(&b, "test \"jpeg fuzz %d\" {\n  let s = %s\n", i, ufBytes(c.data))
		fmt.Fprintf(&b, "  jx_check(\"decode\", jx_describe(s), %s)\n", xoStr(c.want))
		fmt.Fprintf(&b, "  jx_check(\"decode_config\", jx_config(s), %s)\n}\n\n", xoStr(c.cfg))
	}
	return b.String()
}

func jxCoverage(t *testing.T, cases []jxCase) {
	t.Helper()
	oks := 0
	kinds := map[string]int{}
	for _, c := range cases {
		if strings.HasPrefix(c.want, "ok ") {
			oks++
			kinds[strings.Fields(c.want)[1]]++
		} else {
			kinds[c.want]++
		}
	}
	t.Logf("answers: %d ok of %d, %d distinct", oks, len(cases), len(kinds))
	if oks < len(cases)/10 || len(kinds) < 20 {
		t.Fatalf("the cases stopped covering the decoder: %d ok, %d distinct answers", oks, len(kinds))
	}
}

const jxCorpus = "testdata/corpus/jpegfuzz/jpegfuzz_test.xo"

func TestJpegFuzzCorpusCurrent(t *testing.T) {
	cases := jxCases(rand.New(rand.NewSource(1)), 350)
	jxCoverage(t, cases)
	text := jxXoFile(1, cases)
	if os.Getenv("XO_JPEGFUZZ_WRITE") == "1" {
		if err := os.MkdirAll(filepath.Dir(jxCorpus), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(jxCorpus, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(jxCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != text {
		t.Fatalf("%s is not what seed 1 writes (Go's image/jpeg or the generator changed): XO_JPEGFUZZ_WRITE=1 go test ./internal/stdtest -run TestJpegFuzzCorpusCurrent", jxCorpus)
	}
}

func TestJpegFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the jpeg fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_JPEGFUZZ_SEED", 2))
	n := envInt(t, "XO_JPEGFUZZ_N", 1000)
	cases := jxCases(rand.New(rand.NewSource(seed)), n)
	runFuzzDir(t, "jpegfuzz_test.xo", jxXoFile(seed, cases), envBackends("XO_JPEGFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
