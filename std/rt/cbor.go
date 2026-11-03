package rt

// std/cbor on the Go backend: compiled from std/native/cbor.xo
// (std/go_compiled.txt, decision 0312). The display method stays here, as
// the runtime calls it through interfaces.

// M_display is the error text: `cbor: unexpected EOF`, or `cbor: ` and the
// message (std/native/cbor.xo cborerr_display).
func (e Cbor_CborErr) M_display(*Task) string {
	switch e.Tag {
	case 1:
		return "cbor: unexpected EOF"
	case 2:
		return "cbor: " + e.P.(Cbor_CborErr_Syntax).F_msg
	case 3:
		return "cbor: " + e.P.(Cbor_CborErr_Invalid).F_msg
	}
	return "cbor: " + e.P.(Cbor_CborErr_Limit).F_msg
}
