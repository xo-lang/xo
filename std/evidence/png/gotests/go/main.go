// Command go is the Go side of std/png's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package png's result as text;
// std/evidence/png/gotests/main.xo prints std/png's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
)

func model(im image.Image) (string, []byte) {
	switch m := im.(type) {
	case *image.Gray:
		return "Gray", m.Pix
	case *image.Gray16:
		return "Gray16", m.Pix
	case *image.Paletted:
		return "Paletted", m.Pix
	case *image.RGBA:
		return "RGBA", m.Pix
	case *image.NRGBA:
		return "NRGBA", m.Pix
	case *image.RGBA64:
		return "RGBA64", m.Pix
	case *image.NRGBA64:
		return "NRGBA64", m.Pix
	}
	return fmt.Sprintf("%T", im), nil
}

func decoded(data string) string {
	im, err := png.Decode(strings.NewReader(data))
	if err != nil {
		return "err " + err.Error()
	}
	name, pix := model(im)
	var pal []color.Color
	if p, ok := im.(*image.Paletted); ok {
		pal = p.Palette
	}
	var b strings.Builder
	for _, c := range pal {
		n := color.NRGBAModel.Convert(c).(color.NRGBA)
		fmt.Fprintf(&b, ",%d", uint32(n.R)<<24|uint32(n.G)<<16|uint32(n.B)<<8|uint32(n.A))
	}
	r := im.Bounds()
	return fmt.Sprintf("%s %dx%d %d %d%s", name, r.Dx(), r.Dy(), crc32.ChecksumIEEE(pix), len(pal), b.String())
}

func config(data string) string {
	c, err := png.DecodeConfig(strings.NewReader(data))
	if err != nil {
		return "err " + err.Error()
	}
	var name string
	switch c.ColorModel {
	case color.GrayModel:
		name = "Gray"
	case color.Gray16Model:
		name = "Gray16"
	case color.RGBAModel:
		name = "RGBA"
	case color.NRGBAModel:
		name = "NRGBA"
	case color.RGBA64Model:
		name = "RGBA64"
	case color.NRGBA64Model:
		name = "NRGBA64"
	default:
		if _, ok := c.ColorModel.(color.Palette); ok {
			name = "Paletted"
		}
	}
	return fmt.Sprintf("%s %dx%d", name, c.Width, c.Height)
}

func run(kind string, in []string) string {
	switch kind {
	case "decode":
		return decoded(in[0])
	case "config":
		return config(in[0])
	}
	return "unknown kind " + kind
}

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<24)
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		in := make([]string, 0, len(f)-1)
		for _, s := range f[1:] {
			b, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			in = append(in, string(b))
		}
		fmt.Fprintln(w, base64.StdEncoding.EncodeToString([]byte(run(f[0], in))))
	}
}
