package main

import (
	"bytes"
	"compress/lzw"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"math/rand"
	"strings"
)

// gifCorpus is image/gif (std/gif, decision 0578 step 5): Go's decoder over
// encoded animations, truncations and byte changes of them, and hand built
// streams for each error text; Go's encoder over images of several models,
// palettes and sizes, and EncodeAll over animations.
func gifCorpus() string {
	var o strings.Builder
	o.WriteString(header("image/gif (std/gif, decision 0578)"))
	o.WriteString("use std/gif\nuse std/image\n\n")
	r := rand.New(rand.NewSource(5780))
	lit := func(b []byte) string {
		var sb strings.Builder
		sb.WriteString(`b"`)
		for _, c := range b {
			fmt.Fprintf(&sb, `\x%02x`, c)
		}
		sb.WriteString(`"`)
		return sb.String()
	}
	palInts := func(p color.Palette) []int {
		var out []int
		for _, c := range p {
			n := color.NRGBAModel.Convert(c).(color.NRGBA)
			out = append(out, int(n.R)<<24|int(n.G)<<16|int(n.B)<<8|int(n.A))
		}
		return out
	}
	intsText := func(v []int) string {
		var sb strings.Builder
		for i, x := range v {
			if i > 0 {
				sb.WriteString(",")
			}
			fmt.Fprintf(&sb, "%08x", x)
		}
		return sb.String()
	}
	xoInts := func(v []int) string {
		parts := make([]string, len(v))
		for i, x := range v {
			parts[i] = fmt.Sprintf("0x%08x", x)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	frameText := func(m *image.Paletted, delay, disposal int) string {
		b := m.Bounds()
		pix := make([]byte, 0, b.Dx()*b.Dy())
		for y := b.Min.Y; y < b.Max.Y; y++ {
			i := m.PixOffset(b.Min.X, y)
			pix = append(pix, m.Pix[i:i+b.Dx()]...)
		}
		return fmt.Sprintf("|%d,%d,%d,%d,d%d,p%d,[%s],%s", b.Min.X, b.Min.Y, b.Dx(), b.Dy(), delay, disposal, intsText(palInts(m.Palette)), hex.EncodeToString(pix))
	}
	allText := func(data []byte) string {
		g, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil {
			return "ERR " + err.Error()
		}
		var gp []int
		if p, ok := g.Config.ColorModel.(color.Palette); ok {
			gp = palInts(p)
		}
		s := fmt.Sprintf("%dx%d loop=%d bg=%d gpal=[%s] frames=%d", g.Config.Width, g.Config.Height, g.LoopCount, g.BackgroundIndex, intsText(gp), len(g.Image))
		for i, m := range g.Image {
			s += frameText(m, g.Delay[i], int(g.Disposal[i]))
		}
		return s
	}
	firstText := func(data []byte) string {
		im, err := gif.Decode(bytes.NewReader(data))
		if err != nil {
			return "ERR " + err.Error()
		}
		return frameText(im.(*image.Paletted), 0, 0)
	}
	cfgText := func(data []byte) string {
		c, err := gif.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return "ERR " + err.Error()
		}
		var gp []int
		if p, ok := c.ColorModel.(color.Palette); ok {
			gp = palInts(p)
		}
		return fmt.Sprintf("%dx%d gpal=[%s]", c.Width, c.Height, intsText(gp))
	}
	randPal := func(n int, transparent int) color.Palette {
		p := make(color.Palette, n)
		for i := range p {
			p[i] = color.RGBA{uint8(r.Intn(256)), uint8(r.Intn(256)), uint8(r.Intn(256)), 255}
		}
		if transparent >= 0 && transparent < n {
			p[transparent] = color.RGBA{}
		}
		return p
	}
	randFrame := func(rc image.Rectangle, p color.Palette) *image.Paletted {
		m := image.NewPaletted(rc, p)
		for i := range m.Pix {
			if r.Intn(3) == 0 && i > 0 {
				m.Pix[i] = m.Pix[i-1]
			} else {
				m.Pix[i] = uint8(r.Intn(len(p)))
			}
		}
		return m
	}

	// stream builder for the hand built cases
	type sb struct{ b []byte }
	put := func(s *sb, v ...byte) { s.b = append(s.b, v...) }
	le16 := func(v int) []byte { return []byte{byte(v), byte(v >> 8)} }
	header89 := func(s *sb, w, h int, gct color.Palette, bg int) {
		put(s, []byte("GIF89a")...)
		put(s, le16(w)...)
		put(s, le16(h)...)
		if gct != nil {
			n := 1
			for 1<<(n+1) < len(gct) {
				n++
			}
			put(s, 0x80|byte(n-1+0), byte(bg), 0)
			for i := 0; i < 1<<n; i++ {
				if i < len(gct) {
					c := gct[i].(color.RGBA)
					put(s, c.R, c.G, c.B)
				} else {
					put(s, 0, 0, 0)
				}
			}
		} else {
			put(s, 0, 0, 0)
		}
	}
	blocks := func(data []byte) []byte {
		var out []byte
		for len(data) > 0 {
			n := len(data)
			if n > 255 {
				n = 255
			}
			out = append(out, byte(n))
			out = append(out, data[:n]...)
			data = data[n:]
		}
		return append(out, 0)
	}
	lzwBytes := func(pix []byte, lw int) []byte {
		var b bytes.Buffer
		w := lzw.NewWriter(&b, lzw.LSB, lw)
		w.Write(pix)
		w.Close()
		return b.Bytes()
	}
	image89 := func(s *sb, x, y, w, h int, fields byte, lct color.Palette, lw int, pix []byte, raw []byte) {
		put(s, 0x2c)
		put(s, le16(x)...)
		put(s, le16(y)...)
		put(s, le16(w)...)
		put(s, le16(h)...)
		put(s, fields)
		if lct != nil {
			for _, c := range lct {
				cc := c.(color.RGBA)
				put(s, cc.R, cc.G, cc.B)
			}
		}
		put(s, byte(lw))
		if raw != nil {
			put(s, raw...)
		} else {
			put(s, blocks(lzwBytes(pix, lw))...)
		}
	}
	gc := func(s *sb, flags byte, delay int, ti byte) {
		put(s, 0x21, 0xf9, 4, flags)
		put(s, le16(delay)...)
		put(s, ti, 0)
	}

	type dcase struct{ data []byte }
	var cases []dcase
	add := func(b []byte) { cases = append(cases, dcase{b}) }

	// Go encoded animations and stills
	var bases [][]byte
	for i := 0; i < 14; i++ {
		n := []int{2, 3, 4, 7, 16, 17, 64, 100, 200, 256, 5, 9, 31, 128}[i]
		p := randPal(n, []int{-1, 0, 1}[i%3])
		w, h := 1+r.Intn(14), 1+r.Intn(12)
		var g gif.GIF
		frames := 1 + i%4
		for f := 0; f < frames; f++ {
			x, y := r.Intn(w), r.Intn(h)
			rc := image.Rect(x, y, x+1+r.Intn(w-x), y+1+r.Intn(h-y))
			if f == 0 {
				rc = image.Rect(0, 0, w, h)
			}
			fp := p
			if f%2 == 1 && n > 3 {
				fp = randPal(n, f%3-1)
			}
			g.Image = append(g.Image, randFrame(rc, fp))
			g.Delay = append(g.Delay, r.Intn(30))
			g.Disposal = append(g.Disposal, byte(r.Intn(4)))
		}
		g.LoopCount = []int{-1, 0, 3, 65535}[i%4]
		g.Config = image.Config{ColorModel: p, Width: w, Height: h}
		g.BackgroundIndex = byte(r.Intn(n))
		var b bytes.Buffer
		if err := gif.EncodeAll(&b, &g); err != nil {
			panic(err)
		}
		bases = append(bases, b.Bytes())
		add(b.Bytes())
	}
	// a larger image (several sub-blocks, a table reset)
	{
		p := randPal(256, 7)
		m := image.NewPaletted(image.Rect(0, 0, 90, 70), p)
		for i := range m.Pix {
			m.Pix[i] = uint8(r.Intn(256))
		}
		var b bytes.Buffer
		gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{m}, Delay: []int{4}, Config: image.Config{ColorModel: p, Width: 90, Height: 70}})
		add(b.Bytes())
	}
	// truncations and byte changes
	for bi, base := range bases {
		if bi%2 != 0 {
			continue
		}
		step := 1
		if len(base) > 150 {
			step = len(base) / 150
		}
		for n := 0; n < len(base); n += step {
			add(base[:n])
		}
		for k := 0; k < 40; k++ {
			c := append([]byte(nil), base...)
			c[r.Intn(len(c))] = byte(r.Intn(256))
			add(c)
		}
	}
	// hand built streams
	gct4 := randPal(4, -1)
	pix6 := []byte{0, 1, 2, 3, 2, 1}
	{
		var s sb
		header89(&s, 3, 2, gct4, 0)
		image89(&s, 0, 0, 3, 2, 0, nil, 2, pix6, nil)
		put(&s, 0x3b)
		add(s.b)
		// trailer missing
		add(s.b[:len(s.b)-1])
		// GIF87a
		c := append([]byte(nil), s.b...)
		copy(c, "GIF87a")
		add(c)
		copy(c, "GIF90a")
		add(c)
		copy(c, "GIF8\x00\xffa")
		add(c)
		// not enough, too much
		var t sb
		header89(&t, 3, 2, gct4, 0)
		image89(&t, 0, 0, 3, 2, 0, nil, 2, pix6[:5], nil)
		put(&t, 0x3b)
		add(t.b)
		var u sb
		header89(&u, 3, 2, gct4, 0)
		image89(&u, 0, 0, 3, 2, 0, nil, 2, append(append([]byte(nil), pix6...), 1, 2), nil)
		put(&u, 0x3b)
		add(u.b)
		// bad pixel (index 5 with a 4 entry palette)
		var v sb
		header89(&v, 3, 2, gct4, 0)
		image89(&v, 0, 0, 3, 2, 0, nil, 3, []byte{0, 1, 5, 3, 2, 1}, nil)
		put(&v, 0x3b)
		add(v.b)
		// literal width out of range
		for _, lw := range []int{1, 9, 0} {
			var w sb
			header89(&w, 3, 2, gct4, 0)
			image89(&w, 0, 0, 3, 2, 0, nil, lw, pix6, []byte{2, 0, 0, 0})
			put(&w, 0x3b)
			add(w.b)
		}
		// frame bounds
		var x sb
		header89(&x, 3, 2, gct4, 0)
		image89(&x, 2, 0, 3, 2, 0, nil, 2, pix6, nil)
		put(&x, 0x3b)
		add(x.b)
		// no color table
		var y sb
		header89(&y, 3, 2, nil, 0)
		image89(&y, 0, 0, 3, 2, 0, nil, 2, pix6, nil)
		put(&y, 0x3b)
		add(y.b)
		// local color table and transparency (index beyond the palette too)
		var z sb
		header89(&z, 3, 2, nil, 0)
		gc(&z, 0x09, 7, 2)
		image89(&z, 0, 0, 3, 2, 0x81, randPal(4, -1), 2, pix6, nil)
		gc(&z, 0x01, 5, 9)
		image89(&z, 0, 0, 3, 2, 0x81, randPal(4, -1), 2, pix6, nil)
		put(&z, 0x3b)
		add(z.b)
		// interlaced 3x9
		var il sb
		header89(&il, 3, 9, gct4, 0)
		ip := make([]byte, 27)
		for i := range ip {
			ip[i] = byte(i / 3 % 4)
		}
		image89(&il, 0, 0, 3, 9, 0x40, nil, 2, ip, nil)
		put(&il, 0x3b)
		add(il.b)
		// extensions: comment, plain text, application, NETSCAPE, unknown, bad graphic control
		ext := func(label byte, body ...[]byte) []byte {
			e := []byte{0x21, label}
			for _, b := range body {
				e = append(e, b...)
			}
			return e
		}
		for _, e := range [][]byte{
			ext(0xfe, []byte{3, 'a', 'b', 'c', 0}),
			ext(0x01, append([]byte{13}, make([]byte, 13)...), []byte{2, 'h', 'i', 0}),
			ext(0xff, []byte{11}, []byte("XMP DataXMP"), []byte{2, 1, 2, 0}),
			ext(0xff, []byte{11}, []byte("NETSCAPE2.0"), []byte{3, 1, 9, 0, 0}),
			ext(0xff, []byte{11}, []byte("NETSCAPE2.0"), []byte{3, 2, 9, 0, 0}),
			ext(0xff, []byte{11}, []byte("NETSCAPE2.0"), []byte{0}),
			ext(0x77, []byte{0}),
			ext(0xf9, []byte{5, 0, 0, 0, 0, 0, 0}),
			ext(0xf9, []byte{4, 0, 0, 0, 0, 1}),
			ext(0xf9, []byte{4, 0x1c, 1, 2, 3, 0}),
		} {
			var s2 sb
			header89(&s2, 3, 2, gct4, 0)
			put(&s2, e...)
			image89(&s2, 0, 0, 3, 2, 0, nil, 2, pix6, nil)
			put(&s2, 0x3b)
			add(s2.b)
			add(s2.b[:len(s2.b)/2])
		}
		// unknown block type and an image block missing
		var q sb
		header89(&q, 3, 2, gct4, 0)
		put(&q, 0x99)
		add(q.b)
		var q2 sb
		header89(&q2, 3, 2, gct4, 0)
		put(&q2, 0x3b)
		add(q2.b)
		add([]byte("GIF89a"))
		add(nil)
	}
	// random garbage after a good header
	for i := 0; i < 30; i++ {
		var s2 sb
		header89(&s2, 3, 2, gct4, 0)
		junk := make([]byte, 1+r.Intn(30))
		r.Read(junk)
		put(&s2, junk...)
		add(s2.b)
	}

	o.WriteString("/// (gif bytes, DecodeAll summary, Decode summary, DecodeConfig summary)\nconst DEC_CASES: List[(Bytes, Str, Str, Str)] = [\n")
	for _, c := range cases {
		fmt.Fprintf(&o, "  (%s, %q, %q, %q),\n", lit(c.data), allText(c.data), firstText(c.data), cfgText(c.data))
	}
	o.WriteString("]\n")

	// encoders
	o.WriteString("\n/// (model, w, h, pix, palette, num_colors, Go's Encode bytes or error text)\nconst ENC_CASES: List[(Str, Int, Int, Bytes, List[Int], Int, Bytes, Str)] = [\n")
	type ecase struct {
		model string
		w, h  int
		pix   []byte
		pal   []int
		num   int
	}
	var ecs []ecase
	for _, w := range []int{1, 5, 17} {
		for _, h := range []int{1, 4, 13} {
			for _, num := range []int{0, 256, 16, 3, 1} {
				for _, model := range []string{"Paletted", "Paletted64", "RGBA", "NRGBA", "Gray"} {
					e := ecase{model: model, w: w, h: h, num: num}
					switch model {
					case "Paletted":
						e.pal = palInts(randPal(2+r.Intn(14), r.Intn(3)-1))
						e.pix = make([]byte, w*h)
						for i := range e.pix {
							e.pix[i] = byte(r.Intn(len(e.pal)))
						}
					case "Paletted64":
						e.model = "Paletted"
						e.pal = palInts(randPal(64, 5))
						e.pix = make([]byte, w*h)
						for i := range e.pix {
							e.pix[i] = byte(r.Intn(64))
						}
					case "RGBA":
						e.pix = make([]byte, 4*w*h)
						for i := 0; i < len(e.pix); i += 4 {
							a := r.Intn(256)
							e.pix[i], e.pix[i+1], e.pix[i+2], e.pix[i+3] = byte(r.Intn(a+1)), byte(r.Intn(a+1)), byte(r.Intn(a+1)), byte(a)
						}
					case "NRGBA":
						e.pix = make([]byte, 4*w*h)
						r.Read(e.pix)
					case "Gray":
						e.pix = make([]byte, w*h)
						r.Read(e.pix)
					}
					ecs = append(ecs, e)
				}
			}
		}
	}
	goImage := func(e ecase) image.Image {
		rc := image.Rect(0, 0, e.w, e.h)
		switch e.model {
		case "Paletted":
			p := make(color.Palette, len(e.pal))
			for i, v := range e.pal {
				p[i] = color.NRGBA{uint8(v >> 24), uint8(v >> 16), uint8(v >> 8), uint8(v)}
			}
			return &image.Paletted{Pix: e.pix, Stride: e.w, Rect: rc, Palette: p}
		case "RGBA":
			return &image.RGBA{Pix: e.pix, Stride: 4 * e.w, Rect: rc}
		case "NRGBA":
			return &image.NRGBA{Pix: e.pix, Stride: 4 * e.w, Rect: rc}
		}
		return &image.Gray{Pix: e.pix, Stride: e.w, Rect: rc}
	}
	for _, e := range ecs {
		var b bytes.Buffer
		err := gif.Encode(&b, goImage(e), &gif.Options{NumColors: e.num})
		et := ""
		if err != nil {
			et = err.Error()
		}
		pal := xoInts(e.pal)
		fmt.Fprintf(&o, "  (%q, %d, %d, %s, %s, %d, %s, %q),\n", e.model, e.w, e.h, lit(e.pix), pal, e.num, lit(b.Bytes()), et)
	}
	o.WriteString("]\n")

	// EncodeAll animations
	o.WriteString("\n/// (screen w, h, loop, bg, global palette, frames (x, y, w, h, delay, disposal, palette, pix), disposal given, Go's EncodeAll bytes or error text)\nconst ALL_CASES: List[(Int, Int, Int, Int, List[Int], List[(Int, Int, Int, Int, Int, Int, List[Int], Bytes)], Int, Bytes, Str)] = [\n")
	for i := 0; i < 40; i++ {
		w, h := 1+r.Intn(20), 1+r.Intn(16)
		n := []int{2, 3, 4, 8, 16, 17, 100, 256}[i%8]
		gp := randPal(n, []int{-1, 0, 1}[i%3])
		g := gif.GIF{LoopCount: []int{-1, 0, 2}[i%3], BackgroundIndex: byte(r.Intn(n))}
		g.Config = image.Config{ColorModel: gp, Width: w, Height: h}
		if i%10 == 9 {
			g.Config = image.Config{}
		}
		frames := 1 + i%3
		for f := 0; f < frames; f++ {
			x, y := r.Intn(w), r.Intn(h)
			rc := image.Rect(x, y, x+1+r.Intn(w-x), y+1+r.Intn(h-y))
			fp := gp
			switch f {
			case 1:
				fp = randPal(n, -1)
			case 2:
				fp = gp[:1+r.Intn(n)]
				if len(fp) < 2 {
					fp = gp[:2]
				}
			}
			g.Image = append(g.Image, randFrame(rc, fp))
			g.Delay = append(g.Delay, r.Intn(5))
			g.Disposal = append(g.Disposal, byte(r.Intn(4)))
		}
		if i%10 == 9 {
			g.Image[0] = randFrame(image.Rect(0, 0, w, h), gp)
		}
		if i%4 == 3 {
			g.Disposal = nil
		}
		var b bytes.Buffer
		err := gif.EncodeAll(&b, &g)
		et := ""
		if err != nil {
			et = err.Error()
		}
		var fr strings.Builder
		fr.WriteString("[")
		for k, m := range g.Image {
			if k > 0 {
				fr.WriteString(", ")
			}
			bd := m.Bounds()
			pix := make([]byte, 0)
			for y := bd.Min.Y; y < bd.Max.Y; y++ {
				j := m.PixOffset(bd.Min.X, y)
				pix = append(pix, m.Pix[j:j+bd.Dx()]...)
			}
			d := 0
			if g.Disposal != nil {
				d = int(g.Disposal[k])
			}
			fmt.Fprintf(&fr, "(%d, %d, %d, %d, %d, %d, %s, %s)", bd.Min.X, bd.Min.Y, bd.Dx(), bd.Dy(), g.Delay[k], d, xoInts(palInts(m.Palette)), lit(pix))
		}
		fr.WriteString("]")
		var gpt []int
		if p, ok := g.Config.ColorModel.(color.Palette); ok {
			gpt = palInts(p)
		}
		hasD := 0
		if g.Disposal != nil {
			hasD = 1
		}
		fmt.Fprintf(&o, "  (%d, %d, %d, %d, %s, %s, %d, %s, %q),\n", g.Config.Width, g.Config.Height, g.LoopCount, g.BackgroundIndex, xoInts(gpt), fr.String(), hasD, lit(b.Bytes()), et)
	}
	o.WriteString("]\n")
	o.WriteString(gifTests)
	return o.String()
}

const gifTests = `
fn hexs(v: List[Int]) -> Str {
  var parts: List[Str] = []
  for x in v {
    parts.push(x.to_str_width(8, 16, true))
  }
  parts.join(",")
}

fn frame_text(m: image.Image, delay: Int, disposal: Int) -> Str {
  "|${m.min_x},${m.min_y},${m.width},${m.height},d${delay},p${disposal},[${hexs(m.palette)}],${m.pix.to_hex()}"
}

fn all_text(data: Bytes) -> Str {
  match gif.decode_all(data) {
    Ok(g) => {
      var s = "${g.width}x${g.height} loop=${g.loop_count} bg=${g.background_index} gpal=[${hexs(g.palette)}] frames=${g.frames.len()}"
      var i = 0
      for f in g.frames {
        s = s + frame_text(f, g.delay[i], g.disposal[i])
        i += 1
      }
      s
    }
    Err(e) => "ERR ${e.msg}"
  }
}

fn first_text(data: Bytes) -> Str {
  match gif.decode(data) {
    Ok(m) => frame_text(m, 0, 0)
    Err(e) => "ERR ${e.msg}"
  }
}

fn cfg_text(data: Bytes) -> Str {
  match gif.decode_config(data) {
    Ok(c) => "${c.width}x${c.height} gpal=[${hexs(c.palette)}]"
    Err(e) => "ERR ${e.msg}"
  }
}

test "decode, decode_all and decode_config match Go" {
  for (data, all, first, cfg) in DEC_CASES {
    expect((data, all_text(data))) == (data, all)
    expect((data, first_text(data))) == (data, first)
    expect((data, cfg_text(data))) == (data, cfg)
  }
}

fn model_of(m: Str) -> image.Model {
  if m == "Paletted" { image.Model.Paletted } else if m == "RGBA" { image.Model.RGBA } else if m == "NRGBA" { image.Model.NRGBA } else { image.Model.Gray }
}

test "encode matches Go's Encode" {
  for (model, w, h, pix, pal, num, want, err) in ENC_CASES {
    let bpp = if model == "RGBA" || model == "NRGBA" { 4 } else { 1 }
    let im = image.Image{width: w, height: h, model: model_of(model), pix: pix, stride: w * bpp, palette: pal}
    match gif.encode(im, num) {
      Ok(got) => expect((model, w, h, num, "", got)) == (model, w, h, num, err, want)
      Err(e) => expect((model, w, h, num, e.msg)) == (model, w, h, num, err)
    }
  }
}

test "encode_all matches Go's EncodeAll" {
  var n = 0
  for (w, h, loop, bg, gpal, frames, has_d, want, err) in ALL_CASES {
    var fs: List[image.Image] = []
    var delay: List[Int] = []
    var disposal: List[Int] = []
    for (x, y, fw, fh, d, dp, pal, pix) in frames {
      fs.push(image.Image{width: fw, height: fh, model: image.Model.Paletted, pix: pix, stride: fw, palette: pal, min_x: x, min_y: y})
      delay.push(d)
      if has_d == 1 {
        disposal.push(dp)
      }
    }
    let g = gif.Gif{frames: fs, delay: delay, disposal: disposal, loop_count: loop, width: w, height: h, palette: gpal, background_index: bg}
    match gif.encode_all(g) {
      Ok(got) => expect((n, "", got)) == (n, err, want)
      Err(e) => expect((n, e.msg)) == (n, err)
    }
    n += 1
  }
}

test "encode_all argument errors" {
  let im = image.Image{width: 1, height: 1, model: image.Model.Paletted, pix: b"\x00", stride: 1, palette: [0xff0000ff, 0x00ff00ff]}
  match gif.encode_all(gif.Gif{frames: [], delay: [], disposal: [], loop_count: -1, width: 1, height: 1, palette: [], background_index: 0}) {
    Ok(_) => expect(false) == true
    Err(e) => expect(e.msg) == "gif: must provide at least one image"
  }
  match gif.encode_all(gif.Gif{frames: [im], delay: [], disposal: [], loop_count: -1, width: 1, height: 1, palette: [], background_index: 0}) {
    Ok(_) => expect(false) == true
    Err(e) => expect(e.msg) == "gif: mismatched image and delay lengths"
  }
}
`
