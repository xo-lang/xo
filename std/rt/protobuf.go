package rt

import "strconv"

// std/protobuf on the Go backend: compiled from std/native/protobuf.xo
// (std/go_compiled.txt, decision 0312). The display method stays here, as
// the runtime calls it through interfaces; without it an error printed as
// its variant's name (`Recursion`) where the native backends print
// protowire's text (std/native/protobuf.xo protoerr_display).

// M_display is the error text, protowire's for each case.
func (e Protobuf_ProtoErr) M_display(*Task) string {
	switch e.Tag {
	case 1:
		return "proto: unexpected EOF"
	case 2:
		return "proto: variable length integer overflow"
	case 3:
		return "proto: invalid field number"
	case 4:
		return "proto: invalid wire type " + strconv.FormatInt(e.P.(Protobuf_ProtoErr_BadWire).F_wire, 10)
	case 5:
		return "proto: unexpected end group"
	case 6:
		return "proto: exceeded maximum recursion depth"
	case 7:
		return "proto: invalid length"
	}
	return "proto: string field contains invalid UTF-8"
}
