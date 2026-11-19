// Command go is the Go side of std/jpeg's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package jpeg's result as text;
// std/evidence/jpeg/gotests/main.xo prints std/jpeg's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"strconv"
	"strings"
)

var subs = map[image.YCbCrSubsampleRatio]string{
	image.YCbCrSubsampleRatio444: "S444",
	image.YCbCrSubsampleRatio440: "S440",
	image.YCbCrSubsampleRatio422: "S422",
	image.YCbCrSubsampleRatio420: "S420",
	image.YCbCrSubsampleRatio411: "S411",
	image.YCbCrSubsampleRatio410: "S410",
}

// rows appends h rows of n bytes from p, stride apart.
func rows(b, p []byte, n, h, stride int) []byte {
	for y := 0; y < h; y++ {
		b = append(b, p[y*stride:y*stride+n]...)
	}
	return b
}

func print(im image.Image) string {
	r := im.Bounds()
	w, h := r.Dx(), r.Dy()
	var b []byte
	name := ""
	switch m := im.(type) {
	case *image.Gray:
		name, b = "Gray", rows(b, m.Pix, w, h, m.Stride)
	case *image.CMYK:
		name, b = "CMYK", rows(b, m.Pix, 4*w, h, m.Stride)
	case *image.RGBA:
		name, b = "RGBA", rows(b, m.Pix, 4*w, h, m.Stride)
	case *image.YCbCr:
		cw, ch := w, h
		switch m.SubsampleRatio {
		case image.YCbCrSubsampleRatio422:
			cw = (w + 1) / 2
		case image.YCbCrSubsampleRatio420:
			cw, ch = (w+1)/2, (h+1)/2
		case image.YCbCrSubsampleRatio440:
			ch = (h + 1) / 2
		case image.YCbCrSubsampleRatio411:
			cw = (w + 3) / 4
		case image.YCbCrSubsampleRatio410:
			cw, ch = (w+3)/4, (h+1)/2
		}
		name = "YCbCr " + subs[m.SubsampleRatio]
		b = rows(b, m.Y, w, h, m.YStride)
		b = rows(b, m.Cb, cw, ch, m.CStride)
		b = rows(b, m.Cr, cw, ch, m.CStride)
	default:
		return fmt.Sprintf("%T", im)
	}
	return fmt.Sprintf("%s %dx%d %d", name, w, h, crc32.ChecksumIEEE(b))
}

func decoded(data string) string {
	im, err := jpeg.Decode(strings.NewReader(data))
	if err != nil {
		return "err " + err.Error()
	}
	return print(im)
}

func encoded(data string, quality int) string {
	p, err := png.Decode(strings.NewReader(data))
	if err != nil {
		return "err " + err.Error()
	}
	if _, ok := p.(*image.RGBA); !ok {
		return fmt.Sprintf("png model %T", p)
	}
	var w bytes.Buffer
	if err := jpeg.Encode(&w, p, &jpeg.Options{Quality: quality}); err != nil {
		return "err " + err.Error()
	}
	return fmt.Sprintf("%d %d %s", w.Len(), crc32.ChecksumIEEE(w.Bytes()), decoded(w.String()))
}

func run(kind string, in []string) string {
	switch kind {
	case "decode":
		return decoded(in[0])
	case "encode":
		q, _ := strconv.Atoi(in[1])
		return encoded(in[0], q)
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
