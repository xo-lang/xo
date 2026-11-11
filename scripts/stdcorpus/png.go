package main

import (
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// pngModelName is the Xo std/png Model of a decoded image, or "" for a
// type the module does not use.
func pngModelName(img image.Image) string {
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
	case *image.NRGBA64:
		return "NRGBA64"
	}
	return ""
}

// pngPix is the pix bytes and the palette (as 0xRRGGBBAA words) of a
// decoded image.
func pngPix(img image.Image) ([]byte, []uint32) {
	switch m := img.(type) {
	case *image.Gray:
		return m.Pix, nil
	case *image.Gray16:
		return m.Pix, nil
	case *image.Paletted:
		var pal []uint32
		for _, c := range m.Palette {
			n := color.NRGBAModel.Convert(c).(color.NRGBA)
			pal = append(pal, uint32(n.R)<<24|uint32(n.G)<<16|uint32(n.B)<<8|uint32(n.A))
		}
		return m.Pix, pal
	case *image.RGBA:
		return m.Pix, nil
	case *image.NRGBA:
		return m.Pix, nil
	case *image.RGBA64:
		return m.Pix, nil
	case *image.NRGBA64:
		return m.Pix, nil
	}
	return nil, nil
}

func pngPalCRC(pal []uint32) uint32 {
	var b []byte
	for _, p := range pal {
		b = append(b, byte(p>>24), byte(p>>16), byte(p>>8), byte(p))
	}
	return crc32.ChecksumIEEE(b)
}

// pngErrText is the first part of a want string for an error: a kind and
// Go's text for the kinds whose text Xo copies.
func pngErrText(err error) string {
	var fe png.FormatError
	var ue png.UnsupportedError
	switch {
	case errors.As(err, &fe):
		return "format " + err.Error()
	case errors.As(err, &ue):
		return "unsupported " + err.Error()
	case err == io.ErrUnexpectedEOF:
		return "truncated"
	}
	return "flate"
}

// pngDescribe is what the Xo test builds from std/png's decode.
func pngDescribe(data []byte) string {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return "err " + pngErrText(err)
	}
	pix, pal := pngPix(img)
	b := img.Bounds()
	return fmt.Sprintf("ok %s %dx%d pix=%d/%d pal=%d/%d", pngModelName(img), b.Dx(), b.Dy(), crc32.ChecksumIEEE(pix), len(pix), len(pal), pngPalCRC(pal))
}

func pngDescribeConfig(data []byte) string {
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "err " + pngErrText(err)
	}
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
	return fmt.Sprintf("ok %s %dx%d", m, cfg.Width, cfg.Height)
}

// pngSeeds are the PNG files of Go's own tests small enough for a
// corpus, by name.
func pngSeeds() (names []string, files [][]byte) {
	root := filepath.Join(runtime.GOROOT(), "src", "image", "png", "testdata")
	var paths []string
	for _, pat := range []string{"*.png", "pngsuite/*.png"} {
		m, _ := filepath.Glob(filepath.Join(root, pat))
		paths = append(paths, m...)
	}
	sort.Strings(paths)
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil || len(b) > 3000 {
			continue
		}
		rel, _ := filepath.Rel(root, p)
		names = append(names, rel)
		files = append(files, b)
	}
	return
}

// pngMutate changes a PNG in a way decoders trip on.
func pngMutate(r *rand.Rand, b []byte) []byte {
	out := append([]byte(nil), b...)
	switch r.Intn(7) {
	case 0: // flip a byte
		if len(out) > 0 {
			out[r.Intn(len(out))] ^= byte(1 + r.Intn(255))
		}
	case 1: // cut
		if len(out) > 0 {
			out = out[:r.Intn(len(out))]
		}
	case 2: // drop a stretch
		if len(out) > 8 {
			i := 8 + r.Intn(len(out)-8)
			j := i + r.Intn(min(len(out)-i, 40)+1)
			out = append(out[:i:i], out[j:]...)
		}
	case 3: // header byte (IHDR fields at 16..28)
		if len(out) > 29 {
			out[16+r.Intn(13)] = byte(r.Intn(256))
		}
	case 4: // garbage after the data
		out = append(out, byte(r.Intn(256)), byte(r.Intn(256)), byte(r.Intn(256)), byte(r.Intn(256)))
	case 5: // fix up the CRC of the first chunk after changing a header byte
		if len(out) > 33 {
			out[16+r.Intn(13)] = byte(r.Intn(20))
			c := crc32.ChecksumIEEE(out[12:29])
			out[29], out[30], out[31], out[32] = byte(c>>24), byte(c>>16), byte(c>>8), byte(c)
		}
	default: // two flips
		for k := 0; k < 2 && len(out) > 0; k++ {
			out[r.Intn(len(out))] ^= byte(1 + r.Intn(255))
		}
	}
	return out
}

// pngImage is an image the encoder corpus writes.
type pngImage struct {
	model int // 0 Gray, 1 Gray16, 2 Paletted, 3 RGBA, 4 NRGBA, 5 RGBA64, 6 NRGBA64
	w, h  int
	pix   []byte
	pal   []uint32
}

func pngFill(r *rand.Rand, pix []byte, w, bytesPerPixel int) {
	switch r.Intn(4) {
	case 0: // noise
		r.Read(pix)
	case 1: // gradient with a little noise
		ax, ay := r.Intn(7), r.Intn(7)
		for i := range pix {
			p := i / bytesPerPixel
			x, y := p%w, p/w
			pix[i] = byte(ax*x + ay*y + i%bytesPerPixel*17 + r.Intn(3))
		}
	case 2: // flat with rare noise
		c := byte(r.Intn(256))
		for i := range pix {
			pix[i] = c
			if r.Intn(40) == 0 {
				pix[i] = byte(r.Intn(256))
			}
		}
	default: // blocks
		for i := range pix {
			p := i / bytesPerPixel
			x, y := p%w, p/w
			pix[i] = byte(((x/4 + y/4) % 2) * 200)
		}
	}
}

func pngRandImage(r *rand.Rand) pngImage {
	model := r.Intn(7)
	w, h := 1+r.Intn(40), 1+r.Intn(30)
	if r.Intn(25) == 0 {
		w, h = 1+r.Intn(120), 1+r.Intn(80)
	}
	bpp := []int{1, 2, 1, 4, 4, 8, 8}[model]
	im := pngImage{model: model, w: w, h: h, pix: make([]byte, w*h*bpp)}
	pngFill(r, im.pix, w, bpp)
	switch model {
	case 2:
		n := []int{1, 2, 3, 4, 5, 16, 17, 100, 256}[r.Intn(9)]
		opaque := r.Intn(2) == 0
		for i := 0; i < n; i++ {
			a := uint32(255)
			if !opaque {
				a = uint32(r.Intn(256))
			}
			im.pal = append(im.pal, uint32(r.Intn(256))<<24|uint32(r.Intn(256))<<16|uint32(r.Intn(256))<<8|a)
		}
		for i := range im.pix {
			im.pix[i] = byte(int(im.pix[i]) % n)
		}
	case 3, 4, 5, 6:
		opaque := r.Intn(3) == 0
		step := 4
		if model >= 5 {
			step = 8
		}
		for i := 0; i < len(im.pix); i += step {
			if step == 4 {
				if opaque {
					im.pix[i+3] = 255
				}
				if model == 3 { // premultiplied: color <= alpha
					a := im.pix[i+3]
					for k := 0; k < 3; k++ {
						if im.pix[i+k] > a {
							im.pix[i+k] = byte(int(im.pix[i+k]) % (int(a) + 1))
						}
					}
				}
			} else {
				if opaque {
					im.pix[i+6], im.pix[i+7] = 255, 255
				}
				if model == 5 {
					a := int(im.pix[i+6])<<8 | int(im.pix[i+7])
					for k := 0; k < 3; k++ {
						v := int(im.pix[i+2*k])<<8 | int(im.pix[i+2*k+1])
						if v > a {
							v %= a + 1
							im.pix[i+2*k], im.pix[i+2*k+1] = byte(v>>8), byte(v)
						}
					}
				}
			}
		}
	}
	return im
}

func (im pngImage) goImage() image.Image {
	rect := image.Rect(0, 0, im.w, im.h)
	switch im.model {
	case 0:
		m := image.NewGray(rect)
		copy(m.Pix, im.pix)
		return m
	case 1:
		m := image.NewGray16(rect)
		copy(m.Pix, im.pix)
		return m
	case 2:
		var pal color.Palette
		for _, p := range im.pal {
			pal = append(pal, color.NRGBA{byte(p >> 24), byte(p >> 16), byte(p >> 8), byte(p)})
		}
		m := image.NewPaletted(rect, pal)
		copy(m.Pix, im.pix)
		return m
	case 3:
		m := image.NewRGBA(rect)
		copy(m.Pix, im.pix)
		return m
	case 4:
		m := image.NewNRGBA(rect)
		copy(m.Pix, im.pix)
		return m
	case 5:
		m := image.NewRGBA64(rect)
		copy(m.Pix, im.pix)
		return m
	}
	m := image.NewNRGBA64(rect)
	copy(m.Pix, im.pix)
	return m
}

func xwords(w []uint32) string {
	var b strings.Builder
	b.WriteString("[")
	for i, x := range w {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "0x%08x", x)
	}
	b.WriteString("]")
	return b.String()
}

func pngCorpus() string {
	r := rand.New(rand.NewSource(42))
	names, files := pngSeeds()
	var o strings.Builder
	o.WriteString(header("image/png: Decode and DecodeConfig on Go's test images (testdata and pngsuite), on changed copies of them, and on images Go encoded; Encode of seeded random images at every compression level"))
	o.WriteString("use std/binary\nuse std/crc32\nuse std/png\n\n")

	// the inputs of the decode tests: the seeds, changed seeds, Go encoded images
	type dcase struct {
		data []byte
	}
	var cases [][]byte
	for _, f := range files {
		cases = append(cases, f)
	}
	for _, f := range files {
		for k := 0; k < 2; k++ {
			cases = append(cases, pngMutate(r, f))
		}
	}
	var encs []pngImage
	var levels []int
	var wants [][]byte
	lv := []png.CompressionLevel{png.DefaultCompression, png.NoCompression, png.BestSpeed, png.BestCompression}
	for i := 0; i < 160; i++ {
		im := pngRandImage(r)
		l := r.Intn(4)
		var buf bytes.Buffer
		enc := png.Encoder{CompressionLevel: lv[l]}
		if err := enc.Encode(&buf, im.goImage()); err != nil {
			panic(err)
		}
		encs = append(encs, im)
		levels = append(levels, l)
		wants = append(wants, buf.Bytes())
		if i%4 == 0 {
			cases = append(cases, buf.Bytes())
			cases = append(cases, pngMutate(r, buf.Bytes()))
		}
	}

	o.WriteString("/// (PNG bytes, Go's Decode result: `ok MODEL WxH pix=CRC32/LEN pal=N/CRC32` or `err KIND [TEXT]`, Go's DecodeConfig result)\nconst DECODE: List[(Bytes, Str, Str)] = [\n")
	for _, c := range cases {
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xbytes(c), xq(pngDescribe(c)), xq(pngDescribeConfig(c)))
	}
	o.WriteString("]\n\n")
	o.WriteString("/// (model, width, height, pix, palette, level, Go's Encode output)\nconst ENCODE: List[(Int, Int, Int, Bytes, List[Int], Int, Bytes)] = [\n")
	for i, im := range encs {
		fmt.Fprintf(&o, "  (%d, %d, %d, %s, %s, %d, %s),\n", im.model, im.w, im.h, xbytes(im.pix), xwords(im.pal), levels[i], xbytes(wants[i]))
	}
	o.WriteString("]\n\n")
	// At of the encoded images: each corner and a seeded point, as
	// color.NRGBAModel 8 bit words (what Image.at reads)
	o.WriteString("/// (ENCODE row, x, y, Go's At(x, y) as an NRGBA 0xRRGGBBAA word)\nconst AT: List[(Int, Int, Int, Int)] = [\n")
	for i, im := range encs {
		gi := im.goImage()
		pts := [][2]int{{0, 0}, {im.w - 1, 0}, {0, im.h - 1}, {im.w - 1, im.h - 1}, {r.Intn(im.w), r.Intn(im.h)}}
		for _, p := range pts {
			n := color.NRGBAModel.Convert(gi.At(p[0], p[1])).(color.NRGBA)
			fmt.Fprintf(&o, "  (%d, %d, %d, %d),\n", i, p[0], p[1], uint32(n.R)<<24|uint32(n.G)<<16|uint32(n.B)<<8|uint32(n.A))
		}
	}
	o.WriteString("]\n")
	_ = names
	o.WriteString(`
const MODELS: List[png.Model] = [png.Model.Gray, png.Model.Gray16, png.Model.Paletted, png.Model.RGBA, png.Model.NRGBA, png.Model.RGBA64, png.Model.NRGBA64]

const LEVELS: List[png.Compression] = [png.Compression.Default, png.Compression.None, png.Compression.Speed, png.Compression.Best]

/// Whether got and want are both errors of the pixel data stream. Go reads
/// the rows as the stream inflates, so when a file has several faults in it
/// (a flipped byte in the compressed data, a bad row filter, a cut), which
/// error comes first depends on the byte where each shows; Xo inflates a
/// block at a time and may name another. That the file is refused, and every
/// error that is not about the stream (chunks, header, palette, checksums),
/// are compared exactly.
fn stream_error(e: Str) -> Bool {
  e.starts_with("err flate") || e == "err truncated" || e.contains("pixel data") || e.contains("bad filter type") || e.contains("zlib:") || e.contains("flate:") || e.contains("unexpected EOF")
}

fn both_stream_errors(got: Str, want: Str) -> Bool {
  got.starts_with("err ") && want.starts_with("err ") && stream_error(got) && stream_error(want)
}

fn describe(data: Bytes) -> Str {
  match png.decode(data) {
    Ok(img) => {
      var pb = BytesBuilder.new()
      for p in img.palette {
        pb.write(binary.u32_be(p))
      }
      "ok ${img.model} ${img.width}x${img.height} pix=${crc32.checksum(img.pix)}/${img.pix.len()} pal=${img.palette.len()}/${crc32.checksum(pb.freeze())}"
    }
    Err(e) => match e {
      png.PngErr.Format(_) => "err format ${e.display()}"
      png.PngErr.Unsupported(_) => "err unsupported ${e.display()}"
      png.PngErr.Truncated => "err truncated"
      png.PngErr.Flate(_) => "err flate"
    }
  }
}

fn describe_config(data: Bytes) -> Str {
  match png.decode_config(data) {
    Ok(c) => "ok ${c.model} ${c.width}x${c.height}"
    Err(e) => match e {
      png.PngErr.Format(_) => "err format ${e.display()}"
      png.PngErr.Unsupported(_) => "err unsupported ${e.display()}"
      png.PngErr.Truncated => "err truncated"
      png.PngErr.Flate(_) => "err flate"
    }
  }
}

test "decode gives Go's image or Go's error" {
  var bad: List[(Int, Str, Str)] = []
  for (i, (data, want, _)) in DECODE.indexed() {
    let got = describe(data)
    // a header over max_pixels is refused where Go allocates from it
    if got != want && !got.ends_with("image too large") && !both_stream_errors(got, want) {
      bad.push((i, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}

test "decode_config gives Go's size and image type or Go's error" {
  var bad: List[(Int, Str, Str)] = []
  for (i, (data, _, want)) in DECODE.indexed() {
    let got = describe_config(data)
    if got != want {
      bad.push((i, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}

test "encode writes Go's bytes" {
  var bad: List[(Int, Int, Int, Int)] = []
  for (i, (model, w, h, pix, pal, level, want)) in ENCODE.indexed() {
    let img = png.Image{width: w, height: h, model: MODELS[model], pix: pix, palette: pal}
    let got = png.encode(img, LEVELS[level]).ok() ?? b""
    if got != want {
      bad.push((i, model, got.len(), want.len()))
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}

test "at reads Go's pixel, as an NRGBA word" {
  var bad: List[(Int, Int, Int, Int, Int)] = []
  for (i, x, y, want) in AT {
    let (model, w, h, pix, pal, _, _) = ENCODE[i]
    let img = png.Image{width: w, height: h, model: MODELS[model], pix: pix, palette: pal}
    let got = img.at(x, y)
    if got != want {
      bad.push((i, x, y, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}

test "a decoded encoding is the image" {
  var bad: List[Int] = []
  for (i, (_, w, h, _, _, _, want)) in ENCODE.indexed() {
    match png.decode(want) {
      Ok(img) => {
        // the type may be a plainer one (opaque NRGBA comes back as RGBA)
        if img.width != w || img.height != h {
          bad.push(i)
        }
      }
      Err(_) => bad.push(i)
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}
`)
	return o.String()
}
