package main

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
)

func jpegCrop(pix []byte, stride, w, h, bpp int) []byte {
	var out []byte
	for y := 0; y < h; y++ {
		out = append(out, pix[y*stride:y*stride+w*bpp]...)
	}
	return out
}

func jpegErrText(err error) string {
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

var jpegRatios = map[image.YCbCrSubsampleRatio]string{
	image.YCbCrSubsampleRatio444: "S444", image.YCbCrSubsampleRatio440: "S440", image.YCbCrSubsampleRatio422: "S422",
	image.YCbCrSubsampleRatio420: "S420", image.YCbCrSubsampleRatio411: "S411", image.YCbCrSubsampleRatio410: "S410",
}

// jpegDescribe is what the Xo test builds from std/jpeg's decode: the image
// type, size, planes by checksum and the RGBA conversion's checksum; ok is
// false when Go's decoder panics (the case is dropped).
func jpegDescribe(data []byte) (desc string, cfg string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		desc = "err " + jpegErrText(err)
	} else {
		b := img.Bounds()
		w, h := b.Dx(), b.Dy()
		rgba := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
		rc := crc32.ChecksumIEEE(rgba.Pix)
		switch m := img.(type) {
		case *image.Gray:
			p := jpegCrop(m.Pix, m.Stride, w, h, 1)
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
			yp := jpegCrop(m.Y, m.YStride, w, h, 1)
			cb := jpegCrop(m.Cb, m.CStride, cw, ch, 1)
			cr := jpegCrop(m.Cr, m.CStride, cw, ch, 1)
			desc = fmt.Sprintf("ok YCbCr %dx%d %s y=%d/%d cb=%d cr=%d pix=0/0 rgba=%d", w, h, jpegRatios[m.SubsampleRatio], crc32.ChecksumIEEE(yp), len(yp), crc32.ChecksumIEEE(cb), crc32.ChecksumIEEE(cr), rc)
		case *image.CMYK:
			p := jpegCrop(m.Pix, m.Stride, w, h, 4)
			desc = fmt.Sprintf("ok CMYK %dx%d S444 y=0/0 cb=0 cr=0 pix=%d/%d rgba=%d", w, h, crc32.ChecksumIEEE(p), len(p), rc)
		case *image.RGBA:
			p := jpegCrop(m.Pix, m.Stride, w, h, 4)
			desc = fmt.Sprintf("ok RGBA %dx%d S444 y=0/0 cb=0 cr=0 pix=%d/%d rgba=%d", w, h, crc32.ChecksumIEEE(p), len(p), rc)
		default:
			return "", "", false
		}
	}
	c, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		cfg = "err " + jpegErrText(err)
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

func jpegSeeds() (files [][]byte, small [][]byte) {
	dir := filepath.Join(runtime.GOROOT(), "src", "image", "testdata")
	paths, _ := filepath.Glob(filepath.Join(dir, "*.jpeg"))
	sort.Strings(paths)
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		files = append(files, b)
		if len(b) <= 8000 {
			small = append(small, b)
		}
	}
	return
}

// jpegMutate changes a JPEG in a way decoders trip on.
func jpegMutate(r *rand.Rand, b []byte) []byte {
	out := append([]byte(nil), b...)
	switch r.Intn(8) {
	case 0:
		out[r.Intn(len(out))] ^= byte(1 + r.Intn(255))
	case 1:
		out = out[:r.Intn(len(out))]
	case 2: // drop a stretch
		i := 2 + r.Intn(len(out)-2)
		j := i + r.Intn(min(len(out)-i, 30)+1)
		out = append(out[:i:i], out[j:]...)
	case 3: // change a byte near a marker
		var marks []int
		for i := 0; i+1 < len(out); i++ {
			if out[i] == 0xff && out[i+1] != 0 && out[i+1] != 0xff {
				marks = append(marks, i)
			}
		}
		if len(marks) > 0 {
			i := marks[r.Intn(len(marks))]
			out[min(i+2+r.Intn(14), len(out)-1)] = byte(r.Intn(256))
		}
	case 4: // change a marker
		var marks []int
		for i := 0; i+1 < len(out); i++ {
			if out[i] == 0xff && out[i+1] != 0 && out[i+1] != 0xff {
				marks = append(marks, i)
			}
		}
		if len(marks) > 0 {
			out[marks[r.Intn(len(marks))]+1] = byte(0xc0 + r.Intn(0x40))
		}
	case 5: // two changes
		for k := 0; k < 2; k++ {
			out[r.Intn(len(out))] ^= byte(1 << r.Intn(8))
		}
	case 6: // insert a restart interval
		var marks []int
		for i := 0; i+1 < len(out); i++ {
			if out[i] == 0xff && out[i+1] == 0xda {
				marks = append(marks, i)
			}
		}
		if len(marks) > 0 {
			i := marks[0]
			dri := []byte{0xff, 0xdd, 0, 4, 0, byte(1 + r.Intn(5))}
			out = append(out[:i:i], append(dri, out[i:]...)...)
		}
	default: // garbage at the end
		out = append(out, byte(r.Intn(256)), byte(r.Intn(256)))
	}
	return out
}

func jpegEncoded(r *rand.Rand) []byte {
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

func jpegCorpus() string {
	r := rand.New(rand.NewSource(42))
	files, small := jpegSeeds()
	var cases [][]byte
	cases = append(cases, files...)
	for _, f := range small {
		for k := 0; k < 4; k++ {
			cases = append(cases, jpegMutate(r, f))
		}
	}
	for i := 0; i < 60; i++ {
		e := jpegEncoded(r)
		cases = append(cases, e)
		if i%3 == 0 {
			cases = append(cases, jpegMutate(r, e))
		}
	}
	var o strings.Builder
	o.WriteString(header("image/jpeg: Decode and DecodeConfig on Go's test images (every subsampling ratio, baseline and progressive, restart intervals, CMYK, RGB, gray), changed copies of them, and images Go encoded"))
	o.WriteString("use std/crc32\nuse std/jpeg\n\n")
	o.WriteString("/// (JPEG bytes, Go's Decode result: `ok MODEL WxH SUBSAMPLE y=CRC/LEN cb=CRC cr=CRC pix=CRC/LEN rgba=CRC` or `err KIND [TEXT]`, Go's DecodeConfig result)\nconst CASES: List[(Bytes, Str, Str)] = [\n")
	n := 0
	for _, c := range cases {
		d, cf, ok := jpegDescribe(c)
		if !ok {
			continue
		}
		n++
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xbytes(c), xq(d), xq(cf))
	}
	o.WriteString("]\n")
	o.WriteString(`
fn describe(data: Bytes) -> Str {
  match jpeg.decode(data) {
    Ok(img) => {
      if img.model == jpeg.Model.YCbCr && img.cb.len() != img.chroma_width() * img.chroma_height() {
        fault("chroma plane size")
      }
      let rgba = img.to_rgba()
      "ok ${img.model} ${img.width}x${img.height} ${img.subsample} y=${crc32.checksum(img.y)}/${img.y.len()} cb=${crc32.checksum(img.cb)} cr=${crc32.checksum(img.cr)} pix=${crc32.checksum(img.pix)}/${img.pix.len()} rgba=${crc32.checksum(rgba)}"
    }
    Err(e) => kind(e)
  }
}

fn kind(e: jpeg.JpegErr) -> Str {
  match e {
    jpeg.JpegErr.Format(_) => "err format ${e.display()}"
    jpeg.JpegErr.Unsupported(_) => "err unsupported ${e.display()}"
    jpeg.JpegErr.Truncated => "err truncated"
    jpeg.JpegErr.Encode(_) => "err encode ${e.display()}"
  }
}

fn describe_config(data: Bytes) -> Str {
  match jpeg.decode_config(data) {
    Ok(c) => "ok ${c.model} ${c.width}x${c.height}"
    Err(e) => kind(e)
  }
}

test "decode gives Go's image or Go's error" {
  var bad: List[(Int, Str, Str)] = []
  for (i, (data, want, _)) in CASES.indexed() {
    let got = describe(data)
    if got != want && !got.ends_with("image too large") {
      bad.push((i, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}

test "decode_config gives Go's size and image type or Go's error" {
  var bad: List[(Int, Str, Str)] = []
  for (i, (data, _, want)) in CASES.indexed() {
    let got = describe_config(data)
    if got != want {
      bad.push((i, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}
`)
	_ = n
	return o.String()
}
