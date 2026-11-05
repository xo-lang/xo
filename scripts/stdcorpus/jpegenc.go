package main

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"image"
	"image/jpeg"
	"strings"
)

// jpegencPix is the n bytes of a test image, as the Xo test makes them: a
// 32 bit generator seeded from seed, in one of four modes (0 noise, 1 a
// smooth gradient with a little noise, 2 flat, 3 blocky stripes with noise).
func jpegencPix(mode, seed, n, w int) []byte {
	out := make([]byte, n)
	s := uint32(seed)*2654435761 + 1
	for i := range out {
		s = s*1664525 + 1013904223
		v := int(s >> 24)
		switch mode {
		case 0:
			out[i] = byte(v)
		case 1:
			x, y := 0, 0
			if w > 0 {
				x, y = i%w, i/w
			}
			out[i] = byte((x*7+y*3)&255 + v>>5)
		case 2:
			out[i] = byte(128 + seed&63)
		default:
			x, y := 0, 0
			if w > 0 {
				x, y = i%w, i/w
			}
			out[i] = byte((((x/4 + y/4) & 1) * 255) ^ (v >> 6))
		}
	}
	return out
}

var jpegencRatios = []struct {
	name string
	r    image.YCbCrSubsampleRatio
}{
	{"S444", image.YCbCrSubsampleRatio444}, {"S440", image.YCbCrSubsampleRatio440}, {"S422", image.YCbCrSubsampleRatio422},
	{"S420", image.YCbCrSubsampleRatio420}, {"S411", image.YCbCrSubsampleRatio411}, {"S410", image.YCbCrSubsampleRatio410},
}

// jpegencRun is Go's jpeg.Encode on the image the Xo test builds from the
// same arguments: the output, or the error text.
func jpegencRun(kind string, w, h, mode, seed, q int) ([]byte, string) {
	var m image.Image
	switch {
	case kind == "gray":
		g := image.NewGray(image.Rect(0, 0, w, h))
		copy(g.Pix, jpegencPix(mode, seed, w*h, w))
		m = g
	case kind == "rgba":
		g := image.NewRGBA(image.Rect(0, 0, w, h))
		copy(g.Pix, jpegencPix(mode, seed, w*h*4, w*4))
		m = g
	case kind == "cmyk":
		g := image.NewCMYK(image.Rect(0, 0, w, h))
		copy(g.Pix, jpegencPix(mode, seed, w*h*4, w*4))
		m = g
	default:
		var ratio image.YCbCrSubsampleRatio
		for _, r := range jpegencRatios {
			if "ycbcr:"+r.name == kind {
				ratio = r.r
			}
		}
		g := image.NewYCbCr(image.Rect(0, 0, w, h), ratio)
		copy(g.Y, jpegencPix(mode, seed, w*h, w))
		cw := 0
		if len(g.Cb) > 0 {
			cw = g.CStride
		}
		copy(g.Cb, jpegencPix(mode, seed+1, len(g.Cb), cw))
		copy(g.Cr, jpegencPix(mode, seed+2, len(g.Cr), cw))
		m = g
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, m, &jpeg.Options{Quality: q}); err != nil {
		return nil, err.Error()
	}
	return buf.Bytes(), ""
}

func jpegencCorpus() string {
	kinds := []string{"gray", "ycbcr:S444", "ycbcr:S440", "ycbcr:S422", "ycbcr:S420", "ycbcr:S411", "ycbcr:S410", "rgba", "cmyk"}
	sizes := [][2]int{{1, 1}, {8, 8}, {16, 16}, {7, 9}, {17, 31}, {33, 5}, {40, 40}, {64, 48}, {5, 70}, {0, 0}, {0, 5}, {5, 0}}
	quals := []int{1, 25, 50, 75, 90, 100, 0, 101, 49, 51}
	var o strings.Builder
	o.WriteString(header("image/jpeg: Encode (baseline 4:2:0, the standard Huffman tables, quality scaled tables) on generated images of every kind Go's encoder reads (gray, YCbCr in six subsampling ratios, RGBA, CMYK) at odd sizes and qualities, compared as CRC-32 and length of the whole output, with small outputs in full"))
	o.WriteString("use std/crc32\nuse std/jpeg\n\n")
	o.WriteString("/// (kind, width, height, mode, seed, quality, Go's output `ok CRC/LEN` or `err TEXT`)\n")
	o.WriteString("const CASES: List[(Str, Int, Int, Int, Int, Int, Str)] = [\n")
	var full []string
	n := 0
	for ki, k := range kinds {
		for si, sz := range sizes {
			for rep := 0; rep < 2; rep++ {
				n++
				mode := (ki + si + rep) % 4
				seed := 1000*ki + 10*si + rep + 1
				q := quals[(ki*3+si+rep*5)%len(quals)]
				out, e := jpegencRun(k, sz[0], sz[1], mode, seed, q)
				want := "err " + e
				if e == "" {
					want = fmt.Sprintf("ok %d/%d", crc32.ChecksumIEEE(out), len(out))
					if len(out) <= 700 && rep == 0 && si%3 == 0 {
						full = append(full, fmt.Sprintf("  (%s, %d, %d, %d, %d, %d, %s),\n", xq(k), sz[0], sz[1], mode, seed, q, xbytes(out)))
					}
				}
				fmt.Fprintf(&o, "  (%s, %d, %d, %d, %d, %d, %s),\n", xq(k), sz[0], sz[1], mode, seed, q, xq(want))
			}
		}
	}
	// an image too large to encode
	out, e := jpegencRun("gray", 65536, 1, 3, 7, 75)
	want := "err " + e
	if e == "" {
		want = fmt.Sprintf("ok %d/%d", crc32.ChecksumIEEE(out), len(out))
	}
	fmt.Fprintf(&o, "  (\"gray\", 65536, 1, 3, 7, 75, %s),\n", xq(want))
	o.WriteString("]\n\n")
	o.WriteString("/// (kind, width, height, mode, seed, quality, Go's whole output)\n")
	o.WriteString("const FULL: List[(Str, Int, Int, Int, Int, Int, Bytes)] = [\n")
	for _, f := range full {
		o.WriteString(f)
	}
	o.WriteString("]\n")
	o.WriteString(jpegencTest)
	_ = n
	return o.String()
}

const jpegencTest = `
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

test "encode gives Go's bytes (CRC-32 and length)" {
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

test "encode gives Go's whole output for small images" {
  var bad: List[Int] = []
  for (i, (kind, w, h, mode, seed, q, want)) in FULL.indexed() {
    match run(kind, w, h, mode, seed, q) {
      Ok(out) => {
        if out != want {
          bad.push(i)
        }
      }
      Err(_) => bad.push(i)
    }
  }
  expect(bad) == []
}

test "decode of an encoded image has the encoded size" {
  for (kind, w, h, mode, seed, q) in [("gray", 17, 31, 1, 3, 90), ("ycbcr:S420", 33, 5, 0, 4, 75), ("rgba", 40, 40, 3, 5, 25), ("cmyk", 16, 16, 1, 6, 100)] {
    let out = run(kind, w, h, mode, seed, q).ok() ?? fault("encode")
    let img = jpeg.decode(out).ok() ?? fault("decode")
    expect(img.width) == w
    expect(img.height) == h
  }
}
`
