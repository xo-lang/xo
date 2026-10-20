package rt

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// std/uuid (std.md 24, decision 0282): RFC 9562 version 4 and 7 UUIDs
// and parsing, following github.com/google/uuid v1.6.0 (BSD-3;
// THIRD_PARTY_NOTICES), as std/native/uuid.xo does: Parse's forms and
// error texts; v7 without google/uuid's process wide counter.

func (e Uuid_UuidErr) M_display(*Task) string { return e.F_msg }

func uuidText(b []byte) Uuid_Uuid {
	h := hex.EncodeToString(b)
	return Uuid_Uuid{F0: h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]}
}

func uuidBits(b []byte, version byte) []byte {
	b[6] = b[6]&0x0f | version<<4
	b[8] = b[8]&0x3f | 0x80
	return b
}

func Uuid_v4(tk *Task, r Rand) Uuid_Uuid {
	return uuidText(uuidBits([]byte(r.M_bytes(tk, 16)), 4))
}

func Uuid_v7(tk *Task, r Rand, c Clock) Uuid_Uuid {
	ms := Time_unix_millis(tk, c.M_now(tk))
	b := make([]byte, 16)
	for k := 0; k < 6; k++ {
		b[k] = byte(ms >> (40 - 8*k))
	}
	copy(b[6:], []byte(r.M_bytes(tk, 10)))
	return uuidText(uuidBits(b, 7))
}

func uuidXtob(a, b byte) (byte, bool) {
	v, err := hex.DecodeString(string([]byte{a, b}))
	if err != nil {
		return 0, false
	}
	return v[0], true
}

func uuidFail(msg string) Result[Uuid_Uuid, Uuid_UuidErr] {
	return Err[Uuid_Uuid](Uuid_UuidErr{F_msg: msg})
}

func Uuid_parse(_ *Task, s string) Result[Uuid_Uuid, Uuid_UuidErr] {
	var out [16]byte
	switch len(s) {
	case 36:
	case 36 + 9:
		if !strings.EqualFold(s[:9], "urn:uuid:") {
			return uuidFail(fmt.Sprintf("invalid urn prefix: %q", s[:9]))
		}
		s = s[9:]
	case 36 + 2:
		s = s[1:]
	case 32:
		for i := range out {
			v, ok := uuidXtob(s[i*2], s[i*2+1])
			if !ok {
				return uuidFail("invalid UUID format")
			}
			out[i] = v
		}
		return Ok[Uuid_Uuid, Uuid_UuidErr](uuidText(out[:]))
	default:
		return uuidFail(fmt.Sprintf("invalid UUID length: %d", len(s)))
	}
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return uuidFail("invalid UUID format")
	}
	for i, x := range [16]int{0, 2, 4, 6, 9, 11, 14, 16, 19, 21, 24, 26, 28, 30, 32, 34} {
		v, ok := uuidXtob(s[x], s[x+1])
		if !ok {
			return uuidFail("invalid UUID format")
		}
		out[i] = v
	}
	return Ok[Uuid_Uuid, Uuid_UuidErr](uuidText(out[:]))
}

func Uuid_from_bytes(_ *Task, b Bytes) Result[Uuid_Uuid, Uuid_UuidErr] {
	if len(b) != 16 {
		return uuidFail(fmt.Sprintf("invalid UUID (got %d bytes)", len(b)))
	}
	return Ok[Uuid_Uuid, Uuid_UuidErr](uuidText([]byte(b)))
}

func Uuid_nil(*Task) Uuid_Uuid { return Uuid_Uuid{F0: "00000000-0000-0000-0000-000000000000"} }

func (u Uuid_Uuid) M_bytes(*Task) Bytes {
	b, _ := hex.DecodeString(strings.ReplaceAll(u.F0, "-", ""))
	return Bytes(b)
}

func (u Uuid_Uuid) M_version(*Task) int64 {
	v, _ := hex.DecodeString("0" + u.F0[14:15])
	return int64(v[0])
}

func (u Uuid_Uuid) M_display(*Task) string { return u.F0 }
