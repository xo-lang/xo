// A fresh-seed differential for std/jpeg's encoder against Go's image/jpeg
// (decision 0510): random kinds (gray, YCbCr in six ratios, RGBA, CMYK),
// sizes (including empty and not a multiple of 8 or 16), pixel modes and
// qualities (including out of range), each compared with Go's whole output
// as CRC-32 and length. The committed cases are the stdcorpus jpegenc
// corpus, which TestStdCorpora runs on every backend. XO_JPEGENC_SEED
// (default 2), XO_JPEGENC_N cases (default 1500), XO_JPEGENC_BACKENDS
// (default arm64 and llvm).
package stdtest

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"image"
	"image/jpeg"
	"math/rand"
	"strings"
	"testing"
)

func jePix(mode, seed, n, w int) []byte {
	out := make([]byte, n)
	s := uint32(seed)*2654435761 + 1
	for i := range out {
		s = s*1664525 + 1013904223
		v := int(s >> 24)
		x, y := 0, 0
		if w > 0 {
			x, y = i%w, i/w
		}
		switch mode {
		case 0:
			out[i] = byte(v)
		case 1:
			out[i] = byte((x*7+y*3)&255 + v>>5)
		case 2:
			out[i] = byte(128 + seed&63)
		default:
			out[i] = byte((((x/4 + y/4) & 1) * 255) ^ (v >> 6))
		}
	}
	return out
}

var jeKinds = []string{"gray", "ycbcr:S444", "ycbcr:S440", "ycbcr:S422", "ycbcr:S420", "ycbcr:S411", "ycbcr:S410", "rgba", "cmyk"}

var jeRatios = map[string]image.YCbCrSubsampleRatio{
	"ycbcr:S444": image.YCbCrSubsampleRatio444, "ycbcr:S440": image.YCbCrSubsampleRatio440, "ycbcr:S422": image.YCbCrSubsampleRatio422,
	"ycbcr:S420": image.YCbCrSubsampleRatio420, "ycbcr:S411": image.YCbCrSubsampleRatio411, "ycbcr:S410": image.YCbCrSubsampleRatio410,
}

func jeRun(kind string, w, h, mode, seed, q int) string {
	var m image.Image
	switch kind {
	case "gray":
		g := image.NewGray(image.Rect(0, 0, w, h))
		copy(g.Pix, jePix(mode, seed, w*h, w))
		m = g
	case "rgba":
		g := image.NewRGBA(image.Rect(0, 0, w, h))
		copy(g.Pix, jePix(mode, seed, w*h*4, w*4))
		m = g
	case "cmyk":
		g := image.NewCMYK(image.Rect(0, 0, w, h))
		copy(g.Pix, jePix(mode, seed, w*h*4, w*4))
		m = g
	default:
		g := image.NewYCbCr(image.Rect(0, 0, w, h), jeRatios[kind])
		copy(g.Y, jePix(mode, seed, w*h, w))
		cw := 0
		if len(g.Cb) > 0 {
			cw = g.CStride
		}
		copy(g.Cb, jePix(mode, seed+1, len(g.Cb), cw))
		copy(g.Cr, jePix(mode, seed+2, len(g.Cr), cw))
		m = g
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, m, &jpeg.Options{Quality: q}); err != nil {
		return "err " + err.Error()
	}
	out := buf.Bytes()
	return fmt.Sprintf("ok %d/%d", crc32.ChecksumIEEE(out), len(out))
}

const jeXo = `
fn pix(mode: Int, seed: Int, n: Int, w: Int) -> Bytes {
  var out = BytesBuilder.with_capacity(n)
  var s = (seed * 2654435761 + 1) & 4294967295
  for i in 0..n {
    s = (s * 1664525 + 1013904223) & 4294967295
    let v = s >> 24
    var x = 0
    var y = 0
    if w > 0 {
      x = i % w
      y = i / w
    }
    var b = 0
    if mode == 0 {
      b = v
    } else if mode == 1 {
      b = (((x * 7 + y * 3) & 255) + (v >> 5)) & 255
    } else if mode == 2 {
      b = (128 + (seed & 63)) & 255
    } else {
      b = ((((x / 4 + y / 4) & 1) * 255) ^ (v >> 6)) & 255
    }
    out.write_byte(UInt8(b))
  }
  out.freeze()
}

fn subsample_of(kind: Str) -> jpeg.Subsample {
  if kind == "ycbcr:S444" {
    return jpeg.Subsample.S444
  } else if kind == "ycbcr:S440" {
    return jpeg.Subsample.S440
  } else if kind == "ycbcr:S422" {
    return jpeg.Subsample.S422
  } else if kind == "ycbcr:S420" {
    return jpeg.Subsample.S420
  } else if kind == "ycbcr:S411" {
    return jpeg.Subsample.S411
  }
  jpeg.Subsample.S410
}

fn image_of(kind: Str, w: Int, h: Int, mode: Int, seed: Int) -> jpeg.Image {
  if kind == "gray" {
    return jpeg.Image{width: w, height: h, model: jpeg.Model.Gray, pix: pix(mode, seed, w * h, w)}
  } else if kind == "rgba" {
    return jpeg.Image{width: w, height: h, model: jpeg.Model.RGBA, pix: pix(mode, seed, w * h * 4, w * 4)}
  } else if kind == "cmyk" {
    return jpeg.Image{width: w, height: h, model: jpeg.Model.CMYK, pix: pix(mode, seed, w * h * 4, w * 4)}
  }
  var img = jpeg.Image{width: w, height: h, model: jpeg.Model.YCbCr, subsample: subsample_of(kind)}
  let cw = img.chroma_width()
  let ch = img.chroma_height()
  img.y = pix(mode, seed, w * h, w)
  img.cb = pix(mode, seed + 1, cw * ch, cw)
  img.cr = pix(mode, seed + 2, cw * ch, cw)
  img
}

fn run(kind: Str, w: Int, h: Int, mode: Int, seed: Int, q: Int) -> Result[Bytes, jpeg.JpegErr] {
  jpeg.encode(image_of(kind, w, h, mode, seed), q)
}

`

const jeTest = `
test "encode gives Go's bytes" {
  var bad: List[(Int, Str, Str)] = []
  for (i, (kind, w, h, mode, seed, q, want)) in CASES.indexed() {
    let got = match run(kind, w, h, mode, seed, q) {
      Ok(out) => "ok ${crc32.checksum(out)}/${out.len()}"
      Err(e) => "err ${e.display()}"
    }
    if got != want {
      bad.push((i, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}
`

func TestJpegEncFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the jpeg encode fuzz is a heavy test; its seed 1 cases run in TestStdCorpora (corpus jpegenc)")
	}
	seed := int64(envInt(t, "XO_JPEGENC_SEED", 2))
	n := envInt(t, "XO_JPEGENC_N", 1500)
	r := rand.New(rand.NewSource(seed))
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/jpegencfuzz_test.go (seed %d, %d cases).\nuse std/crc32\nuse std/jpeg\n", seed, n)
	b.WriteString(jeXo)
	b.WriteString("\nconst CASES: List[(Str, Int, Int, Int, Int, Int, Str)] = [\n")
	oks := 0
	for i := 0; i < n; i++ {
		kind := jeKinds[r.Intn(len(jeKinds))]
		var w, h int
		switch r.Intn(6) {
		case 0:
			w, h = r.Intn(4), r.Intn(4)
		case 1:
			w, h = 1+r.Intn(200), 1+r.Intn(20)
		default:
			w, h = 1+r.Intn(70), 1+r.Intn(70)
		}
		mode, pseed, q := r.Intn(4), r.Intn(1<<20), r.Intn(110)-3
		want := jeRun(kind, w, h, mode, pseed, q)
		if strings.HasPrefix(want, "ok ") {
			oks++
		}
		fmt.Fprintf(&b, "  (%q, %d, %d, %d, %d, %d, %q),\n", kind, w, h, mode, pseed, q, want)
	}
	b.WriteString("]\n")
	b.WriteString(jeTest)
	if oks < n*9/10 {
		t.Fatalf("only %d of %d cases encode", oks, n)
	}
	runFuzzDir(t, "jpegenc_test.xo", b.String(), envBackends("XO_JPEGENC_BACKENDS", []string{"arm64", "llvm"}), 1)
}
