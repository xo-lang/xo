package rt

// std/png on the Go backend: compiled from std/native/png.xo
// (std/go_compiled.txt, decision 0312). The display method stays here, as
// the runtime calls it through interfaces.

// M_display is Go's error text: `png: invalid format: ...`, `png: unsupported
// feature: ...`, `unexpected EOF`, or the zlib reader's own text
// (std/native/png.xo pngerr_display).
func (e Png_PngErr) M_display(*Task) string {
	switch e.Tag {
	case 1:
		return "png: invalid format: " + e.P.(Png_PngErr_Format).F_msg
	case 2:
		return "png: unsupported feature: " + e.P.(Png_PngErr_Unsupported).F_msg
	case 3:
		return "unexpected EOF"
	}
	return e.P.(Png_PngErr_Flate).F_msg
}
