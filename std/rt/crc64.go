package rt

import "hash/crc64"

// std/crc64's update on the Go backend (decision 0597 part 2: declared
// bodyless in std/crc64.xo, the native backends run
// std/native/crc64_native.xo's): Go's hash/crc64 itself. checksum is
// generated from std/crc64.xo.

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

// Crc64_update is `crc64.update(crc, data, poly)`.
func Crc64_update(_ *Task, crc int64, data Bytes, poly Crc64_Poly) int64 {
	return int64(crc64.Update(uint64(crc), crc64Table(poly), []byte(data)))
}
