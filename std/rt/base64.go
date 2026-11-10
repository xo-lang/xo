package rt

import (
	"strconv"
)

// The display method of std/base64's error (std.md 21); its functions are
// std/native/base64.xo on the Go backend too (decision 0312). std/base32 is
// merged (decision 0588) and generated as its own package.

func (e Base64_DecodeErr) M_display(*Task) string {
	return "illegal base64 data at input byte " + strconv.FormatInt(e.F_at, 10)
}
