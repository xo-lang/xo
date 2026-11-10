// Command imagescaleoracle prints the std/image scaling corpus:
// golang.org/x/image v0.46.0's draw package (std/image's reference for
// scaling) on seeded random images. scripts/stdcorpus runs it and appends
// the output to the image corpus; it is its own module so the repository's
// go.mod has no third-party requirement.
package main

import (
	"fmt"
	"image"
	"image/color"
	"math/rand"
	"os"
	"strings"

	xdraw "golang.org/x/image/draw"
)

func bytesLit(b []byte) string {
	var sb strings.Builder
	sb.WriteString(`b"`)
	for _, c := range b {
		fmt.Fprintf(&sb, `\x%02x`, c)
	}
	sb.WriteString(`"`)
	return sb.String()
}

var pal = color.Palette{color.NRGBA{10, 20, 30, 255}, color.NRGBA{200, 100, 50, 128}, color.NRGBA{0, 0, 0, 0}, color.NRGBA{255, 255, 255, 255}}

type pimg interface {
	xdraw.Image
}

func mk(name string, rc image.Rectangle) (xdraw.Image, []byte) {
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
	v := image.NewPaletted(rc, pal)
	return v, v.Pix
}

func main() {
	r := rand.New(rand.NewSource(46))
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
			r.Read(px)
		}
	}
	dstNames := []string{"RGBA", "NRGBA", "Gray", "Gray16", "RGBA64", "NRGBA64", "CMYK", "Paletted"}
	srcNames := []string{"RGBA", "NRGBA", "Gray", "Gray16", "RGBA64", "NRGBA64", "CMYK", "Paletted"}
	type sc struct {
		dr, sr image.Rectangle
	}
	cases := []sc{
		{image.Rect(0, 0, 7, 5), image.Rect(0, 0, 4, 3)},
		{image.Rect(1, 1, 4, 3), image.Rect(0, 0, 4, 3)},
		{image.Rect(0, 0, 5, 4), image.Rect(1, 1, 4, 3)},
		{image.Rect(-2, 0, 5, 4), image.Rect(0, 0, 4, 3)},
		{image.Rect(1, 0, 5, 3), image.Rect(0, 0, 4, 3)},
	}
	var o strings.Builder
	o.WriteString("\n/// (kernel, dst model, dst pix, src model, src pix, dst rect, src rect, op, pix after): x/image/draw Scale (5 by 4 destination, 4 by 3 source).\nconst NN_CASES: List[(Int, Str, Bytes, Str, Bytes, (Int, Int, Int, Int), (Int, Int, Int, Int), Int, Bytes)] = [\n")
	type kn struct {
		sc xdraw.Scaler
	}
	ks := []kn{{xdraw.NearestNeighbor}, {xdraw.BiLinear}, {xdraw.CatmullRom}, {xdraw.ApproxBiLinear}}
	for ki, k := range ks {
		dns, sns := dstNames, srcNames
		if ki > 0 {
			dns = []string{"RGBA", "NRGBA", "Gray", "Paletted"}
			sns = []string{"RGBA", "NRGBA", "Gray", "RGBA64", "CMYK", "Paletted"}
		}
		for _, dn := range dns {
			for _, sn := range sns {
				for _, op := range []xdraw.Op{xdraw.Over, xdraw.Src} {
					for _, c := range cases {
						dimg, dpix := mk(dn, image.Rect(0, 0, 5, 4))
						randPix(dn, dpix)
						before := append([]byte(nil), dpix...)
						simg, spix := mk(sn, image.Rect(0, 0, 4, 3))
						randPix(sn, spix)
						k.sc.Scale(dimg, c.dr, simg, c.sr, op, nil)
						opi := 0
						if op == xdraw.Src {
							opi = 1
						}
						var after []byte
						switch v := dimg.(type) {
						case *image.RGBA:
							after = v.Pix
						case *image.NRGBA:
							after = v.Pix
						case *image.Gray:
							after = v.Pix
						case *image.Gray16:
							after = v.Pix
						case *image.RGBA64:
							after = v.Pix
						case *image.NRGBA64:
							after = v.Pix
						case *image.CMYK:
							after = v.Pix
						case *image.Paletted:
							after = v.Pix
						}
						fmt.Fprintf(&o, "  (%d, %q, %s, %q, %s, (%d, %d, %d, %d), (%d, %d, %d, %d), %d, %s),\n", ki, dn, bytesLit(before), sn, bytesLit(spix),
							c.dr.Min.X, c.dr.Min.Y, c.dr.Max.X, c.dr.Max.Y, c.sr.Min.X, c.sr.Min.Y, c.sr.Max.X, c.sr.Max.Y, opi, bytesLit(after))
					}
				}
			}
		}
	}
	o.WriteString("]\n")
	o.WriteString(`
test "Image.scale is x/image/draw Scale" {
  for (ki, dn, dpix, sn, spix, drc, src_rc, opi, want) in NN_CASES {
    var dst = image.Image{width: 5, height: 4, model: model_of(dn), pix: dpix, palette: if dn == "Paletted" { DRAW_PAL } else { [] }}
    let src = image.Image{width: 4, height: 3, model: model_of(sn), pix: spix, palette: if sn == "Paletted" { DRAW_PAL } else { [] }}
    let (dx0, dy0, dx1, dy1) = drc
    let (sx0, sy0, sx1, sy1) = src_rc
    dst.scale(image.Rect{x0: dx0, y0: dy0, x1: dx1, y1: dy1}, src, image.Rect{x0: sx0, y0: sy0, x1: sx1, y1: sy1}, if opi == 0 { image.Op.Over } else { image.Op.Src }, if ki == 0 { image.Kernel.NearestNeighbor } else if ki == 1 { image.Kernel.BiLinear } else if ki == 2 { image.Kernel.CatmullRom } else { image.Kernel.ApproxBiLinear })
    var bad = -1
    for i in 0..want.len() {
      if dst.pix[i] != want[i] {
        bad = i
        break
      }
    }
    var got4: List[Int] = []
    var want4: List[Int] = []
    if bad >= 0 {
      let q = bad - bad % 4
      for k in 0..4 {
        got4.push(Int(dst.pix[q + k]))
        want4.push(Int(want[q + k]))
      }
    }
    expect((ki, dn, sn, opi, bad, got4, want4)) == (ki, dn, sn, opi, -1, [], [])
  }
}
`)
	os.Stdout.WriteString(o.String())
}
