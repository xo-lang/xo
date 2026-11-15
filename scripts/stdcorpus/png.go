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

// pngDecodeTooLarge reports whether Go's Decode of data would allocate an
// image of more than 4 GiB: a mutated header (seed case: NRGBA64
// 59x117440542, 55 GB) made a bare `stdcorpus -check` reach 46 GB on the
// Mac (2026-11-15, gotchas/stdcorpus-png-huge-mutation.md). Such inputs
// are left out of DECODE; HUGE covers the dimension overflow header only.
func pngDecodeTooLarge(data []byte) bool {
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return false
	}
	return int64(cfg.Width)*int64(cfg.Height)*8 > 4<<30
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

// pngHeaderOnly is a PNG signature and an IHDR chunk for w x h of the
// given depth, color type, and interlace method, and nothing after it.
func pngHeaderOnly(w, h uint32, depth, ct, il byte) []byte {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	body := []byte{'I', 'H', 'D', 'R', byte(w >> 24), byte(w >> 16), byte(w >> 8), byte(w), byte(h >> 24), byte(h >> 16), byte(h >> 8), byte(h), depth, ct, 0, 0, il}
	b.Write([]byte{0, 0, 0, 13})
	b.Write(body)
	c := crc32.ChecksumIEEE(body)
	b.Write([]byte{byte(c >> 24), byte(c >> 16), byte(c >> 8), byte(c)})
	return b.Bytes()
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

	// fixed inputs the fuzz found, after the seeded ones so those stay:
	// png fuzz seed 20 case 4728 (2026-11-15), a 27x15 paletted image whose
	// one IDAT inflates 79 bytes of 75 and then cuts a stored block, with a
	// bad checksum; Go reports too much pixel data before the checksum
	cases = append(cases, []byte("\x89PNG\x0d\x0a\x1a\x0a\x00\x00\x00\x0dIHDR\x00\x00\x00\x1b\x00\x00\x00\x0f\x01\x03\x00\x00\x00/]&\x9b\x00\x00\x00\x03PLTE\xb2\x92)`egP\x00\x00\x00\x01tRNSk\x9a\x86`\xb6\x00\x00\x00\x0fIDATx\x9cb\xa0\x1e\x08\x04\x00\x00\xff\xff\x00K\x00\x01\xaaR:\x1b\x00\x00\x00\x00IEND\xaeB`\x82"))
	// png fuzz seed 29 case 4393 (2026-11-15), a 30x14 NRGBA image whose
	// IDAT inflates one byte past the image in a block the data cuts, with
	// a bad checksum: Go's one byte check read empties the flush the error
	// makes and gets the error with it, so Go reports the checksum
	cases = append(cases, []byte("\x89PNG\x0d\x0a\x1a\x0a\x00\x00\x00\x0dIHDR\x00\x00\x00\x1e\x00\x00\x00\x0e\x08\x06\x00\x00\x008\xe6\xac9\x00\x00\x06\xaeIDATx\x9c\x00\x9e\x06a\xf9\x02\xe6S\xb3\xc6\xe1\x87Y\xdfC\xb9\xe4\xed\x9bS\x10\x81\xb2,\xb4\xe8Sq\x88k\xa5\xf3\xde,wEG\xf7i)\xda\x0c\x962w\xa9\xc9\x10\xfb\x04\x803\x92\xa5\x0a\xe9E\xa5\x87\xca\xc6\xe9\x11\xab?\x93\x85\xafo\x95\x0e\xef\x80C\xe9\xd1\x0a<\xc9\x03]4\x09d\xef|V%3\x7f\xacC\x924\xeaJ\x07MU\xa3t\x88U\x08\xdc\xd1\xa9\xb3\x86\x1c\x08\xe6b\x0c\x0b\xb9\x15_+\x8d\x15\xd0\x19\x00a+\x00\x0b\x01J\xf5\x0d\x86\x03t+2\xbeH\xf0\x02\xc6\xbci\x9fY\xaaf1|\x00o\xdd\xdbv\xb4z\xa4\xeb\x17\xef\xae\xc8\xf5\xf8\xc3\x92Vg\xbdTw\xdb\xf3\xb1\x01\x91\xbc\xee\xbc\xedg\xcf\x1b\xb4\xb3\xa26F\x90i\xbfj\x12\x08q\x02\xb5\xaf\xb7\xaf\x8b\x10}\x0a\xaf\x91\x94`\xc7#\x97T\xeb\xec\xd9\x95m)\x92+%Ts\x9b \x92O\xe5/\xf6c:\xbc\x8e\x04\x5c\x0d+\xd3<\x1ep\xf6\xa3\x88\x1b\x00\xb8>\xaf\xa0\xf6\xa2\x06\x92\x16\x8f\x9b\x9f\xa4\xad\x14^\xdd\xa0\x93\x1f\x9fU\xe2o\xba_fg\xe7\xfd\xe114\x93DcU\x1a\x1d\x19\x01l\xfe\xb9]\xf1,z{\x82\xddK\xe2\x06\xf3?\x8d\xbdG\xb6\xfa\x0cH\xdeTQ\xcd\x0e\xa6\x97\xdf\xb8DS\xa2\x197u\xfb\x96\xe9\x09\xf76\xab\x9df\x1b\x913U\x80\xc0\x95\x0a\x18\xe5M@j\xe2\xd5\xa8\xf7\xb2\x12\xae\xea\xab6\x99\x84\xd3PV[\x03P\x15\xe5\x00h\xf6?\xcdr\x80\xb4\x16\x03+\x1e\xa63?\xd8\x1d\x84~\xfc!\xb0yB\xf4\x1e\xbfx\x90o}q\xb3'5S\xae\x02\x92\xe8\x91\xbf\x04\xd7B-7y\x05\x19#L\xde\x83\xb8c?\xc2\xe0\x00\xc7\xe9Fk\xad\xbcDy\x09\x5c\xfc\x7f\x0fD\xa4\x09\xefES!\xc0\x0a\x02\xb7\xd8\xac\x22\xfe\xf0.\xb8\x92u\x1c\x87\xbdE\xf8\xd2u\xb2wN\x1ef)\x86\xbb\x0d\xf2j\xfdqe\x97~O\x08\xad>\xb4\x04\xf4/'\x7f4\x1c\xb1\xb1\xf8\xd54\xf8\xb1>Xr!\xce$gw\xda\xa0\xf6\x18\xcd\x88\x88\xf6\xec\x0f\x096\xc7\x99\x053\x990\xe2\xa4N\x10\xda\xd2/\xfb\xb7g\xc1\x87;s\xdd\x98T]'\xd9\xc0\x08\x892\x01\x0d\x92\xcd\xf1\xd1\x13\xdf?K=\x8d+\xb5\x95\x82\xb9\xd1C\xde\xa5\x13[\xec\xaeyv\xb9\x22\xf5\xa8\xd6\xb8nK\x1ai&\xcfK\x93]\x81\xf4\xda\xfd\xa00Q\xf2\xe1\xf9\x06\x8c\x15~Y\x00\x95\x0c\xca\x19p\x96=\xe8\x89\xf9\xed\xec\xf87\x15#\xc2a3\xc9\xcec\x13\xb8\xbdeo\x05i)\xe1i,!T\x05\xcb/$}W\xb4\x94[\xabA\xac\xbf\x9af\xbeg2\x15\xfb\xf0\x87\x11DD?\xbf\xfb\xe9\x01\xda\xd1\xa5&\xdf\xfb\x86\xbb)\xa4\xa4\x12`#LY\x80\xfaf\xdd\x96m\xfdC\x93\xdf\xcc\xbaMg\x07\xe1\xc4\x93#\xbb\xfd\xe1\xad\x0f\xe2\xe6>\xe5\xa1\xff\x13%\xec\xe5\xe6\x93w\xd9\xe0\x02@\x09Y\xd9\xeb{\x06\x93\xdf\xc0\xb1\x87\xb8Ko\xb2\x83\xf3\xd4\xc63\xbbP\xdc+\xb9\xcb\xb3:\x9c#\xe6$P\xfaA\x81\xe9\x04\xb3D\xc4*Z\xdb\xd8\xfb3\x8eW\x93\xeaD\xa7\x14\x0d\x1f4}\xbd\xfbQs\xac\xdd\xd1>\xe9B\xf0<,eh\xb6\xd3V5\xef\x91Ft\x8c\x14\xb7\xc5\xccK\xd1\x04/\x9a\xf0w\x9a\xcc0\xaf\xaf]\x94\x14\x8an0&2y\x15@\xbb\x0d\xfb\x01\xed\xa9\xa9\xf4\x80\xe4\x01\x19 q\xd5\xdbD5_\xdb\xc7\x8c\x01\xf2\xa6L>\xab \xbc6= \x11M\xb9\x14\x22\xdc\xf4\x19T\xf8\x12\xb2\x09\x07\xa9j\xfb\xbb\xeb\x92\xe7k\xc6ZKM\xb4\xfd\x95\xd4q\xbeL\x80\x89\xc3B\xe7*e\x88\xa4\xf5\xc4\xb5\x96\xdc\xed\xac\x93\x1d\x22a\xadpKN8\x05M\xa2\xc2\x96I6\x1e%\xd8\x1d\x15zY\xe2\x1doi\xf7\x88\x02\xdeM\xac\x93\x01Z\xa4#&\xcf\xf6\xe9b\xb0O\xcf*m\x93\x02|.\xeaHA\x98\xe7\xb5\xb8\xa54`{\x94>i\xcc]<B\xbdK\xe2\x98.\x16\x82\x17\x91\xda\x9c\x07\xe2=\xd8@VB\x16\xfb\x92 \x84r\xc8\xd2t5\x00\xbd8B'\x09\xa2\xe1\x04\xf7\x10\x16\xd13\xe7\xc7\xc6\xdc\xca\xca\xf9\x133\xee.\x8a\xa0\x15Z?LAld\x92K`\x8fpf\xc7j\xeb\x0d\x9a\x06\x1b\xd6P\xba\xb4RQ_\xc33\x14\x11\x8e\xfe\x93q\xc3\x88\xf6\xe6\xf9s\xd2\xd3\xfak\x01\xb3\x93\xc2\x8f\x15\x0e\xa8k\x0e\x0dU(\xc8\xe3\x0e\xd7\x12\x97nRB\xf2\xb3U\x9a\xc5\xf7`\x0b\xc2j\x9a;\xad\x22lC\x1d*\x99\x95\xf6Y\xfc\x90\xd2+\x10\xee\x06\x9aoN\xf4\x9f\x0a\x1e\xdcm\xc5\x06\xc2MU\xfa\x9d2\xad\x01f\xfc<\xff\xe5\x03Rj\xaet\xb3'ye\x16\xc0\xf0 \x9c\x13\xb1\xc8s\xff[R\x19Y\x0f\x10\xe9\xdcw?\xf8\x1c)\x15,A3&>\xf9\xc3W\x80]\xa0j;\x00\xd4\xe1\x10o>r3\xcd/\x17@\x93\xce\xc9\x96S\xb3Ai\xa5\x99\xe2\xbf\x00`\xa1\xb3\xca\xb5\xe1n\xc1\xe6r\x16\xad\xdeP\x90\x03\x13\x1drU\xd7~\xca\x11\xa0nv1\xfd\xeb_\xb3\xda3\x82!4L,Ve\xde\xd1csQL\xc8\x05\x88v\xb3i\xc5C\x08\xfe\xec\xf4\xfbto\xa4\xbbdB#:\xb5\x84\x90z\xe8\x07>\xdc\xc79_\xec\xe0\x1bz\x02\xec\xf2\xff\xb5\xdb\x1e\x97\xfa\xb1k`_\x04\x0b_\x97C\x19\x8a\xc8:U\x10\x1bl\xe2\xe8\x1b\xed!\x95\x92\xadD\xc7\xaa\x16z\xd66\xfc(\xcd==\x07\xa9\x1f\x9cd\xed\x99\xc6\x1bN\xf6B<\xa6YUY\xcf\x01\xf7\x17\xbe_O\x0c\xaeF\xa9\x92,\xb3\x0b\xe7\xc92\xe9(\xcf\xad \xe5%\xb3\xd3\xaf\xcf\xb8R\x0f)_\x87{$\xe7'p\x16\x80?v\xb5\xe3\xc5\xd2\xb8\xfa@\xbd\xdb\xe3\xa7\x12'\xbe\x12\xee\x0c>8\x12\xd8\x96\xff\x14\xd1!M\x01\xdc\x9f\xd1_\xda\x18\xb8\xa9\x81W\x9e3\xa0\x0c\x89\xa5\x0e\xf5\xca\x0f\x82\xe1a\xc2\x14\x19\x07\xad\xbf\xd0\x81\xb42O\x0b9('b8\xf2\xaa]6w:\xebU\xd0\x9arw\xed\xe3\xd93\xf1-\x05\x22\x22\xd9kd\x99M\x1e\xadRpx\xd2\xdf\xa3Y\x1e\x8d\xb1\xaa\x1b \xf2\x19F\xd0\x043\x8d\x80\x13\xb7\xef\xf1\xbd\x1d\x18X\xd5\xea\x89E\xb8\xe1v6jB\x11(KAz\xb5\x9a0 \x07#\xe1\xfe\x02w\xa1\x0e\x98t\xf2\xaa\x9f\xb4\xd1\xf9/\xba6\x8drS-\x9e\xd99\xfb\xb8\xc8q\xaa\x10\x97\x06z\xc3~\xe8\xa7\xcc\x044k\x9a\x129\xe0A\xd6\xdd\xad+\xfd\xc2\x9b\xcfk\xc0\x02\x97\xa9.\xf1\x9a\xb3\xf6\xaecx\x02\xc1+87\x0cs\x8c\xdaI\x91\xf5J\xbf\x9b\xfa\xe5T\xb8\xc0\xdd\x10\xa3\x8b\x0c\x5c\x11TB\xa5\x19Q5\xd8t\xfa\xf4\x0fD\x9c\xf8\xcc\x92B\xcc\x15\x00\xfcB\xfe\x02f\x9fU\x0d=\x00\x00\x00\xff\xff\xfb<:\xff\x14\xda6\x89\x00\x00\x00\x00IEND\xaeB`\x82"))
	// png fuzz seed 190 case 2549 (2026-11-15), a 30x6 paletted image whose
	// IDAT inflates one byte past the image, then an empty stored block,
	// then data the bad checksum cuts: the empty block's flush hands the
	// byte over with no error, so Go reports too much pixel data
	cases = append(cases, []byte("\x89PNG\x0d\x0a\x1a\x0a\x00\x00\x00\x0dIHDR\x00\x00\x00\x1e\x00\x00\x00\x06\x08\x03\x00\x00\x00\xe3k\xdef\x00\x00\x009PLTE\xa5Pso0\xc8f\x90\xfbsT\xe8\x8a\xe9\xc2\x0f\xcf\xa5ZC\xd1\x92Ui\x852\x7f\xfd\xc9\x1c\xf06B\x0d\xfb\xd0\x0c\x9at\xe4\x11[\xa2N\xf1\x82\xc7\x0bA\xbe\xd7\xc8u\x19\x9a\x04\x98?\xdd\xec)\x00\x00\x00\x13tRNS\x88f\xb9*-\x14u\xbfW\xeb\xf3\xffC\x81d\xd5\xd7\xfd(c'0\x11\x00\x00\x00\x7fIDATx\x9c\x04\xc0\x01\x02D!\x04\x04Pil_\x14\xcd\xfd\x0f\xbbON^\xfa\xfd\x0d\xc6<\xcf\xad\xear\xfb\xd7O\xdf@R\xe2~kr\xd3N\xa0o\xbbv\xf3\x04\x02\xce~!\x03\xcfDZ\x97\xaa\xb5F\xc3\xf7\xa9\xe8\x84\x96\xed)\xf3\xe7\x01`\xcf\x04\xdex\xb2/\xd7\x89&\xe68\xf9I\xe9\x023\x8ceYf\xacU\xd7\xb3*\xc7\xcfU\xa5\xef\x1e\x0c\x19\xdcM,\xe0|\x1d,\xb7\xa4^*\xff\x00\x00\x00\xff\xff\x95\xad\x06\xdeC\xcd\xdf*\x00\x00\x00\x00IEND\xaeB`\x82"))

	o.WriteString("/// (PNG bytes, Go's Decode result: `ok MODEL WxH pix=CRC32/LEN pal=N/CRC32` or `err KIND [TEXT]`, Go's DecodeConfig result)\nconst DECODE: List[(Bytes, Str, Str)] = [\n")
	for _, c := range cases {
		if pngDecodeTooLarge(c) {
			continue
		}
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xbytes(c), xq(pngDescribe(c)), xq(pngDescribeConfig(c)))
	}
	o.WriteString("]\n\n")
	// headers too large for an Int to hold the image, decoded with no
	// max_pixels limit: Go refuses the pixel counts whose 8 bytes a pixel
	// overflow; Xo also refuses what overflows its row sums, where Go reads
	// on (and stops at the missing IDAT here)
	o.WriteString("/// (header only PNG, Go's Decode result, Go's DecodeConfig result, whether Xo's decode refuses it as dimension overflow)\nconst HUGE: List[(Bytes, Str, Str, Bool)] = [\n")
	for _, c := range []struct {
		w, h          uint32
		depth, ct, il byte
		over          bool
	}{
		{2147483647, 2147483647, 16, 6, 0, true},
		{2147483647, 2147483647, 8, 0, 0, true},
		{2147483647, 2147483647, 7, 9, 0, true},
		{1073741825, 1073741823, 16, 6, 0, true},
		{1073741825, 1073741823, 16, 6, 1, true},
		{1073741825, 1073741823, 8, 2, 0, false},
		{1073741825, 1073741823, 8, 0, 0, false},
		{1073741825, 1073741823, 1, 0, 1, false},
		{2147483647, 536870911, 8, 0, 0, false},
	} {
		d := pngHeaderOnly(c.w, c.h, c.depth, c.ct, c.il)
		fmt.Fprintf(&o, "  (%s, %s, %s, %v),\n", xbytes(d), xq(pngDescribe(d)), xq(pngDescribeConfig(d)), c.over)
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

fn describe_huge(data: Bytes) -> Str {
  match png.decode(data, 9223372036854775807) {
    Ok(_) => "ok"
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

test "a header too large for an Int is refused, not overflowed" {
  var bad: List[(Int, Str, Str)] = []
  for (i, (data, want, want_config, over)) in HUGE.indexed() {
    let got = describe_huge(data)
    let w = if over { "err unsupported png: unsupported feature: dimension overflow" } else { want }
    if got != w {
      bad.push((i, got, w))
    }
    let gc = describe_config(data)
    if gc != want_config {
      bad.push((i, gc, want_config))
    }
  }
  expect(bad) == []
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
