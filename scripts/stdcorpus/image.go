package main

import (
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
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
	o.WriteString("use std/image\nuse std/png\nuse std/jpeg\n\n")

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
	o.WriteString(`
test "from_png and to_png move the pixels and keep the model" {
  let pal = [0xff0000ff, 0x00ff00ff, 0x0000ffff]
  let p = png.Image{width: 2, height: 2, model: png.Model.Paletted, pix: b"\x00\x01\x02\x00", palette: pal}
  let im = image.from_png(p)
  expect((im.width, im.height, im.model, im.pix, im.palette)) == (2, 2, image.Model.Paletted, p.pix, pal)
  let back = im.to_png() ?? fault("a Paletted image has a PNG form")
  expect((back.width, back.height, back.model, back.pix, back.palette)) == (2, 2, png.Model.Paletted, p.pix, pal)
}

test "to_png crops a sub image and refuses CMYK and YCbCr" {
  let g = image.Image{width: 3, height: 3, model: image.Model.Gray, pix: b"\x01\x02\x03\x04\x05\x06\x07\x08\x09"}
  let sub = g.sub_image(image.Rect{x0: 1, y0: 1, x1: 3, y1: 3})
  let p = sub.to_png() ?? fault("a Gray sub image has a PNG form")
  expect((p.width, p.height, p.pix)) == (2, 2, b"\x05\x06\x08\x09")
  let c = image.Image{width: 1, height: 1, model: image.Model.CMYK, pix: b"\x00\x00\x00\x00"}
  expect(c.to_png().is_none()) == true
}
`)
	// YCbCr At and SubImage for every subsampling ratio: a 5 by 4 image with
	// random planes, probed whole and through two sub images (one clipped).
	o.WriteString("\n/// (ratio, y, cb, cr, rect, (w, h, min_x, min_y), [(x, y, r, g, b, a)]): YCbCr At and SubImage.\nconst YCC_CASES: List[(Str, Bytes, Bytes, Bytes, (Int, Int, Int, Int), (Int, Int, Int, Int), List[(Int, Int, Int, Int, Int, Int)])] = [\n")
	ratios := []struct {
		name string
		r    image.YCbCrSubsampleRatio
	}{
		{"S444", image.YCbCrSubsampleRatio444}, {"S422", image.YCbCrSubsampleRatio422}, {"S420", image.YCbCrSubsampleRatio420},
		{"S440", image.YCbCrSubsampleRatio440}, {"S411", image.YCbCrSubsampleRatio411}, {"S410", image.YCbCrSubsampleRatio410},
	}
	for rep := 0; rep < 3; rep++ {
		for _, ra := range ratios {
			yc := image.NewYCbCr(big, ra.r)
			copy(yc.Y, fill(len(yc.Y)))
			copy(yc.Cb, fill(len(yc.Cb)))
			copy(yc.Cr, fill(len(yc.Cr)))
			for _, rc := range []image.Rectangle{big, image.Rect(1, 1, 4, 3), image.Rect(-1, 2, 3, 9), image.Rect(3, 1, 5, 4)} {
				var sub image.Image = yc
				if rc != big {
					sub = yc.SubImage(rc)
				}
				b := sub.Bounds()
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
				fmt.Fprintf(&o, "  (%q, %s, %s, %s, (%d, %d, %d, %d), (%d, %d, %d, %d), [%s]),\n", ra.name, bytesLit(yc.Y), bytesLit(yc.Cb), bytesLit(yc.Cr),
					rc.Min.X, rc.Min.Y, rc.Max.X, rc.Max.Y, b.Dx(), b.Dy(), b.Min.X, b.Min.Y, probes.String())
			}
		}
	}
	o.WriteString("]\n")
	o.WriteString(`
test "YCbCr images: at and sub_image are Go's" {
  for (name, yp, cbp, crp, rc, dims, probes) in YCC_CASES {
    let ss = match name {
      "S444" => jpeg.Subsample.S444
      "S422" => jpeg.Subsample.S422
      "S420" => jpeg.Subsample.S420
      "S440" => jpeg.Subsample.S440
      "S411" => jpeg.Subsample.S411
      _ => jpeg.Subsample.S410
    }
    let im = image.Image{width: 5, height: 4, model: image.Model.YCbCr, pix: b"", y: yp, cb: cbp, cr: crp, subsample: ss}
    let (x0, y0, x1, y1) = rc
    let sub = im.sub_image(image.Rect{x0: x0, y0: y0, x1: x1, y1: y1})
    let (w, h, mx, my) = dims
    expect((sub.width, sub.height)) == (w, h)
    if w > 0 {
      expect((sub.min_x, sub.min_y)) == (mx, my)
      for (x, y, r, g, b, a) in probes {
        expect(sub.at(x, y)) == image.Color{r: r, g: g, b: b, a: a}
      }
    }
  }
}
`)
	o.WriteString(`
test "from_jpeg and to_jpeg keep the samples and the chroma subsampling" {
  let j = jpeg.Image{width: 2, height: 2, model: jpeg.Model.YCbCr, subsample: jpeg.Subsample.S420, y: b"\x10\x20\x30\x40", cb: b"\x80", cr: b"\x90"}
  let im = image.from_jpeg(j)
  expect((im.width, im.height, im.model, im.subsample, im.y, im.cb, im.cr)) == (2, 2, image.Model.YCbCr, jpeg.Subsample.S420, j.y, j.cb, j.cr)
  let back = im.to_jpeg() ?? fault("a tight YCbCr image has a JPEG form")
  expect((back.model, back.subsample, back.y, back.cb, back.cr)) == (jpeg.Model.YCbCr, jpeg.Subsample.S420, j.y, j.cb, j.cr)
  let g = jpeg.Image{width: 2, height: 1, model: jpeg.Model.Gray, pix: b"\x07\x09"}
  let gi = image.from_jpeg(g)
  expect(gi.model) == image.Model.Gray
  let gb = gi.to_jpeg() ?? fault("a Gray image has a JPEG form")
  expect((gb.model, gb.pix)) == (jpeg.Model.Gray, g.pix)
  let n = image.Image{width: 1, height: 1, model: image.Model.NRGBA, pix: b"\x00\x00\x00\x00"}
  expect(n.to_jpeg().is_none()) == true
}
`)
	// draw.Draw: every destination model (but Paletted) against every source
	// model, both ops, two rectangles with source offsets and clipping.
	o.WriteString("\n/// The palette of the Paletted destinations of DRAW_CASES.\nconst DRAW_PAL: List[Int] = [10 << 24 | 20 << 16 | 30 << 8 | 255, 200 << 24 | 100 << 16 | 50 << 8 | 128, 0, 255 << 24 | 255 << 16 | 255 << 8 | 255]\n")
	o.WriteString("\n/// (dst model, dst pix, src model, src pix, palette, (y, cb, cr), rect, sp, op, pix after): draw.Draw with a nil mask.\nconst DRAW_CASES: List[(Str, Bytes, Str, Bytes, List[Int], (Bytes, Bytes, Bytes), (Int, Int, Int, Int), (Int, Int), Int, Bytes)] = [\n")
	dpal := color.Palette{color.NRGBA{10, 20, 30, 255}, color.NRGBA{200, 100, 50, 128}, color.NRGBA{0, 0, 0, 0}, color.NRGBA{255, 255, 255, 255}}
	dpalv := []int{10<<24 | 20<<16 | 30<<8 | 255, 200<<24 | 100<<16 | 50<<8 | 128, 0, 255<<24 | 255<<16 | 255<<8 | 255}
	randPix := func(name string, px []byte) {
		switch name {
		case "RGBA":
			for i := 0; i < len(px); i += 4 {
				a := r.Intn(256)
				px[i], px[i+1], px[i+2], px[i+3] = byte(r.Intn(a+1)), byte(r.Intn(a+1)), byte(r.Intn(a+1)), byte(a)
			}
		case "RGBA64":
			for i := 0; i < len(px); i += 8 {
				a := r.Intn(65536)
				vals := [4]int{r.Intn(a + 1), r.Intn(a + 1), r.Intn(a + 1), a}
				for k, v := range vals {
					px[i+2*k], px[i+2*k+1] = byte(v>>8), byte(v)
				}
			}
		case "Paletted":
			for i := range px {
				px[i] = byte(r.Intn(4))
			}
		default:
			copy(px, fill(len(px)))
		}
	}
	mkImg := func(name string, rc image.Rectangle) (draw.Image, []byte) {
		switch name {
		case "Gray":
			v := image.NewGray(rc)
			return v, v.Pix
		case "Gray16":
			v := image.NewGray16(rc)
			return v, v.Pix
		case "NRGBA":
			v := image.NewNRGBA(rc)
			return v, v.Pix
		case "NRGBA64":
			v := image.NewNRGBA64(rc)
			return v, v.Pix
		case "CMYK":
			v := image.NewCMYK(rc)
			return v, v.Pix
		case "RGBA":
			v := image.NewRGBA(rc)
			return v, v.Pix
		case "RGBA64":
			v := image.NewRGBA64(rc)
			return v, v.Pix
		}
		v := image.NewPaletted(rc, dpal)
		return v, v.Pix
	}
	dstNames := []string{"RGBA", "NRGBA", "Gray", "Gray16", "RGBA64", "NRGBA64", "CMYK", "Paletted"}
	srcNames := []string{"RGBA", "NRGBA", "Gray", "Gray16", "RGBA64", "NRGBA64", "CMYK", "Paletted", "YCbCr"}
	type drawRect struct {
		rc image.Rectangle
		sp image.Point
	}
	for _, dn := range dstNames {
		for _, sn := range srcNames {
			for _, op := range []draw.Op{draw.Over, draw.Src} {
				for _, dr := range []drawRect{{image.Rect(1, 1, 5, 4), image.Point{0, 1}}, {image.Rect(-2, 0, 4, 3), image.Point{2, 2}}} {
					dimg, dpix := mkImg(dn, big)
					randPix(dn, dpix)
					before := append([]byte(nil), dpix...)
					var simg image.Image
					var spix, yp, cbp, crp []byte
					var palText = "[]"
					if sn == "YCbCr" {
						yc := image.NewYCbCr(image.Rect(0, 0, 4, 4), image.YCbCrSubsampleRatio420)
						copy(yc.Y, fill(len(yc.Y)))
						copy(yc.Cb, fill(len(yc.Cb)))
						copy(yc.Cr, fill(len(yc.Cr)))
						simg, yp, cbp, crp = yc, yc.Y, yc.Cb, yc.Cr
					} else {
						si, sp := mkImg(sn, image.Rect(0, 0, 4, 3))
						randPix(sn, sp)
						simg, spix = si, sp
						if sn == "Paletted" {
							var sb strings.Builder
							sb.WriteString("[")
							for i, v := range dpalv {
								if i > 0 {
									sb.WriteString(", ")
								}
								fmt.Fprintf(&sb, "%d", v)
							}
							sb.WriteString("]")
							palText = sb.String()
						}
					}
					draw.Draw(dimg, dr.rc, simg, dr.sp, op)
					opi := 0
					if op == draw.Src {
						opi = 1
					}
					after, _ := mkImg(dn, big)
					_ = after
					_, aft := func() (int, []byte) {
						switch v := dimg.(type) {
						case *image.RGBA:
							return 0, v.Pix
						case *image.NRGBA:
							return 0, v.Pix
						case *image.Gray:
							return 0, v.Pix
						case *image.Gray16:
							return 0, v.Pix
						case *image.RGBA64:
							return 0, v.Pix
						case *image.NRGBA64:
							return 0, v.Pix
						case *image.CMYK:
							return 0, v.Pix
						case *image.Paletted:
							return 0, v.Pix
						}
						return 0, nil
					}()
					fmt.Fprintf(&o, "  (%q, %s, %q, %s, %s, (%s, %s, %s), (%d, %d, %d, %d), (%d, %d), %d, %s),\n", dn, bytesLit(before), sn, bytesLit(spix), palText,
						bytesLit(yp), bytesLit(cbp), bytesLit(crp), dr.rc.Min.X, dr.rc.Min.Y, dr.rc.Max.X, dr.rc.Max.Y, dr.sp.X, dr.sp.Y, opi, bytesLit(aft))
				}
			}
		}
	}
	o.WriteString("]\n")
	o.WriteString(`
fn model_of(name: Str) -> image.Model {
  match name {
    "Gray" => image.Model.Gray
    "Gray16" => image.Model.Gray16
    "NRGBA" => image.Model.NRGBA
    "NRGBA64" => image.Model.NRGBA64
    "CMYK" => image.Model.CMYK
    "RGBA" => image.Model.RGBA
    "RGBA64" => image.Model.RGBA64
    "YCbCr" => image.Model.YCbCr
    _ => image.Model.Paletted
  }
}

test "Image.draw is Go's draw.Draw" {
  for (dn, dpix, sn, spix, pal, planes, rc, sp, opi, want) in DRAW_CASES {
    var dst = image.Image{width: 5, height: 4, model: model_of(dn), pix: dpix, palette: if dn == "Paletted" { DRAW_PAL } else { [] }}
    let (yp, cbp, crp) = planes
    var src = image.Image{width: 4, height: 3, model: model_of(sn), pix: spix, palette: pal}
    if sn == "YCbCr" {
      src = image.Image{width: 4, height: 4, model: image.Model.YCbCr, pix: b"", y: yp, cb: cbp, cr: crp, subsample: jpeg.Subsample.S420}
    }
    let (x0, y0, x1, y1) = rc
    let (spx, spy) = sp
    dst.draw(image.Rect{x0: x0, y0: y0, x1: x1, y1: y1}, src, spx, spy, if opi == 0 { image.Op.Over } else { image.Op.Src })
    expect((dn, sn, rc, sp, opi, dst.pix)) == (dn, sn, rc, sp, opi, want)
  }
}
`)
	// draw.DrawMask: every destination model against several source and mask
	// models, both ops, one rectangle with source and mask offsets and clipping.
	o.WriteString("\n/// (dst model, dst pix, src model, src pix, mask model, mask pix, rect, sp, mp, op, pix after): draw.DrawMask.\nconst MASK_CASES: List[(Str, Bytes, Str, Bytes, Str, Bytes, (Int, Int, Int, Int), (Int, Int), (Int, Int), Int, Bytes)] = [\n")
	pixOfD := func(v draw.Image) []byte {
		switch v := v.(type) {
		case *image.RGBA:
			return v.Pix
		case *image.NRGBA:
			return v.Pix
		case *image.Gray:
			return v.Pix
		case *image.Gray16:
			return v.Pix
		case *image.RGBA64:
			return v.Pix
		case *image.NRGBA64:
			return v.Pix
		case *image.CMYK:
			return v.Pix
		case *image.Paletted:
			return v.Pix
		}
		return nil
	}
	for _, dn := range dstNames {
		for _, sn := range []string{"RGBA", "NRGBA", "Gray", "RGBA64", "CMYK", "Paletted"} {
			for _, mn := range []string{"RGBA", "NRGBA", "RGBA64", "Gray"} {
				for _, op := range []draw.Op{draw.Over, draw.Src} {
					dimg, dpix := mkImg(dn, big)
					randPix(dn, dpix)
					before := append([]byte(nil), dpix...)
					simg, spix := mkImg(sn, image.Rect(0, 0, 4, 3))
					randPix(sn, spix)
					mimg, mpix := mkImg(mn, image.Rect(0, 0, 4, 3))
					randPix(mn, mpix)
					if mn == "NRGBA" {
						for i := 0; i < len(mpix); i += 4 {
							mpix[i+3] = byte(r.Intn(256))
							if i%8 == 0 {
								mpix[i+3] = 0
							}
						}
					}
					rc, sp, mp := image.Rect(1, 0, 5, 4), image.Point{0, 1}, image.Point{1, 0}
					draw.DrawMask(dimg, rc, simg, sp, mimg, mp, op)
					opi := 0
					if op == draw.Src {
						opi = 1
					}
					fmt.Fprintf(&o, "  (%q, %s, %q, %s, %q, %s, (%d, %d, %d, %d), (%d, %d), (%d, %d), %d, %s),\n", dn, bytesLit(before), sn, bytesLit(spix), mn, bytesLit(mpix),
						rc.Min.X, rc.Min.Y, rc.Max.X, rc.Max.Y, sp.X, sp.Y, mp.X, mp.Y, opi, bytesLit(pixOfD(dimg)))
				}
			}
		}
	}
	o.WriteString("]\n")
	o.WriteString(`
test "Image.draw_mask is Go's draw.DrawMask" {
  for (dn, dpix, sn, spix, mn, mpix, rc, sp, mp, opi, want) in MASK_CASES {
    var dst = image.Image{width: 5, height: 4, model: model_of(dn), pix: dpix, palette: if dn == "Paletted" { DRAW_PAL } else { [] }}
    let src = image.Image{width: 4, height: 3, model: model_of(sn), pix: spix, palette: if sn == "Paletted" { DRAW_PAL } else { [] }}
    let mask = image.Image{width: 4, height: 3, model: model_of(mn), pix: mpix}
    let (x0, y0, x1, y1) = rc
    let (spx, spy) = sp
    let (mpx, mpy) = mp
    dst.draw_mask(image.Rect{x0: x0, y0: y0, x1: x1, y1: y1}, src, spx, spy, mask, mpx, mpy, if opi == 0 { image.Op.Over } else { image.Op.Src })
    expect((dn, sn, mn, opi, dst.pix)) == (dn, sn, mn, opi, want)
  }
}
`)
	// draw.Draw from an image.Uniform.
	o.WriteString("\n/// (dst model, dst pix, color r g b a, rect, op, pix after): draw.Draw from an image.Uniform.\nconst FILL_CASES: List[(Str, Bytes, (Int, Int, Int, Int), (Int, Int, Int, Int), Int, Bytes)] = [\n")
	for _, dn := range dstNames {
		for _, cv := range [][4]int{{51400, 25700, 12850, 65535}, {30000, 10000, 5000, 40000}, {0, 0, 0, 0}} {
			for _, op := range []draw.Op{draw.Over, draw.Src} {
				for _, rc := range []image.Rectangle{image.Rect(1, 1, 4, 3), image.Rect(-1, 2, 7, 9)} {
					dimg, dpix := mkImg(dn, big)
					randPix(dn, dpix)
					before := append([]byte(nil), dpix...)
					u := image.NewUniform(color.RGBA64{uint16(cv[0]), uint16(cv[1]), uint16(cv[2]), uint16(cv[3])})
					draw.Draw(dimg, rc, u, image.Point{}, op)
					opi := 0
					if op == draw.Src {
						opi = 1
					}
					fmt.Fprintf(&o, "  (%q, %s, (%d, %d, %d, %d), (%d, %d, %d, %d), %d, %s),\n", dn, bytesLit(before), cv[0], cv[1], cv[2], cv[3],
						rc.Min.X, rc.Min.Y, rc.Max.X, rc.Max.Y, opi, bytesLit(pixOfD(dimg)))
				}
			}
		}
	}
	o.WriteString("]\n")
	o.WriteString(`
test "Image.fill is Go's draw.Draw from a Uniform" {
  for (dn, dpix, c, rc, opi, want) in FILL_CASES {
    var dst = image.Image{width: 5, height: 4, model: model_of(dn), pix: dpix, palette: if dn == "Paletted" { DRAW_PAL } else { [] }}
    let (cr, cg, cb, ca) = c
    let (x0, y0, x1, y1) = rc
    dst.fill(image.Rect{x0: x0, y0: y0, x1: x1, y1: y1}, image.Color{r: cr, g: cg, b: cb, a: ca}, if opi == 0 { image.Op.Over } else { image.Op.Src })
    expect((dn, opi, dst.pix)) == (dn, opi, want)
  }
}
`)

	// x/image/draw scaling, from its own module (imagescaleoracle).
	{
		cmd := exec.Command("go", "run", ".")
		cmd.Dir = filepath.Join("scripts", "stdcorpus", "imagescaleoracle")
		cmd.Stderr = os.Stderr
		out, err := cmd.Output()
		if err != nil {
			panic("imagescaleoracle: " + err.Error())
		}
		o.Write(out)
	}

	// draw.FloydSteinberg onto a Paletted image of the 216 web safe colors.
	o.WriteString("\n/// (src model, src pix, palette, (y, cb, cr), rect, sp, pix after): draw.FloydSteinberg onto a Paletted image of palette.WebSafe.\nconst FS_CASES: List[(Str, Bytes, List[Int], (Bytes, Bytes, Bytes), (Int, Int, Int, Int), (Int, Int), Bytes)] = [\n")
	for rep := 0; rep < 2; rep++ {
		for _, sn := range srcNames {
			for _, dr := range []drawRect{{image.Rect(0, 0, 5, 4), image.Point{0, 0}}, {image.Rect(1, 1, 5, 4), image.Point{1, 0}}} {
				pd := image.NewPaletted(big, palette.WebSafe)
				var simg image.Image
				var spix, yp, cbp, crp []byte
				palText := "[]"
				if sn == "YCbCr" {
					yc := image.NewYCbCr(image.Rect(0, 0, 5, 4), image.YCbCrSubsampleRatio420)
					copy(yc.Y, fill(len(yc.Y)))
					copy(yc.Cb, fill(len(yc.Cb)))
					copy(yc.Cr, fill(len(yc.Cr)))
					simg, yp, cbp, crp = yc, yc.Y, yc.Cb, yc.Cr
				} else {
					si, sp := mkImg(sn, image.Rect(0, 0, 5, 4))
					randPix(sn, sp)
					simg, spix = si, sp
					if sn == "Paletted" {
						var sb strings.Builder
						sb.WriteString("[")
						for i, v := range dpalv {
							if i > 0 {
								sb.WriteString(", ")
							}
							fmt.Fprintf(&sb, "%d", v)
						}
						sb.WriteString("]")
						palText = sb.String()
					}
				}
				draw.FloydSteinberg.Draw(pd, dr.rc, simg, dr.sp)
				fmt.Fprintf(&o, "  (%q, %s, %s, (%s, %s, %s), (%d, %d, %d, %d), (%d, %d), %s),\n", sn, bytesLit(spix), palText, bytesLit(yp), bytesLit(cbp), bytesLit(crp),
					dr.rc.Min.X, dr.rc.Min.Y, dr.rc.Max.X, dr.rc.Max.Y, dr.sp.X, dr.sp.Y, bytesLit(pd.Pix))
			}
		}
	}
	o.WriteString("]\n")
	imageColorModels(&o)
	o.WriteString(`
test "Image.floyd_steinberg is Go's draw.FloydSteinberg" {
  for (sn, spix, pal, planes, rc, sp, want) in FS_CASES {
    var dst = image.Image{width: 5, height: 4, model: image.Model.Paletted, pix: Bytes.zeroed(20), palette: WEB_SAFE}
    let (yp, cbp, crp) = planes
    var src = image.Image{width: 5, height: 4, model: model_of(sn), pix: spix, palette: pal}
    if sn == "YCbCr" {
      src = image.Image{width: 5, height: 4, model: image.Model.YCbCr, pix: b"", y: yp, cb: cbp, cr: crp, subsample: jpeg.Subsample.S420}
    }
    let (x0, y0, x1, y1) = rc
    let (spx, spy) = sp
    dst.floyd_steinberg(image.Rect{x0: x0, y0: y0, x1: x1, y1: y1}, src, spx, spy)
    expect(dst.pix) == want
  }
}
`)
	return o.String()
}

// imageColorModels writes image/color's models and color types (the
// rest of decision 0578's color step): every model's Convert of a color,
// every color type's RGBA(), Palette.Index and Convert, and an image
// model's Convert, as text lines the Xo test builds the same way.
func imageColorModels(o *strings.Builder) {
	r := rand.New(rand.NewSource(578))
	type c64 = color.RGBA64
	var colors []c64
	edge := []int{0, 1, 128, 255, 256, 32768, 65534, 65535}
	for _, a := range edge {
		for _, x := range edge {
			if x <= a {
				colors = append(colors, c64{R: uint16(x), G: uint16(a / 3), B: uint16(x / 2), A: uint16(a)})
			}
		}
	}
	for i := 0; i < 500; i++ {
		a := r.Intn(65536)
		if i%5 == 0 {
			a = 65535
		}
		colors = append(colors, c64{R: uint16(r.Intn(a + 1)), G: uint16(r.Intn(a + 1)), B: uint16(r.Intn(a + 1)), A: uint16(a)})
	}
	rgba := func(c color.Color) string {
		cr, cg, cb, ca := c.RGBA()
		return fmt.Sprintf("%d %d %d %d", cr, cg, cb, ca)
	}
	o.WriteString("/// (r, g, b, a, Go's models: AlphaModel Alpha16Model | RGBAModel | NRGBA64Model | CMYKModel | YCbCrModel | NYCbCrAModel)\nconst MODELS: List[(Int, Int, Int, Int, Str)] = [\n")
	for _, c := range colors {
		al := color.AlphaModel.Convert(c).(color.Alpha)
		al16 := color.Alpha16Model.Convert(c).(color.Alpha16)
		rg := color.RGBAModel.Convert(c).(color.RGBA)
		n64 := color.NRGBA64Model.Convert(c).(color.NRGBA64)
		cm := color.CMYKModel.Convert(c).(color.CMYK)
		yc := color.YCbCrModel.Convert(c).(color.YCbCr)
		ny := color.NYCbCrAModel.Convert(c).(color.NYCbCrA)
		s := fmt.Sprintf("%d %d | %d %d %d %d | %d %d %d %d | %d %d %d %d | %d %d %d | %d %d %d %d", al.A, al16.A, rg.R, rg.G, rg.B, rg.A, n64.R, n64.G, n64.B, n64.A, cm.C, cm.M, cm.Y, cm.K, yc.Y, yc.Cb, yc.Cr, ny.Y, ny.Cb, ny.Cr, ny.A)
		fmt.Fprintf(o, "  (%d, %d, %d, %d, %s),\n", c.R, c.G, c.B, c.A, xq(s))
	}
	o.WriteString("]\n\n/// (a, b, c, d (8 bit), e, f, g, h (16 bit), Go's RGBA() of Gray{a}, Gray16{e}, Alpha{a}, Alpha16{e}, RGBA{a, b, c, d}, RGBA64{e, f, g, h}, NRGBA64{e, f, g, h}, CMYK{a, b, c, d}, YCbCr{a, b, c}, NYCbCrA{a, b, c, d}, `;` between)\nconst TYPES: List[(Int, Int, Int, Int, Int, Int, Int, Int, Str)] = [\n")
	b8 := []int{0, 1, 127, 128, 254, 255}
	b16 := []int{0, 1, 255, 256, 32768, 65534, 65535}
	for i := 0; i < 500; i++ {
		v8 := func() int {
			if r.Intn(4) == 0 {
				return b8[r.Intn(len(b8))]
			}
			return r.Intn(256)
		}
		v16 := func() int {
			if r.Intn(4) == 0 {
				return b16[r.Intn(len(b16))]
			}
			return r.Intn(65536)
		}
		a, b, c, d := v8(), v8(), v8(), v8()
		e, f, g, h := v16(), v16(), v16(), v16()
		parts := []string{
			rgba(color.Gray{uint8(a)}), rgba(color.Gray16{uint16(e)}), rgba(color.Alpha{uint8(a)}), rgba(color.Alpha16{uint16(e)}),
			rgba(color.RGBA{uint8(a), uint8(b), uint8(c), uint8(d)}), rgba(color.RGBA64{uint16(e), uint16(f), uint16(g), uint16(h)}),
			rgba(color.NRGBA64{uint16(e), uint16(f), uint16(g), uint16(h)}), rgba(color.CMYK{uint8(a), uint8(b), uint8(c), uint8(d)}),
			rgba(color.YCbCr{uint8(a), uint8(b), uint8(c)}), rgba(color.NYCbCrA{color.YCbCr{uint8(a), uint8(b), uint8(c)}, uint8(d)}),
		}
		fmt.Fprintf(o, "  (%d, %d, %d, %d, %d, %d, %d, %d, %s),\n", a, b, c, d, e, f, g, h, xq(strings.Join(parts, ";")))
	}
	o.WriteString("]\n\n/// (palette as 0xRRGGBBAA, r, g, b, a, Go's Palette.Index, Convert's RGBA() or `nil`)\nconst PALETTES: List[(List[Int], Int, Int, Int, Int, Int, Str)] = [\n")
	for i := 0; i < 400; i++ {
		n := r.Intn(7)
		var pal color.Palette
		var ints []string
		for j := 0; j < n; j++ {
			v := color.NRGBA{uint8(r.Intn(256)), uint8(r.Intn(256)), uint8(r.Intn(256)), uint8([]int{0, 128, 255, 255, r.Intn(256)}[r.Intn(5)])}
			if j > 0 && r.Intn(5) == 0 {
				v = pal[r.Intn(j)].(color.NRGBA)
			}
			pal = append(pal, v)
			ints = append(ints, fmt.Sprint(int(v.R)<<24|int(v.G)<<16|int(v.B)<<8|int(v.A)))
		}
		c := colors[r.Intn(len(colors))]
		if n > 0 && r.Intn(4) == 0 {
			cr, cg, cb, ca := pal[r.Intn(n)].RGBA()
			c = c64{R: uint16(cr), G: uint16(cg), B: uint16(cb), A: uint16(ca)}
		}
		conv := "nil"
		if cv := pal.Convert(c); cv != nil {
			conv = rgba(cv)
		}
		idx := 0
		if n > 0 {
			idx = pal.Index(c)
		}
		fmt.Fprintf(o, "  ([%s], %d, %d, %d, %d, %d, %s),\n", strings.Join(ints, ", "), c.R, c.G, c.B, c.A, idx, xq(conv))
	}
	o.WriteString("]\n\n/// (model, r, g, b, a, Go's ColorModel().Convert(c).RGBA() of an image of the model; Paletted's palette is DRAW_PAL)\nconst IMAGE_CONVERT: List[(Str, Int, Int, Int, Int, Str)] = [\n")
	pal := color.Palette{color.NRGBA{10, 20, 30, 255}, color.NRGBA{200, 100, 50, 128}, color.NRGBA{0, 0, 0, 0}, color.NRGBA{255, 255, 255, 255}}
	models := []struct {
		name string
		m    color.Model
	}{{"Gray", color.GrayModel}, {"Gray16", color.Gray16Model}, {"Paletted", pal}, {"RGBA", color.RGBAModel}, {"NRGBA", color.NRGBAModel}, {"RGBA64", color.RGBA64Model}, {"NRGBA64", color.NRGBA64Model}, {"CMYK", color.CMYKModel}, {"YCbCr", color.YCbCrModel}}
	for i := 0; i < 300; i++ {
		m := models[r.Intn(len(models))]
		c := colors[r.Intn(len(colors))]
		fmt.Fprintf(o, "  (%s, %d, %d, %d, %d, %s),\n", xq(m.name), c.R, c.G, c.B, c.A, xq(rgba(m.m.Convert(c))))
	}
	o.WriteString("]\n")
	o.WriteString(`
fn rgba_text(c: image.Color) -> Str { "${c.r} ${c.g} ${c.b} ${c.a}" }

test "the color models are Go's" {
  var bad: List[(Int, Int, Int, Int, Str, Str)] = []
  for (r, g, b, a, want) in MODELS {
    let c = image.Color{r: r, g: g, b: b, a: a}
    let (r8, g8, b8, a8) = c.rgba()
    let (nr, ng, nb, na) = c.nrgba64()
    let (cc, cm, cy, ck) = c.cmyk()
    let (y, cb, cr) = c.ycbcr()
    let (ny, ncb, ncr, nya) = c.nycbcra()
    let got = "${c.alpha()} ${c.alpha16()} | ${r8} ${g8} ${b8} ${a8} | ${nr} ${ng} ${nb} ${na} | ${cc} ${cm} ${cy} ${ck} | ${y} ${cb} ${cr} | ${ny} ${ncb} ${ncr} ${nya}"
    if got != want {
      bad.push((r, g, b, a, got, want))
    }
  }
  expect(bad[0..bad.len().min(10)]) == []
}

test "the color types' RGBA are Go's" {
  var bad: List[(Str, Str)] = []
  for (a, b, c, d, e, f, g, h, want) in TYPES {
    let got = [image.from_gray(a), image.from_gray16(e), image.from_alpha(a), image.from_alpha16(e), image.from_rgba(a, b, c, d), image.from_rgba64(e, f, g, h), image.from_nrgba64(e, f, g, h), image.from_cmyk(a, b, c, d), image.from_ycbcr(a, b, c), image.from_nycbcra(a, b, c, d)].map(rgba_text).join(";")
    if got != want {
      bad.push(("${a} ${b} ${c} ${d} ${e} ${f} ${g} ${h}", got))
    }
  }
  expect(bad[0..bad.len().min(10)]) == []
}

test "palette_index and palette_convert are Go's Palette" {
  var bad: List[Str] = []
  for (pal, r, g, b, a, idx, conv) in PALETTES {
    let c = image.Color{r: r, g: g, b: b, a: a}
    let got = (image.palette_index(pal, c), image.palette_convert(pal, c).map(rgba_text) ?? "nil")
    if got != (idx, conv) {
      bad.push("${pal} ${rgba_text(c)}: ${got.0} ${got.1}, want ${idx} ${conv}")
    }
  }
  expect(bad[0..bad.len().min(10)]) == []
}

test "Image.convert is Go's ColorModel().Convert" {
  var bad: List[Str] = []
  for (m, r, g, b, a, want) in IMAGE_CONVERT {
    let im = image.Image{width: 1, height: 1, model: model_of(m), pix: b"", palette: DRAW_PAL}
    let got = rgba_text(im.convert(image.Color{r: r, g: g, b: b, a: a}))
    if got != want {
      bad.push("${m} ${r} ${g} ${b} ${a}: ${got}, want ${want}")
    }
  }
  expect(bad[0..bad.len().min(10)]) == []
}

test "the named colors are Go's" {
  expect([image.black(), image.white(), image.transparent(), image.opaque()].map(rgba_text)) == ["0 0 0 65535", "65535 65535 65535 65535", "0 0 0 0", "65535 65535 65535 65535"]
}
`)
}
