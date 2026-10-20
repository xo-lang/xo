package rt

import (
	"encoding/base32"
	"encoding/base64"
	"strconv"
)

// std/base64 and std/base32 (std.md 21, decision 0279): Go's encodings.
// Go returns the bytes decoded before an error; Xo returns the error
// alone, as the native backends do.

var base64Encodings = [...]*base64.Encoding{nil, base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding}

var base32Encodings = [...]*base32.Encoding{nil, base32.StdEncoding, base32.HexEncoding,
	base32.StdEncoding.WithPadding(base32.NoPadding), base32.HexEncoding.WithPadding(base32.NoPadding)}

func (e Base64_DecodeErr) M_display(*Task) string {
	return "illegal base64 data at input byte " + strconv.FormatInt(e.F_at, 10)
}

func (e Base32_DecodeErr) M_display(*Task) string {
	return "illegal base32 data at input byte " + strconv.FormatInt(e.F_at, 10)
}

func Base64_encode(_ *Task, data Bytes, enc Base64_Base64) string {
	return base64Encodings[enc.Tag].EncodeToString([]byte(data))
}

func Base64_decode(_ *Task, s string, enc Base64_Base64) Result[Bytes, Base64_DecodeErr] {
	b, err := base64Encodings[enc.Tag].DecodeString(s)
	if err != nil {
		return Err[Bytes](Base64_DecodeErr{F_at: int64(err.(base64.CorruptInputError))})
	}
	return Ok[Bytes, Base64_DecodeErr](Bytes(b))
}

func Base32_encode(_ *Task, data Bytes, enc Base32_Base32) string {
	return base32Encodings[enc.Tag].EncodeToString([]byte(data))
}

func Base32_decode(_ *Task, s string, enc Base32_Base32) Result[Bytes, Base32_DecodeErr] {
	b, err := base32Encodings[enc.Tag].DecodeString(s)
	if err != nil {
		return Err[Bytes](Base32_DecodeErr{F_at: int64(err.(base32.CorruptInputError))})
	}
	return Ok[Bytes, Base32_DecodeErr](Bytes(b))
}
