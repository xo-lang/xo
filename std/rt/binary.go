package rt

import "encoding/binary"

// std/binary (std.md 22, decision 0279): Go's encoding/binary byte
// orders and varints on Bytes. A read past the end faults with the
// native backends' text.

func binaryPut(size int, put func([]byte)) Bytes {
	b := make([]byte, size)
	put(b)
	return Bytes(b)
}

func binaryAt(b Bytes, at int64, size int) []byte {
	if at < 0 || at+int64(size) > int64(len(b)) {
		FaultHere("binary: %d bytes at %d out of range for Bytes of length %d", size, at, len(b))
	}
	return []byte(b[at : at+int64(size)])
}

func Binary_u16_be(_ *Task, n int64) Bytes {
	return binaryPut(2, func(b []byte) { binary.BigEndian.PutUint16(b, uint16(n)) })
}
func Binary_u32_be(_ *Task, n int64) Bytes {
	return binaryPut(4, func(b []byte) { binary.BigEndian.PutUint32(b, uint32(n)) })
}
func Binary_u64_be(_ *Task, n int64) Bytes {
	return binaryPut(8, func(b []byte) { binary.BigEndian.PutUint64(b, uint64(n)) })
}
func Binary_u16_le(_ *Task, n int64) Bytes {
	return binaryPut(2, func(b []byte) { binary.LittleEndian.PutUint16(b, uint16(n)) })
}
func Binary_u32_le(_ *Task, n int64) Bytes {
	return binaryPut(4, func(b []byte) { binary.LittleEndian.PutUint32(b, uint32(n)) })
}
func Binary_u64_le(_ *Task, n int64) Bytes {
	return binaryPut(8, func(b []byte) { binary.LittleEndian.PutUint64(b, uint64(n)) })
}

func Binary_get_u16_be(_ *Task, b Bytes, at int64) int64 {
	return int64(binary.BigEndian.Uint16(binaryAt(b, at, 2)))
}
func Binary_get_u32_be(_ *Task, b Bytes, at int64) int64 {
	return int64(binary.BigEndian.Uint32(binaryAt(b, at, 4)))
}
func Binary_get_u64_be(_ *Task, b Bytes, at int64) int64 {
	return int64(binary.BigEndian.Uint64(binaryAt(b, at, 8)))
}
func Binary_get_u16_le(_ *Task, b Bytes, at int64) int64 {
	return int64(binary.LittleEndian.Uint16(binaryAt(b, at, 2)))
}
func Binary_get_u32_le(_ *Task, b Bytes, at int64) int64 {
	return int64(binary.LittleEndian.Uint32(binaryAt(b, at, 4)))
}
func Binary_get_u64_le(_ *Task, b Bytes, at int64) int64 {
	return int64(binary.LittleEndian.Uint64(binaryAt(b, at, 8)))
}
func Binary_get_i16_be(_ *Task, b Bytes, at int64) int64 {
	return int64(int16(binary.BigEndian.Uint16(binaryAt(b, at, 2))))
}
func Binary_get_i32_be(_ *Task, b Bytes, at int64) int64 {
	return int64(int32(binary.BigEndian.Uint32(binaryAt(b, at, 4))))
}
func Binary_get_i16_le(_ *Task, b Bytes, at int64) int64 {
	return int64(int16(binary.LittleEndian.Uint16(binaryAt(b, at, 2))))
}
func Binary_get_i32_le(_ *Task, b Bytes, at int64) int64 {
	return int64(int32(binary.LittleEndian.Uint32(binaryAt(b, at, 4))))
}

func Binary_uvarint(_ *Task, n int64) Bytes { return Bytes(binary.AppendUvarint(nil, uint64(n))) }
func Binary_varint(_ *Task, n int64) Bytes  { return Bytes(binary.AppendVarint(nil, n)) }

func binaryVarintAt(b Bytes, at int64) []byte {
	if at < 0 || at > int64(len(b)) {
		return nil
	}
	return []byte(b[at:])
}

func Binary_get_uvarint(_ *Task, b Bytes, at int64) Option[Tuple2[int64, int64]] {
	v, n := binary.Uvarint(binaryVarintAt(b, at))
	if n <= 0 {
		return None[Tuple2[int64, int64]]()
	}
	return Some(Tuple2[int64, int64]{int64(v), int64(n)})
}

func Binary_get_varint(_ *Task, b Bytes, at int64) Option[Tuple2[int64, int64]] {
	v, n := binary.Varint(binaryVarintAt(b, at))
	if n <= 0 {
		return None[Tuple2[int64, int64]]()
	}
	return Some(Tuple2[int64, int64]{v, int64(n)})
}
