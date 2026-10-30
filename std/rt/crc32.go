package rt

import "hash/crc32"

// std/crc32 on the Go backend: Go's hash/crc32 itself; std/native/crc32.xo
// ports it.

var crc32Castagnoli = crc32.MakeTable(crc32.Castagnoli)

func crc32Table(p Crc32_Poly) *crc32.Table {
	if p.Tag == 2 { // Castagnoli (tags count from 1)
		return crc32Castagnoli
	}
	return crc32.IEEETable
}

// Crc32_checksum is `crc32.checksum(data, poly)`.
func Crc32_checksum(_ *Task, data Bytes, poly Crc32_Poly) int64 {
	return int64(crc32.Checksum([]byte(data), crc32Table(poly)))
}

// Crc32_update is `crc32.update(crc, data, poly)`.
func Crc32_update(_ *Task, crc int64, data Bytes, poly Crc32_Poly) int64 {
	return int64(crc32.Update(uint32(crc), crc32Table(poly), []byte(data)))
}
