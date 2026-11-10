package rt

// std/gif on the Go backend (decision 0578 step 5): Go's image/gif over
// whole buffers. Frames are std/image Paletted images, palettes 0xRRGGBBAA.

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
)

// M_display is the error's text.
func (e Gif_GifErr) M_display(*Task) string { return e.F_msg }

const (
	imgGray = iota + 1
	imgGray16
	imgPaletted
	imgRGBA
	imgNRGBA
	imgRGBA64
	imgNRGBA64
	imgCMYK
	imgYCbCr
)

func gifPaletteInts(p color.Palette) List[int64] {
	out := make([]int64, len(p))
	for i, c := range p {
		n := color.NRGBAModel.Convert(c).(color.NRGBA)
		out[i] = int64(n.R)<<24 | int64(n.G)<<16 | int64(n.B)<<8 | int64(n.A)
	}
	return ListFromSlice(out)
}

// gifPalette is the palette image/gif's decoder builds for the ints: RGBA
// entries (the decoder's own color type), with the entries' alpha kept.
func gifPalette(l List[int64]) color.Palette {
	p := make(color.Palette, 0, len(List_Slice(l)))
	for _, v := range List_Slice(l) {
		p = append(p, color.NRGBA{R: uint8(v >> 24), G: uint8(v >> 16), B: uint8(v >> 8), A: uint8(v)})
	}
	return p
}

func gifFrame(m *image.Paletted) Image_Image {
	b := m.Bounds()
	pix := make([]byte, 0, b.Dx()*b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		i := m.PixOffset(b.Min.X, y)
		pix = append(pix, m.Pix[i:i+b.Dx()]...)
	}
	return Image_Image{
		F_width: int64(b.Dx()), F_height: int64(b.Dy()), F_model: Image_Model{Tag: imgPaletted},
		F_pix: Bytes(pix), F_stride: int64(b.Dx()), F_palette: gifPaletteInts(m.Palette),
		F_min_x: int64(b.Min.X), F_min_y: int64(b.Min.Y),
	}
}

func gifErr(err error) Gif_GifErr { return Gif_GifErr{F_msg: err.Error()} }

func Gif_decode(_ *Task, data Bytes, maxPixels int64) Result[Image_Image, Gif_GifErr] {
	im, err := gif.Decode(bytes.NewReader([]byte(data)))
	if err != nil {
		return Err[Image_Image](gifErr(err))
	}
	f := gifFrame(im.(*image.Paletted))
	if maxPixels >= 0 && f.F_width*f.F_height > maxPixels {
		return Err[Image_Image](Gif_GifErr{F_msg: "image too large"})
	}
	return Ok[Image_Image, Gif_GifErr](f)
}

func Gif_decode_all(_ *Task, data Bytes, maxPixels int64) Result[Gif_Gif, Gif_GifErr] {
	g, err := gif.DecodeAll(bytes.NewReader([]byte(data)))
	if err != nil {
		return Err[Gif_Gif](gifErr(err))
	}
	frames := make([]Image_Image, len(g.Image))
	delay := make([]int64, len(g.Image))
	disposal := make([]int64, len(g.Image))
	for i, m := range g.Image {
		frames[i] = gifFrame(m)
		if maxPixels >= 0 && frames[i].F_width*frames[i].F_height > maxPixels {
			return Err[Gif_Gif](Gif_GifErr{F_msg: "image too large"})
		}
		delay[i] = int64(g.Delay[i])
		disposal[i] = int64(g.Disposal[i])
	}
	var pal List[int64]
	if p, ok := g.Config.ColorModel.(color.Palette); ok {
		pal = gifPaletteInts(p)
	}
	return Ok[Gif_Gif, Gif_GifErr](Gif_Gif{
		F_frames: ListFromSlice(frames), F_delay: ListFromSlice(delay), F_disposal: ListFromSlice(disposal),
		F_loop_count: int64(g.LoopCount), F_width: int64(g.Config.Width), F_height: int64(g.Config.Height),
		F_palette: pal, F_background_index: int64(g.BackgroundIndex),
	})
}

func Gif_decode_config(_ *Task, data Bytes) Result[Gif_Config, Gif_GifErr] {
	c, err := gif.DecodeConfig(bytes.NewReader([]byte(data)))
	if err != nil {
		return Err[Gif_Config](gifErr(err))
	}
	var pal List[int64]
	if p, ok := c.ColorModel.(color.Palette); ok {
		pal = gifPaletteInts(p)
	}
	return Ok[Gif_Config, Gif_GifErr](Gif_Config{F_width: int64(c.Width), F_height: int64(c.Height), F_palette: pal})
}

// gifImage is the Go image for a std/image Image (every model).
func gifImage(im Image_Image) image.Image {
	w, h := int(im.F_width), int(im.F_height)
	r := image.Rect(int(im.F_min_x), int(im.F_min_y), int(im.F_min_x)+w, int(im.F_min_y)+h)
	stride := int(im.F_stride)
	pix := []byte(im.F_pix)
	bpp := map[int]int{imgGray: 1, imgGray16: 2, imgPaletted: 1, imgRGBA: 4, imgNRGBA: 4, imgRGBA64: 8, imgNRGBA64: 8, imgCMYK: 4}[im.F_model.Tag]
	if stride == 0 {
		stride = w * bpp
	}
	switch im.F_model.Tag {
	case imgGray:
		return &image.Gray{Pix: pix, Stride: stride, Rect: r}
	case imgGray16:
		return &image.Gray16{Pix: pix, Stride: stride, Rect: r}
	case imgPaletted:
		return &image.Paletted{Pix: pix, Stride: stride, Rect: r, Palette: gifPalette(im.F_palette)}
	case imgRGBA:
		return &image.RGBA{Pix: pix, Stride: stride, Rect: r}
	case imgNRGBA:
		return &image.NRGBA{Pix: pix, Stride: stride, Rect: r}
	case imgRGBA64:
		return &image.RGBA64{Pix: pix, Stride: stride, Rect: r}
	case imgNRGBA64:
		return &image.NRGBA64{Pix: pix, Stride: stride, Rect: r}
	case imgCMYK:
		return &image.CMYK{Pix: pix, Stride: stride, Rect: r}
	}
	ratios := []image.YCbCrSubsampleRatio{image.YCbCrSubsampleRatio444, image.YCbCrSubsampleRatio440, image.YCbCrSubsampleRatio422, image.YCbCrSubsampleRatio420, image.YCbCrSubsampleRatio411, image.YCbCrSubsampleRatio410}
	ys, cs := int(im.F_y_stride), int(im.F_c_stride)
	if ys == 0 {
		ys = w
	}
	return &image.YCbCr{Y: []byte(im.F_y), Cb: []byte(im.F_cb), Cr: []byte(im.F_cr), YStride: ys, CStride: cs, SubsampleRatio: ratios[im.F_subsample.Tag-1], Rect: r}
}

func Gif_encode(_ *Task, im Image_Image, numColors int64) Result[Bytes, Gif_GifErr] {
	var b bytes.Buffer
	if err := gif.Encode(&b, gifImage(im), &gif.Options{NumColors: int(numColors)}); err != nil {
		return Err[Bytes](gifErr(err))
	}
	return Ok[Bytes, Gif_GifErr](Bytes(b.Bytes()))
}

func Gif_encode_all(_ *Task, g Gif_Gif) Result[Bytes, Gif_GifErr] {
	out := &gif.GIF{LoopCount: int(g.F_loop_count), BackgroundIndex: uint8(g.F_background_index)}
	for _, f := range List_Slice(g.F_frames) {
		pm, ok := gifImage(f).(*image.Paletted)
		if !ok {
			return Err[Bytes](Gif_GifErr{F_msg: "gif: frames must be Paletted images"})
		}
		out.Image = append(out.Image, pm)
	}
	for _, d := range List_Slice(g.F_delay) {
		out.Delay = append(out.Delay, int(d))
	}
	if len(List_Slice(g.F_disposal)) > 0 {
		for _, d := range List_Slice(g.F_disposal) {
			out.Disposal = append(out.Disposal, byte(d))
		}
	}
	out.Config.Width, out.Config.Height = int(g.F_width), int(g.F_height)
	if len(List_Slice(g.F_palette)) > 0 {
		out.Config.ColorModel = gifPalette(g.F_palette)
	}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, out); err != nil {
		return Err[Bytes](gifErr(err))
	}
	return Ok[Bytes, Gif_GifErr](Bytes(b.Bytes()))
}
