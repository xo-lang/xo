package rt

import "strconv"

// std/charset on the Go backend: compiled from std/native/charset.xo
// (std/go_compiled.txt, decision 0312). The display methods stay here, as
// the runtime calls them through interfaces.

// M_display is `charset: MSG at byte OFFSET` (std/native/charset.xo
// charseterr_display).
func (e Charset_CharsetErr) M_display(*Task) string {
	return "charset: " + e.F_msg + " at byte " + strconv.FormatInt(e.F_offset, 10)
}
