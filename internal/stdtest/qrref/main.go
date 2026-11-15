// Command qrref is the oracle of std/otp's Key.image (scripts/stdcorpus
// otp): pquerna/otp's Key.Image, boombuler/barcode's qr.Encode at level M
// in Auto mode scaled by barcode.Scale. Each stdin line is
// "URI<TAB>WIDTH<TAB>HEIGHT"; each output line is the image's size and its
// pixels, 1 dark and 0 light, row by row, or "ERR msg". A content
// net/url refuses runs Key.Image's body (qr.Encode, barcode.Scale) directly.
package main

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"os"
	"strconv"
	"strings"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"github.com/pquerna/otp"
)

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<20)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	for in.Scan() {
		f := strings.Split(in.Text(), "\t")
		w, _ := strconv.Atoi(f[1])
		h, _ := strconv.Atoi(f[2])
		var img image.Image
		k, err := otp.NewKeyFromURL(f[0])
		if err == nil {
			img, err = k.Image(w, h)
		} else {
			// a content net/url refuses (std/otp's Key holds any uri):
			// Key.Image's own body
			var b barcode.Barcode
			if b, err = qr.Encode(f[0], qr.M, qr.Auto); err == nil {
				img, err = barcode.Scale(b, w, h)
			}
		}
		if err != nil {
			fmt.Fprintf(out, "ERR %v\n", err)
			continue
		}
		b := img.Bounds()
		var sb strings.Builder
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if color.GrayModel.Convert(img.At(x, y)).(color.Gray).Y < 128 {
					sb.WriteByte('1')
				} else {
					sb.WriteByte('0')
				}
			}
		}
		fmt.Fprintf(out, "%d %d %s\n", b.Dx(), b.Dy(), sb.String())
	}
}
