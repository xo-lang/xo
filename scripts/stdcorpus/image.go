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
	return o.String()
}
