package rt

// std/jpeg on the Go backend: compiled from std/native/jpeg.xo
// (std/go_compiled.txt, decision 0312). The display method stays here, as
// the runtime calls it through interfaces.

// M_display is Go's error text: `invalid JPEG format: ...`, `unsupported
// JPEG feature: ...`, an encode error's own text or `unexpected EOF` (std/native/jpeg.xo jpegerr_display).
func (e Jpeg_JpegErr) M_display(*Task) string {
	switch e.Tag {
	case 1:
		return "invalid JPEG format: " + e.P.(Jpeg_JpegErr_Format).F_msg
	case 2:
		return "unsupported JPEG feature: " + e.P.(Jpeg_JpegErr_Unsupported).F_msg
	case 4:
		return e.P.(Jpeg_JpegErr_Encode).F_msg
	}
	return "unexpected EOF"
}
