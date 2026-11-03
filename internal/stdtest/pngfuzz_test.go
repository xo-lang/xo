// A fuzz harness for std/png against Go's image/png (2026-11-03, decision
// 0461: a parser of untrusted input is Early on testing until it is fuzzed).
//
// The cases build the files a hostile upload carries, chunk by chunk with
// valid CRCs so the decoder reaches each stage: IHDR with every color type
// and bit depth (valid and not), sizes 0 to 40, interlace 0 to 2, a bad
// compression or filter method; PLTE of any length (0 to 260 bytes); tRNS of
// any length; IDAT from zlib of random rows with random filter bytes (valid
// 0 to 4 and bad), split into one to three chunks, too short or too long;
// chunks out of order, repeated, unknown critical and ancillary; a missing
// IEND or IDAT; trailing bytes. A quarter are Go-encoded images with a bit
// flipped or the file cut.
//
// Each case is checked as Go answers it: Decode gives the image type, size,
// pix (CRC-32 and length) and palette, or an error of the same kind and
// text; DecodeConfig gives the size and image type or the same error. When
// a file has several faults in the pixel data stream (a flipped byte in the
// compressed data, a bad row filter, a cut), which error comes first depends
// on the byte where each shows, so two errors of the stream are equal.
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/pngfuzz/pngfuzz_test.xo, which TestStdCorpora runs on
// every corpus backend; TestPngFuzzCorpusCurrent checks it is what this
// generator writes (XO_PNGFUZZ_WRITE=1 rewrites it). TestPngFuzz runs a
// fresh seed (XO_PNGFUZZ_SEED, default 2; XO_PNGFUZZ_N cases, default 1000)
// on XO_PNGFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type pfCase struct {
	data []byte
	want string
	cfg  string
}

func pfChunk(typ string, body []byte) []byte {
	var b bytes.Buffer
	n := uint32(len(body))
	b.Write([]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)})
	b.WriteString(typ)
	b.Write(body)
	c := crc32.ChecksumIEEE(b.Bytes()[4:])
	b.Write([]byte{byte(c >> 24), byte(c >> 16), byte(c >> 8), byte(c)})
	return b.Bytes()
}

func pfErr(err error) string {
	switch e := err.(type) {
	case png.FormatError:
		return "format " + e.Error()
	case png.UnsupportedError:
		return "unsupported " + e.Error()
	}
	if err == io.ErrUnexpectedEOF {
		return "truncated"
	}
	return "flate"
}

func pfModel(img image.Image) string {
	switch img.(type) {
	case *image.Gray:
		return "Gray"
	case *image.Gray16:
		return "Gray16"
	case *image.Paletted:
		return "Paletted"
	case *image.RGBA:
		return "RGBA"
	case *image.NRGBA:
		return "NRGBA"
	case *image.RGBA64:
		return "RGBA64"
	}
	return "NRGBA64"
}

func pfDescribe(data []byte) (string, string) {
	var want, cfgs string
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		want = "err " + pfErr(err)
	} else {
		var pix []byte
		var pal []uint32
		switch m := img.(type) {
		case *image.Gray:
			pix = m.Pix
		case *image.Gray16:
			pix = m.Pix
		case *image.Paletted:
			pix = m.Pix
			for _, c := range m.Palette {
				n := color.NRGBAModel.Convert(c).(color.NRGBA)
				pal = append(pal, uint32(n.R)<<24|uint32(n.G)<<16|uint32(n.B)<<8|uint32(n.A))
			}
		case *image.RGBA:
			pix = m.Pix
		case *image.NRGBA:
			pix = m.Pix
		case *image.RGBA64:
			pix = m.Pix
		case *image.NRGBA64:
			pix = m.Pix
		}
		var pb []byte
		for _, p := range pal {
			pb = append(pb, byte(p>>24), byte(p>>16), byte(p>>8), byte(p))
		}
		b := img.Bounds()
		want = fmt.Sprintf("ok %s %dx%d pix=%d/%d pal=%d/%d", pfModel(img), b.Dx(), b.Dy(), crc32.ChecksumIEEE(pix), len(pix), len(pal), crc32.ChecksumIEEE(pb))
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		cfgs = "err " + pfErr(err)
	} else {
		m := "Paletted"
		switch cfg.ColorModel {
		case color.GrayModel:
			m = "Gray"
		case color.Gray16Model:
			m = "Gray16"
		case color.RGBAModel:
			m = "RGBA"
		case color.NRGBAModel:
			m = "NRGBA"
		case color.RGBA64Model:
			m = "RGBA64"
		case color.NRGBA64Model:
			m = "NRGBA64"
		}
		cfgs = fmt.Sprintf("ok %s %dx%d", m, cfg.Width, cfg.Height)
	}
	return want, cfgs
}

func pfZlib(r *rand.Rand, raw []byte) []byte {
	var b bytes.Buffer
	w, _ := zlib.NewWriterLevel(&b, []int{zlib.NoCompression, zlib.BestSpeed, zlib.DefaultCompression, zlib.BestCompression}[r.Intn(4)])
	w.Write(raw)
	w.Close()
	return b.Bytes()
}

// pfBuilt is a file built chunk by chunk.
func pfBuilt(r *rand.Rand) []byte {
	var f bytes.Buffer
	f.WriteString("\x89PNG\r\n\x1a\n")
	valid := [][2]int{{0, 1}, {0, 2}, {0, 4}, {0, 8}, {0, 16}, {2, 8}, {2, 16}, {3, 1}, {3, 2}, {3, 4}, {3, 8}, {4, 8}, {4, 16}, {6, 8}, {6, 16}}
	ct, depth := valid[r.Intn(len(valid))][0], 0
	depth = valid[r.Intn(len(valid))][1]
	for _, v := range valid {
		if v[0] == ct && r.Intn(2) == 0 {
			depth = v[1]
		}
	}
	if r.Intn(12) == 0 {
		ct, depth = r.Intn(8), []int{0, 1, 3, 8, 16, 32}[r.Intn(6)]
	}
	w, h := r.Intn(41), r.Intn(41)
	if r.Intn(4) != 0 {
		w, h = 1+r.Intn(24), 1+r.Intn(24)
	}
	il := 0
	if r.Intn(3) == 0 {
		il = 1
	}
	if r.Intn(40) == 0 {
		il = 2
	}
	hdr := []byte{byte(w >> 24), byte(w >> 16), byte(w >> 8), byte(w), byte(h >> 24), byte(h >> 16), byte(h >> 8), byte(h), byte(depth), byte(ct), 0, 0, byte(il)}
	if r.Intn(30) == 0 {
		hdr[10+r.Intn(2)] = byte(1 + r.Intn(3))
	}
	if r.Intn(40) == 0 {
		hdr = hdr[:12]
	}
	f.Write(pfChunk("IHDR", hdr))
	if r.Intn(25) == 0 {
		f.Write(pfChunk([]string{"tEXt", "gAMA", "XXXX", "xxxx", "pHYs"}[r.Intn(5)], make([]byte, r.Intn(20))))
	}
	if ct == 3 || r.Intn(10) == 0 {
		n := 3 * (1 + r.Intn(1<<uint(min(depth, 8))))
		if r.Intn(6) == 0 {
			n = r.Intn(260)
		}
		pl := make([]byte, n)
		r.Read(pl)
		if r.Intn(25) != 0 {
			f.Write(pfChunk("PLTE", pl))
		}
	}
	if r.Intn(3) == 0 {
		var n int
		switch ct {
		case 0:
			n = 2
		case 2:
			n = 6
		case 3:
			n = r.Intn(260)
		default:
			n = r.Intn(8)
		}
		if r.Intn(8) == 0 {
			n = r.Intn(10)
		}
		tr := make([]byte, n)
		r.Read(tr)
		if ct == 0 || ct == 2 {
			for i := 0; i < n; i += 2 {
				tr[i] = 0
			}
		}
		f.Write(pfChunk("tRNS", tr))
	}
	// the rows
	channels := map[int]int{0: 1, 2: 3, 3: 1, 4: 2, 6: 4}[ct]
	if channels == 0 {
		channels = 1
	}
	rowBytes := func(pw int) int { return (pw*channels*max(depth, 1) + 7) / 8 }
	var raw []byte
	rows := func(pw, ph int) {
		for y := 0; y < ph; y++ {
			ft := byte(r.Intn(5))
			if r.Intn(60) == 0 {
				ft = byte(5 + r.Intn(200))
			}
			raw = append(raw, ft)
			row := make([]byte, rowBytes(pw))
			switch r.Intn(3) {
			case 0:
				r.Read(row)
			case 1:
				for i := range row {
					row[i] = byte(i*3 + y)
				}
			}
			raw = append(raw, row...)
		}
	}
	if il == 1 && w > 0 && h > 0 {
		for _, p := range [7][4]int{{0, 0, 8, 8}, {4, 0, 8, 8}, {0, 4, 4, 8}, {2, 0, 4, 4}, {0, 2, 2, 4}, {1, 0, 2, 2}, {0, 1, 1, 2}} {
			pw, ph := (w-p[0]+p[2]-1)/p[2], (h-p[1]+p[3]-1)/p[3]
			if pw > 0 && ph > 0 {
				rows(pw, ph)
			}
		}
	} else {
		rows(w, h)
	}
	switch r.Intn(14) {
	case 0:
		raw = raw[:r.Intn(len(raw)+1)]
	case 1:
		raw = append(raw, make([]byte, 1+r.Intn(30))...)
	}
	z := pfZlib(r, raw)
	if r.Intn(20) == 0 && len(z) > 0 {
		z[r.Intn(len(z))] ^= byte(1 << r.Intn(8))
	}
	switch k := r.Intn(8); {
	case k == 0: // no IDAT
	case k < 3 && len(z) > 2: // split
		a := r.Intn(len(z))
		b := a + r.Intn(len(z)-a+1)
		f.Write(pfChunk("IDAT", z[:a]))
		f.Write(pfChunk("IDAT", z[a:b]))
		f.Write(pfChunk("IDAT", z[b:]))
	default:
		f.Write(pfChunk("IDAT", z))
	}
	if r.Intn(25) == 0 {
		f.Write(pfChunk("tEXt", []byte("k\x00v")))
	}
	if r.Intn(30) != 0 {
		f.Write(pfChunk("IEND", nil))
	}
	if r.Intn(30) == 0 {
		f.Write([]byte{1, 2, 3})
	}
	return f.Bytes()
}

func pfGoEncoded(r *rand.Rand) []byte {
	w, h := 1+r.Intn(30), 1+r.Intn(30)
	var img image.Image
	switch r.Intn(4) {
	case 0:
		m := image.NewGray(image.Rect(0, 0, w, h))
		r.Read(m.Pix)
		img = m
	case 1:
		m := image.NewNRGBA(image.Rect(0, 0, w, h))
		r.Read(m.Pix)
		img = m
	case 2:
		m := image.NewRGBA64(image.Rect(0, 0, w, h))
		for i := 0; i < len(m.Pix); i += 8 {
			a := r.Intn(256)
			copy(m.Pix[i:], []byte{byte(r.Intn(a + 1)), byte(r.Intn(256)), byte(r.Intn(a + 1)), byte(r.Intn(256)), byte(r.Intn(a + 1)), byte(r.Intn(256)), byte(a), byte(r.Intn(256))})
			m.Pix[i], m.Pix[i+2], m.Pix[i+4] = 0, 0, 0
		}
		img = m
	default:
		var pal color.Palette
		for i, n := 0, 1+r.Intn(20); i < n; i++ {
			pal = append(pal, color.NRGBA{byte(r.Intn(256)), byte(r.Intn(256)), byte(r.Intn(256)), byte(r.Intn(256))})
		}
		m := image.NewPaletted(image.Rect(0, 0, w, h), pal)
		for i := range m.Pix {
			m.Pix[i] = byte(r.Intn(len(pal)))
		}
		img = m
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	out := b.Bytes()
	switch r.Intn(3) {
	case 0:
		out = out[:r.Intn(len(out))]
	case 1:
		out[r.Intn(len(out))] ^= byte(1 << r.Intn(8))
	}
	return out
}

func pfCases(r *rand.Rand, n int) []pfCase {
	var cases []pfCase
	for len(cases) < n {
		var d []byte
		if r.Intn(4) == 0 {
			d = pfGoEncoded(r)
		} else {
			d = pfBuilt(r)
		}
		want, cfg := pfDescribe(d)
		cases = append(cases, pfCase{d, want, cfg})
	}
	return cases
}

const pfRenderXo = `use std/binary
use std/crc32
use std/png

fn pf_describe(data: Bytes) -> Str {
  match png.decode(data) {
    Ok(img) => {
      var pb = BytesBuilder.new()
      for p in img.palette {
        pb.write(binary.u32_be(p))
      }
      "ok ${img.model} ${img.width}x${img.height} pix=${crc32.checksum(img.pix)}/${img.pix.len()} pal=${img.palette.len()}/${crc32.checksum(pb.freeze())}"
    }
    Err(e) => pf_kind(e)
  }
}

fn pf_kind(e: png.PngErr) -> Str {
  match e {
    png.PngErr.Format(_) => "err format ${e.display()}"
    png.PngErr.Unsupported(_) => "err unsupported ${e.display()}"
    png.PngErr.Truncated => "err truncated"
    png.PngErr.Flate(_) => "err flate"
  }
}

fn pf_config(data: Bytes) -> Str {
  match png.decode_config(data) {
    Ok(c) => "ok ${c.model} ${c.width}x${c.height}"
    Err(e) => pf_kind(e)
  }
}

fn pf_stream(e: Str) -> Bool {
  e.starts_with("err flate") || e == "err truncated" || e.contains("pixel data") || e.contains("bad filter type") || e.contains("zlib:") || e.contains("flate:") || e.contains("unexpected EOF")
}

fn pf_same(got: Str, want: Str) -> Bool {
  got == want || (got.starts_with("err ") && want.starts_with("err ") && pf_stream(got) && pf_stream(want)) || got.ends_with("image too large")
}

fn pf_check(what: Str, got: Str, want: Str) {
  if !pf_same(got, want) {
    fault(what + ": got " + got)
  }
}

`

func pfXoFile(seed int64, cases []pfCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/pngfuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// image/png; do not edit: XO_PNGFUZZ_WRITE=1 go test ./internal/stdtest\n// -run TestPngFuzzCorpusCurrent rewrites it.\n")
	b.WriteString(pfRenderXo)
	for i, c := range cases {
		fmt.Fprintf(&b, "test \"png fuzz %d\" {\n  let s = %s\n", i, ufBytes(c.data))
		fmt.Fprintf(&b, "  pf_check(\"decode\", pf_describe(s), %s)\n", xoStr(c.want))
		fmt.Fprintf(&b, "  pf_check(\"decode_config\", pf_config(s), %s)\n}\n\n", xoStr(c.cfg))
	}
	return b.String()
}

// pfCoverage fails when the cases stop reaching the decoder's parts.
func pfCoverage(t *testing.T, cases []pfCase) {
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
	if oks < len(cases)/5 || len(kinds) < 14 {
		t.Fatalf("the cases stopped covering the decoder: %d ok, %d distinct answers: %v", oks, len(kinds), kinds)
	}
}

const pfCorpus = "testdata/corpus/pngfuzz/pngfuzz_test.xo"

func TestPngFuzzCorpusCurrent(t *testing.T) {
	cases := pfCases(rand.New(rand.NewSource(1)), 1000)
	pfCoverage(t, cases)
	text := pfXoFile(1, cases)
	if os.Getenv("XO_PNGFUZZ_WRITE") == "1" {
		if err := os.MkdirAll(filepath.Dir(pfCorpus), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(pfCorpus, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(pfCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != text {
		t.Fatalf("%s is not what seed 1 writes (Go's image/png or the generator changed): XO_PNGFUZZ_WRITE=1 go test ./internal/stdtest -run TestPngFuzzCorpusCurrent", pfCorpus)
	}
}

func TestPngFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the png fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_PNGFUZZ_SEED", 2))
	n := envInt(t, "XO_PNGFUZZ_N", 1000)
	cases := pfCases(rand.New(rand.NewSource(seed)), n)
	runFuzzDir(t, "pngfuzz_test.xo", pfXoFile(seed, cases), envBackends("XO_PNGFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
