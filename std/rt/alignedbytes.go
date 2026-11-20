package rt

import "unsafe"

// AlignedBytes is `s.bytes()` on a string literal of 32 bytes or more in
// the Go backend (task #467): a copy whose data starts on a 32-byte
// boundary, made once at package init. A Go string literal lands at any
// offset in rodata, so a vector loop over it (bytealg.IndexByte loads
// aligned 32-byte blocks) runs a partial head block on every call.
func AlignedBytes(s string) Bytes {
	if len(s) == 0 {
		return ""
	}
	buf := make([]byte, len(s)+31)
	off := int(-uintptr(unsafe.Pointer(unsafe.SliceData(buf))) & 31)
	copy(buf[off:], s)
	return Bytes(unsafe.String(&buf[off], len(s)))
}
