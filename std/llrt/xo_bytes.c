// xo_bytes.c: the fault of an index out of range on Bytes for the native
// backends (decisions 0079 and 0081), included by xo_std.c. Sized
// integers, Rune, Bytes, and the builders are Xo (std/native/llrt
// num.xo, ints.xo, raw.xo, obj.xo, consts.xo); the C versions of their
// intrinsics and the seed's entries went with the 0.12.0 seed.

// xo_fault_bytes_index is `b[i]` out of range on Bytes (std/rt BytesAt).
void xo_x_fault_raise_bytes_index(int64_t i, int64_t n, int64_t loc);
XO_NORETURN void xo_fault_bytes_index(int64_t i, int64_t n, const char *loc) {
	xo_x_fault_raise_bytes_index(i, n, XO_ADDR(loc));
	__builtin_unreachable();
}
