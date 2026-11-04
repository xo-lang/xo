package rt

import "hash/crc64"

// std/crc64 on the Go backend: Go's hash/crc64 itself; std/native/crc64.xo
// ports it.

var (
	crc64Iso  = crc64.MakeTable(crc64.ISO)
	crc64Ecma = crc64.MakeTable(crc64.ECMA)
	crc64Nvme = crc64.MakeTable(0x9a6c9329ac4bc9b5)
)

func crc64Table(p Crc64_Poly) *crc64.Table {
	if p.Tag == 2 { // Ecma (tags count from 1)
		return crc64Ecma
	}
	if p.Tag == 3 { // Nvme
		return crc64Nvme
	}
	return crc64Iso
}

// Crc64_checksum is `crc64.checksum(data, poly)`.
func Crc64_checksum(_ *Task, data Bytes, poly Crc64_Poly) int64 {
	return int64(crc64.Checksum([]byte(data), crc64Table(poly)))
}

// Crc64_update is `crc64.update(crc, data, poly)`.
func Crc64_update(_ *Task, crc int64, data Bytes, poly Crc64_Poly) int64 {
	return int64(crc64.Update(uint64(crc), crc64Table(poly), []byte(data)))
}
