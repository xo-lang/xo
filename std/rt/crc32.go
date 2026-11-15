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

// Crc32_table is `crc32.table(poly)`: Go's MakeTable, its entries as a List.
func Crc32_table(_ *Task, poly int64) Crc32_Table {
	if poly < 0 || poly > 0xffffffff {
		FaultHere("crc32: polynomial %d is not 32 bits", poly)
	}
	t := crc32.MakeTable(uint32(poly))
	e := make([]int64, len(t))
	for i, v := range t {
		e[i] = int64(v)
	}
	return Crc32_Table{F_poly: poly, F_entries: ListFromSlice(e)}
}

// crc32TableOf is t as Go's Table: Go's own for IEEE and Castagnoli (their
// fast paths), else the entries.
func crc32TableOf(t Crc32_Table) *crc32.Table {
	switch t.F_poly {
	case crc32.IEEE:
		return crc32.IEEETable
	case crc32.Castagnoli:
		return crc32Castagnoli
	}
	var tab crc32.Table
	for i, v := range List_Slice(t.F_entries) {
		tab[i] = uint32(v)
	}
	return &tab
}

// Crc32_checksum_table is `crc32.checksum_table(data, t)`.
func Crc32_checksum_table(_ *Task, data Bytes, t Crc32_Table) int64 {
	return int64(crc32.Checksum([]byte(data), crc32TableOf(t)))
}

// Crc32_update_table is `crc32.update_table(crc, data, t)`.
func Crc32_update_table(_ *Task, crc int64, data Bytes, t Crc32_Table) int64 {
	return int64(crc32.Update(uint32(crc), crc32TableOf(t), []byte(data)))
}
