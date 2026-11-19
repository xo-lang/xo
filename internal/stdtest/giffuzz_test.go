// A fuzz harness for std/gif against Go's image/gif (decision 0616's fuzz
// kind). Each case is a GIF Go's EncodeAll writes from random frames
// (1 to 3 frames of up to 12 by 12 pixels at random offsets on a screen of
// up to 16 by 16, a global palette or local ones of 1 to 256 entries, a
// transparent entry, delays, disposal methods and loop counts), then, for
// most cases, mutated: bits flipped, bytes set, the stream cut short, a
// block's length byte changed or bytes inserted. Go's Decode, DecodeAll and
// DecodeConfig of the bytes, as the summaries the gif corpus compares
// (scripts/stdcorpus gif.go: each frame's bounds, delay, disposal, palette
// and pixels, or `ERR` and Go's error text), are what std/gif's decode,
// decode_all and decode_config must give.
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/giffuzz/giffuzz_test.xo, which TestStdCorpora runs on
// every corpus backend; TestGifFuzzCorpusCurrent checks it is what this
// generator writes (XO_GIFFUZZ_WRITE=1 rewrites it). TestGifFuzz runs a
// fresh seed (XO_GIFFUZZ_SEED, default 2; XO_GIFFUZZ_N cases, default 1000)
// on XO_GIFFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"math/rand"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/hostile"
)

// gfCase is one input, how it was made, and Go's three summaries.
type gfCase struct {
	data             []byte
	how              string
	all, first, conf string
}

func gfPalInts(p color.Palette) []int {
	var out []int
	for _, c := range p {
		n := color.NRGBAModel.Convert(c).(color.NRGBA)
		out = append(out, int(n.R)<<24|int(n.G)<<16|int(n.B)<<8|int(n.A))
	}
	return out
}

func gfInts(v []int) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = fmt.Sprintf("%08x", x)
	}
	return strings.Join(parts, ",")
}

func gfFrame(m *image.Paletted, delay, disposal int) string {
	b := m.Bounds()
	pix := make([]byte, 0, b.Dx()*b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		i := m.PixOffset(b.Min.X, y)
		pix = append(pix, m.Pix[i:i+b.Dx()]...)
	}
	return fmt.Sprintf("|%d,%d,%d,%d,d%d,p%d,[%s],%s", b.Min.X, b.Min.Y, b.Dx(), b.Dy(), delay, disposal, gfInts(gfPalInts(m.Palette)), hex.EncodeToString(pix))
}

// gfSummaries is Go's DecodeAll, Decode and DecodeConfig of data as the
// gif corpus writes them.
func gfSummaries(data []byte) (all, first, conf string) {
	if g, err := gif.DecodeAll(bytes.NewReader(data)); err != nil {
		all = "ERR " + err.Error()
	} else {
		var gp []int
		if p, ok := g.Config.ColorModel.(color.Palette); ok {
			gp = gfPalInts(p)
		}
		all = fmt.Sprintf("%dx%d loop=%d bg=%d gpal=[%s] frames=%d", g.Config.Width, g.Config.Height, g.LoopCount, g.BackgroundIndex, gfInts(gp), len(g.Image))
		for i, m := range g.Image {
			all += gfFrame(m, g.Delay[i], int(g.Disposal[i]))
		}
	}
	if im, err := gif.Decode(bytes.NewReader(data)); err != nil {
		first = "ERR " + err.Error()
	} else {
		first = gfFrame(im.(*image.Paletted), 0, 0)
	}
	if c, err := gif.DecodeConfig(bytes.NewReader(data)); err != nil {
		conf = "ERR " + err.Error()
	} else {
		var gp []int
		if p, ok := c.ColorModel.(color.Palette); ok {
			gp = gfPalInts(p)
		}
		conf = fmt.Sprintf("%dx%d gpal=[%s]", c.Width, c.Height, gfInts(gp))
	}
	return
}

func gfPalette(r *rand.Rand) color.Palette {
	n := []int{1, 2, 3, 4, 7, 16, 17, 255, 256}[r.Intn(9)]
	if r.Intn(2) == 0 {
		n = 1 + r.Intn(8)
	}
	p := make(color.Palette, n)
	for i := range p {
		p[i] = color.RGBA{uint8(r.Intn(256)), uint8(r.Intn(256)), uint8(r.Intn(256)), 255}
	}
	if r.Intn(3) == 0 {
		p[r.Intn(n)] = color.RGBA{}
	}
	return p
}

// gfStream is EncodeAll of a random animation, or nil when Go refuses it.
func gfStream(r *rand.Rand) []byte {
	sw, sh := 1+r.Intn(16), 1+r.Intn(16)
	var global color.Palette
	if r.Intn(2) == 0 {
		global = gfPalette(r)
	}
	g := &gif.GIF{LoopCount: []int{-1, 0, 1, 5, 65535}[r.Intn(5)], Config: image.Config{Width: sw, Height: sh}}
	if global != nil {
		g.Config.ColorModel = global
		g.BackgroundIndex = byte(r.Intn(len(global)))
	}
	disposal := r.Intn(2) == 0
	for f, n := 0, 1+r.Intn(3); f < n; f++ {
		p := global
		if p == nil || r.Intn(3) == 0 {
			p = gfPalette(r)
		}
		x, y := r.Intn(sw), r.Intn(sh)
		w, h := 1+r.Intn(min(12, sw-x)), 1+r.Intn(min(12, sh-y))
		m := image.NewPaletted(image.Rect(x, y, x+w, y+h), p)
		for i := range m.Pix {
			if i > 0 && r.Intn(3) == 0 {
				m.Pix[i] = m.Pix[i-1]
			} else {
				m.Pix[i] = uint8(r.Intn(len(p)))
			}
		}
		g.Image = append(g.Image, m)
		g.Delay = append(g.Delay, []int{0, 1, 10, 100, 65535}[r.Intn(5)])
		if disposal {
			g.Disposal = append(g.Disposal, byte(r.Intn(4)))
		}
	}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, g); err != nil {
		return nil
	}
	return b.Bytes()
}

// gfMutate changes d in one to three ways and says how.
func gfMutate(r *rand.Rand, d []byte) ([]byte, string) {
	c := append([]byte(nil), d...)
	var how []string
	for k, n := 0, 1+r.Intn(3); k < n && len(c) > 0; k++ {
		i := r.Intn(len(c))
		switch r.Intn(6) {
		case 0:
			c[i] ^= byte(1 << uint(r.Intn(8)))
			how = append(how, fmt.Sprintf("flip %d", i))
		case 1:
			c[i] = byte(r.Intn(256))
			how = append(how, fmt.Sprintf("set %d", i))
		case 2:
			c = c[:i]
			how = append(how, fmt.Sprintf("cut %d", i))
		case 3:
			// a byte after the header region: often a block length or a
			// descriptor field
			if i < 13 {
				i = 13 + r.Intn(max(1, len(c)-13))
			}
			if i < len(c) {
				c[i] = []byte{0, 1, 2, 0x21, 0x2c, 0x3b, 0xff}[r.Intn(7)]
				how = append(how, fmt.Sprintf("mark %d", i))
			}
		case 4:
			ins := make([]byte, 1+r.Intn(4))
			r.Read(ins)
			c = append(c[:i], append(ins, c[i:]...)...)
			how = append(how, fmt.Sprintf("insert %d", i))
		default:
			// the screen size or the packed fields of the header
			j := 6 + r.Intn(min(7, max(1, len(c)-6)))
			if j < len(c) {
				c[j] = byte(r.Intn(256))
				how = append(how, fmt.Sprintf("header %d", j))
			}
		}
	}
	return c, strings.Join(how, " ")
}

// gfFixed are versions Go quotes with its own escapes (strconv.Quote):
// std/gif wrote \x07 for \a and escaped printable UTF-8 (found by seed 7,
// 2026-11-15).
func gfFixed() []gfCase {
	var cs []gfCase
	for _, v := range []string{"\x07IF89a", "GIF\x08\x0c\x0b", "GIF\xc3\xa9a", "GIF\xff\x7f\x00", "GI\"\\\t\n", "GIF87b"} {
		d := append([]byte(v), 1, 0, 1, 0, 0, 0, 0, 0x3b)
		c := gfCase{data: d, how: "version " + fmt.Sprintf("%q", v)}
		c.all, c.first, c.conf = gfSummaries(d)
		cs = append(cs, c)
	}
	return cs
}

// gfHostile is the k-th hostile GIF (task #352): a 2 by 2 frame written by
// Go, then its screen width and height, its frame's left and top, or its
// frame's width and height set to a hostile 16 bit length (0xffff, the top
// bit). Frame bounds past a small screen are an error before any pixels
// are allocated, so no case makes a 4 GB frame. Built without a draw, so
// the random stream is unchanged.
func gfHostile(k int) gfCase {
	m := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
	m.Pix = []byte{0, 1, 1, 0}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{m}, Delay: []int{0}}); err != nil {
		panic(err)
	}
	d := b.Bytes()
	ls := hostile.Lengths(16)
	v := uint16(ls[(k/3)%len(ls)])
	put := func(i int) { d[i], d[i+1] = byte(v), byte(v>>8) }
	// the image descriptor: after the header, the global palette and any
	// extension blocks
	at := 13
	if d[10]&0x80 != 0 {
		at += 3 << (d[10]&7 + 1)
	}
	for d[at] == 0x21 {
		at += 2
		for d[at] != 0 {
			at += int(d[at]) + 1
		}
		at++
	}
	if d[at] != 0x2c {
		panic("gfHostile: no image descriptor")
	}
	var how string
	switch k % 3 {
	case 0:
		put(6)
		put(8)
		how = fmt.Sprintf("hostile screen %d", v)
	case 1:
		put(at + 1)
		put(at + 3)
		how = fmt.Sprintf("hostile frame offset %d", v)
	default:
		put(at + 5)
		put(at + 7)
		how = fmt.Sprintf("hostile frame size %d", v)
	}
	c := gfCase{data: d, how: how}
	c.all, c.first, c.conf = gfSummaries(d)
	return c
}

// gfCases: with hostileOn, every twenty-fifth case is gfHostile's.
func gfCases(r *rand.Rand, n int, hostileOn bool) []gfCase {
	cs := make([]gfCase, 0, n)
	for len(cs) < n {
		d := gfStream(r)
		if d == nil {
			continue
		}
		how := "as written"
		if r.Intn(4) != 0 {
			d, how = gfMutate(r, d)
		}
		if hostileOn && len(cs)%25 == 24 {
			cs = append(cs, gfHostile(len(cs)/25))
			continue
		}
		c := gfCase{data: d, how: how}
		c.all, c.first, c.conf = gfSummaries(d)
		cs = append(cs, c)
	}
	return cs
}

const gfCheckXo = `use std/gif
use std/image

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
        s = s + frame_text(f, g.delay[i], if g.disposal.is_empty() { 0 } else { g.disposal[i] })
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

test "std/gif decodes written and mutated GIFs as Go's image/gif" {
  var bad: List[Str] = []
  for (h, how, all, first, cfg) in CASES {
    let d = Bytes.from_hex(h).or_fault("hex")
    let got = "${all_text(d)}\n  ${first_text(d)}\n  ${cfg_text(d)}"
    let want = "${all}\n  ${first}\n  ${cfg}"
    if got != want {
      bad.push("${how}: ${h}\n  got  ${got}\n  want ${want}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${CASES.len()} inputs decode differently, first: ${bad[0]}")
  }
}
`

func gfXoFile(seed int64, cases []gfCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/giffuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// image/gif; do not edit: XO_GIFFUZZ_WRITE=1 go test ./internal/stdtest -run TestGifFuzzCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(gfCheckXo)
	b.WriteString("\n/// (input hex, how it was made, Go's DecodeAll, Decode and DecodeConfig summaries)\nconst CASES: List[(Str, Str, Str, Str, Str)] = [\n")
	for _, c := range cases {
		fmt.Fprintf(&b, "  (%q, %q, %q, %q, %q),\n", hex.EncodeToString(c.data), c.how, c.all, c.first, c.conf)
	}
	b.WriteString("]\n")
	return b.String()
}

// gfCoverage fails when the cases stop reaching both decoded and refused
// streams, several frames, and Go's main error kinds.
func gfCoverage(t *testing.T, cases []gfCase) {
	t.Helper()
	ok, multi, errs := 0, 0, map[string]int{}
	for _, c := range cases {
		if strings.HasPrefix(c.all, "ERR ") {
			msg := strings.TrimPrefix(c.all, "ERR ")
			if i := strings.IndexAny(msg, "\"0123456789"); i > 0 {
				msg = msg[:i]
			}
			errs[msg]++
			continue
		}
		ok++
		if !strings.Contains(c.all, "frames=1|") {
			multi++
		}
	}
	n := len(cases)
	if ok < n/4 || multi < n/20 || len(errs) < 5 {
		t.Fatalf("coverage fell: %d of %d decode (%d with several frames); %d error kinds: %v", ok, n, multi, len(errs), errs)
	}
}

const gfCorpus = "testdata/corpus/giffuzz/giffuzz_test.xo"

// TestGifFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestGifFuzzCorpusCurrent(t *testing.T) {
	cases := append(gfFixed(), gfCases(rand.New(rand.NewSource(1)), 300, false)...)
	gfCoverage(t, cases)
	bxCorpusCurrent(t, gfCorpus, "XO_GIFFUZZ_WRITE", gfXoFile(1, cases))
}

// TestGifFuzz runs a fresh seed's cases.
func TestGifFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the gif fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_GIFFUZZ_SEED", 2))
	n := envInt(t, "XO_GIFFUZZ_N", 1000)
	cases := gfCases(rand.New(rand.NewSource(seed)), n, seed != 1)
	runFuzzDir(t, "giffuzz_test.xo", gfXoFile(seed, cases), envBackends("XO_GIFFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
