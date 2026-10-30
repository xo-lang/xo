package rt

import (
	"encoding/binary"
	"hash"
	"hash/fnv"
)

// std/fnv on the Go backend: Go's hash/fnv itself; std/native/fnv.xo
// ports it. An update starts from a sum by restoring the hash's state
// (MarshalBinary's layout: a 4 byte magic, then the state big endian).

func fnvSum(h hash.Hash, data Bytes) []byte {
	h.Write([]byte(data))
	return h.Sum(nil)
}

// fnvFrom is h with its state set to sum (magic "fnv" and a kind byte,
// as fnv.go's marshal writes them).
func fnvFrom(h hash.Hash, kind byte, sum []byte) hash.Hash {
	state := append([]byte{'f', 'n', 'v', kind}, sum...)
	if err := h.(interface{ UnmarshalBinary([]byte) error }).UnmarshalBinary(state); err != nil {
		panic(err)
	}
	return h
}

func be32(n int64) []byte { return binary.BigEndian.AppendUint32(nil, uint32(n)) }
func be64(n int64) []byte { return binary.BigEndian.AppendUint64(nil, uint64(n)) }

func Fnv_sum32(_ *Task, data Bytes) int64 {
	return int64(binary.BigEndian.Uint32(fnvSum(fnv.New32(), data)))
}
func Fnv_sum32a(_ *Task, data Bytes) int64 {
	return int64(binary.BigEndian.Uint32(fnvSum(fnv.New32a(), data)))
}
func Fnv_sum64(_ *Task, data Bytes) int64 {
	return int64(binary.BigEndian.Uint64(fnvSum(fnv.New64(), data)))
}
func Fnv_sum64a(_ *Task, data Bytes) int64 {
	return int64(binary.BigEndian.Uint64(fnvSum(fnv.New64a(), data)))
}
func Fnv_sum128(_ *Task, data Bytes) Bytes  { return Bytes(fnvSum(fnv.New128(), data)) }
func Fnv_sum128a(_ *Task, data Bytes) Bytes { return Bytes(fnvSum(fnv.New128a(), data)) }

func Fnv_update32(_ *Task, sum int64, data Bytes) int64 {
	return int64(binary.BigEndian.Uint32(fnvSum(fnvFrom(fnv.New32(), 1, be32(sum)), data)))
}
func Fnv_update32a(_ *Task, sum int64, data Bytes) int64 {
	return int64(binary.BigEndian.Uint32(fnvSum(fnvFrom(fnv.New32a(), 2, be32(sum)), data)))
}
func Fnv_update64(_ *Task, sum int64, data Bytes) int64 {
	return int64(binary.BigEndian.Uint64(fnvSum(fnvFrom(fnv.New64(), 3, be64(sum)), data)))
}
func Fnv_update64a(_ *Task, sum int64, data Bytes) int64 {
	return int64(binary.BigEndian.Uint64(fnvSum(fnvFrom(fnv.New64a(), 4, be64(sum)), data)))
}

func fnvCheck16(name string, sum Bytes) {
	if len(sum) != 16 {
		FaultHere("fnv.%s: the sum is %d bytes, want 16", name, len(sum))
	}
}

func Fnv_update128(_ *Task, sum, data Bytes) Bytes {
	fnvCheck16("update128", sum)
	return Bytes(fnvSum(fnvFrom(fnv.New128(), 5, []byte(sum)), data))
}
func Fnv_update128a(_ *Task, sum, data Bytes) Bytes {
	fnvCheck16("update128a", sum)
	return Bytes(fnvSum(fnvFrom(fnv.New128a(), 6, []byte(sum)), data))
}
