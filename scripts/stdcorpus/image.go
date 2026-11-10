package main

import (
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"math/rand"
	"strings"
)

// imageCorpus is std/image's color conversions against Go's image/color:
// Gray, Gray16 and NRGBA models over premultiplied 16 bit colors (edges and
// random), RGBToYCbCr and YCbCrToRGB, RGBToCMYK and CMYKToRGB over 8 bit
// triples (the clamps of the YCbCr inverse included), and NRGBA.RGBA().
func imageCorpus() string {
	r := rand.New(rand.NewSource(7))
	edge := []int{0, 1, 127, 128, 255, 256, 257, 32767, 32768, 65534, 65535}
	var o strings.Builder
	o.WriteString(header("image/color: GrayModel, Gray16Model, NRGBAModel, RGBToYCbCr, YCbCrToRGB, RGBToCMYK, CMYKToRGB and NRGBA.RGBA() (std/image, decision 0578)"))
	o.WriteString("use std/image\n\n")

	o.WriteString("/// (r, g, b, a, gray, gray16, nrgba) for a premultiplied color.\nconst COLORS: List[(Int, Int, Int, Int, Int, Int, Int)] = [\n")
	emitColor := func(cr, cg, cb, ca int) {
		c := color.RGBA64{R: uint16(cr), G: uint16(cg), B: uint16(cb), A: uint16(ca)}
		g := color.GrayModel.Convert(c).(color.Gray).Y
		g16 := color.Gray16Model.Convert(c).(color.Gray16).Y
		n := color.NRGBAModel.Convert(c).(color.NRGBA)
		v := int(n.R)<<24 | int(n.G)<<16 | int(n.B)<<8 | int(n.A)
		fmt.Fprintf(&o, "  (%d, %d, %d, %d, %d, %d, %d),\n", cr, cg, cb, ca, g, g16, v)
	}
	for _, a := range edge {
		for _, x := range edge {
			if x <= a {
				emitColor(x, a/2, x/2, a)
				emitColor(a, a, a, a)
			}
		}
	}
	for i := 0; i < 600; i++ {
		a := r.Intn(65536)
		if i%7 == 0 {
			a = 65535
		}
		emitColor(r.Intn(a+1), r.Intn(a+1), r.Intn(a+1), a)
	}
	o.WriteString("]\n\n")

	o.WriteString("/// (r, g, b, y, cb, cr): RGBToYCbCr.\nconst TO_YCC: List[(Int, Int, Int, Int, Int, Int)] = [\n")
	for i := 0; i < 500; i++ {
		cr, cg, cb := r.Intn(256), r.Intn(256), r.Intn(256)
		if i < 8 {
			cr, cg, cb = (i&1)*255, (i>>1&1)*255, (i>>2&1)*255
		}
		y, u, v := color.RGBToYCbCr(uint8(cr), uint8(cg), uint8(cb))
		fmt.Fprintf(&o, "  (%d, %d, %d, %d, %d, %d),\n", cr, cg, cb, y, u, v)
	}
	o.WriteString("]\n\n")

	o.WriteString("/// (y, cb, cr, r, g, b): YCbCrToRGB, clamps included.\nconst FROM_YCC: List[(Int, Int, Int, Int, Int, Int)] = [\n")
	for i := 0; i < 600; i++ {
		y, u, v := r.Intn(256), r.Intn(256), r.Intn(256)
		if i < 8 {
			y, u, v = (i&1)*255, (i>>1&1)*255, (i>>2&1)*255
		}
		cr, cg, cb := color.YCbCrToRGB(uint8(y), uint8(u), uint8(v))
		fmt.Fprintf(&o, "  (%d, %d, %d, %d, %d, %d),\n", y, u, v, cr, cg, cb)
	}
	o.WriteString("]\n\n")

	o.WriteString("/// (r, g, b, c, m, y, k): RGBToCMYK.\nconst TO_CMYK: List[(Int, Int, Int, Int, Int, Int, Int)] = [\n")
	for i := 0; i < 500; i++ {
		cr, cg, cb := r.Intn(256), r.Intn(256), r.Intn(256)
		if i < 8 {
			cr, cg, cb = (i&1)*255, (i>>1&1)*255, (i>>2&1)*255
		}
		c, m, y, k := color.RGBToCMYK(uint8(cr), uint8(cg), uint8(cb))
		fmt.Fprintf(&o, "  (%d, %d, %d, %d, %d, %d, %d),\n", cr, cg, cb, c, m, y, k)
	}
	o.WriteString("]\n\n")

	o.WriteString("/// (c, m, y, k, r, g, b): CMYKToRGB.\nconst FROM_CMYK: List[(Int, Int, Int, Int, Int, Int, Int)] = [\n")
	for i := 0; i < 600; i++ {
		c, m, y, k := r.Intn(256), r.Intn(256), r.Intn(256), r.Intn(256)
		if i < 16 {
			c, m, y, k = (i&1)*255, (i>>1&1)*255, (i>>2&1)*255, (i>>3&1)*255
		}
		cr, cg, cb := color.CMYKToRGB(uint8(c), uint8(m), uint8(y), uint8(k))
		fmt.Fprintf(&o, "  (%d, %d, %d, %d, %d, %d, %d),\n", c, m, y, k, cr, cg, cb)
	}
	o.WriteString("]\n\n")

	o.WriteString("/// (0xRRGGBBAA, r, g, b, a): NRGBA.RGBA().\nconst FROM_NRGBA: List[(Int, Int, Int, Int, Int)] = [\n")
	for i := 0; i < 600; i++ {
		v := r.Uint32()
		if i%5 == 0 {
			v |= 0xff
		}
		if i%11 == 0 {
			v &^= 0xff
		}
		n := color.NRGBA{R: uint8(v >> 24), G: uint8(v >> 16), B: uint8(v >> 8), A: uint8(v)}
		cr, cg, cb, ca := n.RGBA()
		fmt.Fprintf(&o, "  (%d, %d, %d, %d, %d),\n", v, cr, cg, cb, ca)
	}
	o.WriteString("]\n")

	o.WriteString(`
test "Color.gray, gray16 and nrgba are Go's models" {
  for (r, g, b, a, gray, gray16, nrgba) in COLORS {
    let c = image.Color{r: r, g: g, b: b, a: a}
    expect((c.gray(), c.gray16(), c.nrgba())) == (gray, gray16, nrgba)
  }
}

test "rgb_to_ycbcr and ycbcr_to_rgb are Go's" {
  for (r, g, b, y, cb, cr) in TO_YCC {
    expect(image.rgb_to_ycbcr(r, g, b)) == (y, cb, cr)
  }
  for (y, cb, cr, r, g, b) in FROM_YCC {
    expect(image.ycbcr_to_rgb(y, cb, cr)) == (r, g, b)
  }
}

test "rgb_to_cmyk and cmyk_to_rgb are Go's" {
  for (r, g, b, c, m, y, k) in TO_CMYK {
    expect(image.rgb_to_cmyk(r, g, b)) == (c, m, y, k)
  }
  for (c, m, y, k, r, g, b) in FROM_CMYK {
    expect(image.cmyk_to_rgb(c, m, y, k)) == (r, g, b)
  }
}

test "from_nrgba is Go's NRGBA.RGBA()" {
  for (v, r, g, b, a) in FROM_NRGBA {
    expect(image.from_nrgba(v)) == image.Color{r: r, g: g, b: b, a: a}
  }
}
`)
	// Image.At: each model's image of 3 by 2 pixels with random samples
	// (RGBA and RGBA64 kept valid: channels at most alpha), probed inside and
	// one pixel outside on every side.
	bytesLit := func(b []byte) string {
		var sb strings.Builder
		sb.WriteString("b\"")
		for _, c := range b {
			fmt.Fprintf(&sb, "\\x%02x", c)
		}
		sb.WriteString("\"")
		return sb.String()
	}
	o.WriteString("\n/// (model, pix, palette, [(x, y, r, g, b, a)]): Image.At.\nconst AT_CASES: List[(Str, Bytes, List[Int], List[(Int, Int, Int, Int, Int, Int)])] = [\n")
	rect := image.Rect(0, 0, 3, 2)
	emitAt := func(name string, img image.Image, pix []byte, pal []int) {
		fmt.Fprintf(&o, "  (%q, %s, [", name, bytesLit(pix))
		for i, v := range pal {
			if i > 0 {
				o.WriteString(", ")
			}
			fmt.Fprintf(&o, "%d", v)
		}
		o.WriteString("], [")
		first := true
		for y := -1; y <= 2; y++ {
			for x := -1; x <= 3; x++ {
				cr, cg, cb, ca := img.At(x, y).RGBA()
				if !first {
					o.WriteString(", ")
				}
				first = false
				fmt.Fprintf(&o, "(%d, %d, %d, %d, %d, %d)", x, y, cr, cg, cb, ca)
			}
		}
		o.WriteString("]),\n")
	}
	fill := func(n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(r.Intn(256))
		}
		return b
	}
	for rep := 0; rep < 6; rep++ {
		g := image.NewGray(rect)
		copy(g.Pix, fill(len(g.Pix)))
		emitAt("Gray", g, g.Pix, nil)
		g16 := image.NewGray16(rect)
		copy(g16.Pix, fill(len(g16.Pix)))
		emitAt("Gray16", g16, g16.Pix, nil)
		n := image.NewNRGBA(rect)
		copy(n.Pix, fill(len(n.Pix)))
		emitAt("NRGBA", n, n.Pix, nil)
		n64 := image.NewNRGBA64(rect)
		copy(n64.Pix, fill(len(n64.Pix)))
		emitAt("NRGBA64", n64, n64.Pix, nil)
		c := image.NewCMYK(rect)
		copy(c.Pix, fill(len(c.Pix)))
		emitAt("CMYK", c, c.Pix, nil)
		rg := image.NewRGBA(rect)
		for i := 0; i < len(rg.Pix); i += 4 {
			a := r.Intn(256)
			rg.Pix[i], rg.Pix[i+1], rg.Pix[i+2], rg.Pix[i+3] = byte(r.Intn(a+1)), byte(r.Intn(a+1)), byte(r.Intn(a+1)), byte(a)
		}
		emitAt("RGBA", rg, rg.Pix, nil)
		r64 := image.NewRGBA64(rect)
		for i := 0; i < len(r64.Pix); i += 8 {
			a := r.Intn(65536)
			vals := [4]int{r.Intn(a + 1), r.Intn(a + 1), r.Intn(a + 1), a}
			for k, v := range vals {
				r64.Pix[i+2*k], r64.Pix[i+2*k+1] = byte(v>>8), byte(v)
			}
		}
		emitAt("RGBA64", r64, r64.Pix, nil)
		pal := color.Palette{}
		var palv []int
		for k := 0; k < 4; k++ {
			nc := color.NRGBA{uint8(r.Intn(256)), uint8(r.Intn(256)), uint8(r.Intn(256)), uint8(r.Intn(256))}
			pal = append(pal, nc)
			palv = append(palv, int(nc.R)<<24|int(nc.G)<<16|int(nc.B)<<8|int(nc.A))
		}
		pi := image.NewPaletted(rect, pal)
		for i := range pi.Pix {
			pi.Pix[i] = byte(r.Intn(4))
		}
		emitAt("Paletted", pi, pi.Pix, palv)
	}
	o.WriteString("]\n")

	o.WriteString(`
test "Image.at is Go's At" {
  for (name, pix, pal, probes) in AT_CASES {
    let model = match name {
      "Gray" => image.Model.Gray
      "Gray16" => image.Model.Gray16
      "NRGBA" => image.Model.NRGBA
      "NRGBA64" => image.Model.NRGBA64
      "CMYK" => image.Model.CMYK
      "RGBA" => image.Model.RGBA
      "RGBA64" => image.Model.RGBA64
      _ => image.Model.Paletted
    }
    let im = image.Image{width: 3, height: 2, model: model, pix: pix, palette: pal}
    for (x, y, r, g, b, a) in probes {
      expect(im.at(x, y)) == image.Color{r: r, g: g, b: b, a: a}
    }
  }
}
`)
	o.WriteString("\n/// palette.WebSafe as 0xRRGGBBAA.\nconst WEB_SAFE: List[Int] = [")
	for i, c := range palette.WebSafe {
		n := c.(color.RGBA)
		if i > 0 {
			o.WriteString(", ")
		}
		fmt.Fprintf(&o, "%d", int(n.R)<<24|int(n.G)<<16|int(n.B)<<8|int(n.A))
	}
	o.WriteString("]\n\ntest \"web_safe is Go's palette.WebSafe\" {\n  expect(image.web_safe()) == WEB_SAFE\n}\n")
	o.WriteString("\n/// palette.Plan9 as 0xRRGGBBAA.\nconst PLAN9: List[Int] = [")
	for i, c := range palette.Plan9 {
		n := c.(color.RGBA)
		if i > 0 {
			o.WriteString(", ")
		}
		fmt.Fprintf(&o, "%d", int(n.R)<<24|int(n.G)<<16|int(n.B)<<8|int(n.A))
	}
	o.WriteString("]\n\ntest \"plan9 is Go's palette.Plan9\" {\n  expect(image.plan9()) == PLAN9\n}\n")
	// Image.Set: a starting image of each model, three random premultiplied
	// colors set at chosen points (one outside the image), the pix after.
	type setImg interface {
		image.Image
		Set(x, y int, c color.Color)
	}
	o.WriteString("\n/// (model, pix before, palette, [(x, y, r, g, b, a)], pix after): Image.Set.\nconst SET_CASES: List[(Str, Bytes, List[Int], List[(Int, Int, Int, Int, Int, Int)], Bytes)] = [\n")
	pixOf := func(img image.Image) []byte {
		switch v := img.(type) {
		case *image.Gray:
			return v.Pix
		case *image.Gray16:
			return v.Pix
		case *image.NRGBA:
			return v.Pix
		case *image.NRGBA64:
			return v.Pix
		case *image.CMYK:
			return v.Pix
		case *image.RGBA:
			return v.Pix
		case *image.RGBA64:
			return v.Pix
		case *image.Paletted:
			return v.Pix
		}
		return nil
	}
	for rep := 0; rep < 8; rep++ {
		var pal color.Palette
		var palv []int
		for k := 0; k < 6; k++ {
			nc := color.NRGBA{uint8(r.Intn(256)), uint8(r.Intn(256)), uint8(r.Intn(256)), uint8(r.Intn(256))}
			pal = append(pal, nc)
			palv = append(palv, int(nc.R)<<24|int(nc.G)<<16|int(nc.B)<<8|int(nc.A))
		}
		imgs := map[string]setImg{
			"Gray": image.NewGray(rect), "Gray16": image.NewGray16(rect), "NRGBA": image.NewNRGBA(rect),
			"NRGBA64": image.NewNRGBA64(rect), "CMYK": image.NewCMYK(rect), "RGBA": image.NewRGBA(rect),
			"RGBA64": image.NewRGBA64(rect), "Paletted": image.NewPaletted(rect, pal),
		}
		for _, name := range []string{"Gray", "Gray16", "NRGBA", "NRGBA64", "CMYK", "RGBA", "RGBA64", "Paletted"} {
			img := imgs[name]
			px := pixOf(img)
			if name == "Paletted" {
				for i := range px {
					px[i] = byte(r.Intn(6))
				}
			} else {
				copy(px, fill(len(px)))
			}
			before := append([]byte(nil), px...)
			var probes strings.Builder
			for k := 0; k < 4; k++ {
				x, y := r.Intn(5)-1, r.Intn(4)-1
				a := r.Intn(65536)
				if k == 0 {
					a = 65535
				}
				if k == 1 && rep%2 == 0 {
					a = 0
				}
				cr, cg, cb := r.Intn(a+1), r.Intn(a+1), r.Intn(a+1)
				img.Set(x, y, color.RGBA64{R: uint16(cr), G: uint16(cg), B: uint16(cb), A: uint16(a)})
				if k > 0 {
					probes.WriteString(", ")
				}
				fmt.Fprintf(&probes, "(%d, %d, %d, %d, %d, %d)", x, y, cr, cg, cb, a)
			}
			palText := "[]"
			if name == "Paletted" {
				var sb strings.Builder
				sb.WriteString("[")
				for i, v := range palv {
					if i > 0 {
						sb.WriteString(", ")
					}
					fmt.Fprintf(&sb, "%d", v)
				}
				sb.WriteString("]")
				palText = sb.String()
			}
			fmt.Fprintf(&o, "  (%q, %s, %s, [%s], %s),\n", name, bytesLit(before), palText, probes.String(), bytesLit(pixOf(img)))
		}
	}
	o.WriteString("]\n")
	o.WriteString(`
test "Image.set is Go's Set" {
  for (name, before, pal, sets, after) in SET_CASES {
    let model = match name {
      "Gray" => image.Model.Gray
      "Gray16" => image.Model.Gray16
      "NRGBA" => image.Model.NRGBA
      "NRGBA64" => image.Model.NRGBA64
      "CMYK" => image.Model.CMYK
      "RGBA" => image.Model.RGBA
      "RGBA64" => image.Model.RGBA64
      _ => image.Model.Paletted
    }
    var im = image.Image{width: 3, height: 2, model: model, pix: before, palette: pal}
    for (x, y, r, g, b, a) in sets {
      im.set(x, y, image.Color{r: r, g: g, b: b, a: a})
    }
    expect(im.pix) == after
  }
}
`)
	// Image.SubImage and crop: a 5 by 4 image of each model, three rectangles
	// (partly or wholly outside), the sub image's size, corner and pix, and
	// its At over the rectangle and a pixel around it.
	o.WriteString("\n/// (model, pix, palette, rect, (w, h, min_x, min_y, stride), pix of the sub image, [(x, y, r, g, b, a)]): SubImage.\nconst SUB_CASES: List[(Str, Bytes, List[Int], (Int, Int, Int, Int), (Int, Int, Int, Int, Int), Bytes, List[(Int, Int, Int, Int, Int, Int)])] = [\n")
	big := image.Rect(0, 0, 5, 4)
	type subImg interface {
		image.Image
		SubImage(r image.Rectangle) image.Image
	}
	subPix := func(img image.Image) ([]byte, int) {
		switch v := img.(type) {
		case *image.Gray:
			return v.Pix, v.Stride
		case *image.Gray16:
			return v.Pix, v.Stride
		case *image.NRGBA:
			return v.Pix, v.Stride
		case *image.NRGBA64:
			return v.Pix, v.Stride
		case *image.CMYK:
			return v.Pix, v.Stride
		case *image.RGBA:
			return v.Pix, v.Stride
		case *image.RGBA64:
			return v.Pix, v.Stride
		case *image.Paletted:
			return v.Pix, v.Stride
		}
		return nil, 0
	}
	for rep := 0; rep < 5; rep++ {
		var pal color.Palette
		var palv []int
		for k := 0; k < 4; k++ {
			nc := color.NRGBA{uint8(r.Intn(256)), uint8(r.Intn(256)), uint8(r.Intn(256)), uint8(r.Intn(256))}
			pal = append(pal, nc)
			palv = append(palv, int(nc.R)<<24|int(nc.G)<<16|int(nc.B)<<8|int(nc.A))
		}
		imgs := map[string]subImg{
			"Gray": image.NewGray(big), "Gray16": image.NewGray16(big), "NRGBA": image.NewNRGBA(big),
			"NRGBA64": image.NewNRGBA64(big), "CMYK": image.NewCMYK(big), "RGBA": image.NewRGBA(big),
			"RGBA64": image.NewRGBA64(big), "Paletted": image.NewPaletted(big, pal),
		}
		for _, name := range []string{"Gray", "Gray16", "NRGBA", "NRGBA64", "CMYK", "RGBA", "RGBA64", "Paletted"} {
			img := imgs[name]
			px, _ := subPix(img)
			if name == "Paletted" {
				for i := range px {
					px[i] = byte(r.Intn(4))
				}
			} else if name == "RGBA" {
				for i := 0; i < len(px); i += 4 {
					a := r.Intn(256)
					px[i], px[i+1], px[i+2], px[i+3] = byte(r.Intn(a+1)), byte(r.Intn(a+1)), byte(r.Intn(a+1)), byte(a)
				}
			} else if name == "RGBA64" {
				for i := 0; i < len(px); i += 8 {
					a := r.Intn(65536)
					vals := [4]int{r.Intn(a + 1), r.Intn(a + 1), r.Intn(a + 1), a}
					for k, v := range vals {
						px[i+2*k], px[i+2*k+1] = byte(v>>8), byte(v)
					}
				}
			} else {
				copy(px, fill(len(px)))
			}
			rects := []image.Rectangle{image.Rect(1, 1, 4, 3), image.Rect(-2, 2, 3, 9), image.Rect(7, 7, 9, 9)}
			for _, rc := range rects {
				sub := img.SubImage(rc)
				spx, stride := subPix(sub)
				b := sub.Bounds()
				palText := "[]"
				if name == "Paletted" {
					var sb strings.Builder
					sb.WriteString("[")
					for i, v := range palv {
						if i > 0 {
							sb.WriteString(", ")
						}
						fmt.Fprintf(&sb, "%d", v)
					}
					sb.WriteString("]")
					palText = sb.String()
				}
				var probes strings.Builder
				first := true
				for y := b.Min.Y - 1; y <= b.Max.Y; y++ {
					for x := b.Min.X - 1; x <= b.Max.X; x++ {
						cr, cg, cb, ca := sub.At(x, y).RGBA()
						if !first {
							probes.WriteString(", ")
						}
						first = false
						fmt.Fprintf(&probes, "(%d, %d, %d, %d, %d, %d)", x, y, cr, cg, cb, ca)
					}
				}
				fmt.Fprintf(&o, "  (%q, %s, %s, (%d, %d, %d, %d), (%d, %d, %d, %d, %d), %s, [%s]),\n", name, bytesLit(px), palText, rc.Min.X, rc.Min.Y, rc.Max.X, rc.Max.Y,
					b.Dx(), b.Dy(), b.Min.X, b.Min.Y, stride, bytesLit(spx), probes.String())
			}
		}
	}
	o.WriteString("]\n")
	o.WriteString(`
test "Image.sub_image is Go's SubImage" {
  for (name, pix, pal, rc, dims, spix, probes) in SUB_CASES {
    let model = match name {
      "Gray" => image.Model.Gray
      "Gray16" => image.Model.Gray16
      "NRGBA" => image.Model.NRGBA
      "NRGBA64" => image.Model.NRGBA64
      "CMYK" => image.Model.CMYK
      "RGBA" => image.Model.RGBA
      "RGBA64" => image.Model.RGBA64
      _ => image.Model.Paletted
    }
    let im = image.Image{width: 5, height: 4, model: model, pix: pix, palette: pal}
    let (x0, y0, x1, y1) = rc
    let sub = im.sub_image(image.Rect{x0: x0, y0: y0, x1: x1, y1: y1})
    let (w, h, mx, my, stride) = dims
    expect((sub.width, sub.height)) == (w, h)
    if w > 0 {
      expect((sub.min_x, sub.min_y, sub.stride)) == (mx, my, stride)
      expect(sub.pix) == spix
    }
    for (x, y, r, g, b, a) in probes {
      if w > 0 {
        expect(sub.at(x, y)) == image.Color{r: r, g: g, b: b, a: a}
      }
    }
    let c = im.crop(image.Rect{x0: x0, y0: y0, x1: x1, y1: y1})
    expect((c.width, c.height)) == (w, h)
    if w > 0 {
      for yy in 0..h {
        for xx in 0..w {
          expect(c.at(mx + xx, my + yy)) == sub.at(mx + xx, my + yy)
        }
      }
    }
  }
}
`)
	// to_rgba and to_nrgba: 3 by 2 images of each model through
	// color.RGBAModel and color.NRGBAModel pixel by pixel.
	o.WriteString("\n/// (model, pix, palette, RGBA pix, NRGBA pix): to_rgba and to_nrgba.\nconst CONV_CASES: List[(Str, Bytes, List[Int], Bytes, Bytes)] = [\n")
	for rep := 0; rep < 5; rep++ {
		var cpal color.Palette
		var cpalv []int
		for k := 0; k < 5; k++ {
			nc := color.NRGBA{uint8(r.Intn(256)), uint8(r.Intn(256)), uint8(r.Intn(256)), uint8(r.Intn(256))}
			cpal = append(cpal, nc)
			cpalv = append(cpalv, int(nc.R)<<24|int(nc.G)<<16|int(nc.B)<<8|int(nc.A))
		}
		cimgs := map[string]subImg{
			"Gray": image.NewGray(rect), "Gray16": image.NewGray16(rect), "NRGBA": image.NewNRGBA(rect),
			"NRGBA64": image.NewNRGBA64(rect), "CMYK": image.NewCMYK(rect), "RGBA": image.NewRGBA(rect),
			"RGBA64": image.NewRGBA64(rect), "Paletted": image.NewPaletted(rect, cpal),
		}
		for _, name := range []string{"Gray", "Gray16", "NRGBA", "NRGBA64", "CMYK", "RGBA", "RGBA64", "Paletted"} {
			img := cimgs[name]
			px, _ := subPix(img)
			if name == "Paletted" {
				for i := range px {
					px[i] = byte(r.Intn(5))
				}
			} else if name == "RGBA" {
				for i := 0; i < len(px); i += 4 {
					a := r.Intn(256)
					px[i], px[i+1], px[i+2], px[i+3] = byte(r.Intn(a+1)), byte(r.Intn(a+1)), byte(r.Intn(a+1)), byte(a)
				}
			} else if name == "RGBA64" {
				for i := 0; i < len(px); i += 8 {
					a := r.Intn(65536)
					vals := [4]int{r.Intn(a + 1), r.Intn(a + 1), r.Intn(a + 1), a}
					for k, v := range vals {
						px[i+2*k], px[i+2*k+1] = byte(v>>8), byte(v)
					}
				}
			} else {
				copy(px, fill(len(px)))
			}
			var rg, ng []byte
			for y := 0; y < 2; y++ {
				for x := 0; x < 3; x++ {
					c1 := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
					rg = append(rg, c1.R, c1.G, c1.B, c1.A)
					c2 := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
					ng = append(ng, c2.R, c2.G, c2.B, c2.A)
				}
			}
			cpt := "[]"
			if name == "Paletted" {
				var sb strings.Builder
				sb.WriteString("[")
				for i, v := range cpalv {
					if i > 0 {
						sb.WriteString(", ")
					}
					fmt.Fprintf(&sb, "%d", v)
				}
				sb.WriteString("]")
				cpt = sb.String()
			}
			fmt.Fprintf(&o, "  (%q, %s, %s, %s, %s),\n", name, bytesLit(px), cpt, bytesLit(rg), bytesLit(ng))
		}
	}
	o.WriteString("]\n")
	o.WriteString(`
test "Image.to_rgba and to_nrgba are Go's models pixel by pixel" {
  for (name, pix, pal, rgba, nrgba) in CONV_CASES {
    let model = match name {
      "Gray" => image.Model.Gray
      "Gray16" => image.Model.Gray16
      "NRGBA" => image.Model.NRGBA
      "NRGBA64" => image.Model.NRGBA64
      "CMYK" => image.Model.CMYK
      "RGBA" => image.Model.RGBA
      "RGBA64" => image.Model.RGBA64
      _ => image.Model.Paletted
    }
    let im = image.Image{width: 3, height: 2, model: model, pix: pix, palette: pal}
    let a = im.to_rgba()
    expect((a.model, a.width, a.height, a.pix)) == (image.Model.RGBA, 3, 2, rgba)
    let b = im.to_nrgba()
    expect((b.model, b.width, b.height, b.pix)) == (image.Model.NRGBA, 3, 2, nrgba)
  }
}
`)
	return o.String()
}
