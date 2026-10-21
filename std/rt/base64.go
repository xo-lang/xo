package rt

import (
	"strconv"
)

// The display methods of std/base64 and std/base32's errors (std.md 21);
// both modules' functions are std/native/base64.xo and base32.xo on the
// Go backend too (decision 0312).

func (e Base64_DecodeErr) M_display(*Task) string {
	return "illegal base64 data at input byte " + strconv.FormatInt(e.F_at, 10)
}

func (e Base32_DecodeErr) M_display(*Task) string {
	return "illegal base32 data at input byte " + strconv.FormatInt(e.F_at, 10)
}
