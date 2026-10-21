package rt

import "math"

// Checked narrowing conversions `T(x)` of integers (core 3.2), one
// non-generic function per target type and source signedness: rt.Conv is
// generic, and Go does not inline it (a call for every `UInt8(x)`; 5% of
// std/path's clean on the Go backend, 2026-10-21). The source is widened
// to int64 (signed) or uint64 (unsigned) by the caller, which costs
// nothing; the fault is an inline panic of an *arithFault (op 'c' for a
// signed source, 'C' for an unsigned one, b the target's index in
// convNames), so each inlines. compiler/gen emits these for a conversion
// that can fail (widens is false) between Go integer types.

// convNames are the Xo names of the targets, by arithFault.b.
var convNames = [...]string{"Int8", "Int16", "Int32", "Int", "UInt8", "UInt16", "UInt32", "UInt64"}

// ConvS_int8 is `Int8(x)` of a signed integer x.
func ConvS_int8(x int64, at string) int8 {
	if x < math.MinInt8 || x > math.MaxInt8 {
		panic(&arithFault{x, 0, 'c', at})
	}
	return int8(x)
}

// ConvU_int8 is `Int8(x)` of an unsigned integer x.
func ConvU_int8(x uint64, at string) int8 {
	if x > math.MaxInt8 {
		panic(&arithFault{int64(x), 0, 'C', at})
	}
	return int8(x)
}

// ConvS_int16 is `Int16(x)` of a signed integer x.
func ConvS_int16(x int64, at string) int16 {
	if x < math.MinInt16 || x > math.MaxInt16 {
		panic(&arithFault{x, 1, 'c', at})
	}
	return int16(x)
}

// ConvU_int16 is `Int16(x)` of an unsigned integer x.
func ConvU_int16(x uint64, at string) int16 {
	if x > math.MaxInt16 {
		panic(&arithFault{int64(x), 1, 'C', at})
	}
	return int16(x)
}

// ConvS_int32 is `Int32(x)` of a signed integer x.
func ConvS_int32(x int64, at string) int32 {
	if x < math.MinInt32 || x > math.MaxInt32 {
		panic(&arithFault{x, 2, 'c', at})
	}
	return int32(x)
}

// ConvU_int32 is `Int32(x)` of an unsigned integer x.
func ConvU_int32(x uint64, at string) int32 {
	if x > math.MaxInt32 {
		panic(&arithFault{int64(x), 2, 'C', at})
	}
	return int32(x)
}

// ConvU_int64 is `Int(x)` of an unsigned integer x.
func ConvU_int64(x uint64, at string) int64 {
	if x > math.MaxInt64 {
		panic(&arithFault{int64(x), 3, 'C', at})
	}
	return int64(x)
}

// ConvS_uint8 is `UInt8(x)` of a signed integer x.
func ConvS_uint8(x int64, at string) uint8 {
	if x < 0 || x > math.MaxUint8 {
		panic(&arithFault{x, 4, 'c', at})
	}
	return uint8(x)
}

// ConvU_uint8 is `UInt8(x)` of an unsigned integer x.
func ConvU_uint8(x uint64, at string) uint8 {
	if x > math.MaxUint8 {
		panic(&arithFault{int64(x), 4, 'C', at})
	}
	return uint8(x)
}

// ConvS_uint16 is `UInt16(x)` of a signed integer x.
func ConvS_uint16(x int64, at string) uint16 {
	if x < 0 || x > math.MaxUint16 {
		panic(&arithFault{x, 5, 'c', at})
	}
	return uint16(x)
}

// ConvU_uint16 is `UInt16(x)` of an unsigned integer x.
func ConvU_uint16(x uint64, at string) uint16 {
	if x > math.MaxUint16 {
		panic(&arithFault{int64(x), 5, 'C', at})
	}
	return uint16(x)
}

// ConvS_uint32 is `UInt32(x)` of a signed integer x.
func ConvS_uint32(x int64, at string) uint32 {
	if x < 0 || x > math.MaxUint32 {
		panic(&arithFault{x, 6, 'c', at})
	}
	return uint32(x)
}

// ConvU_uint32 is `UInt32(x)` of an unsigned integer x.
func ConvU_uint32(x uint64, at string) uint32 {
	if x > math.MaxUint32 {
		panic(&arithFault{int64(x), 6, 'C', at})
	}
	return uint32(x)
}

// ConvS_uint64 is `UInt64(x)` of a signed integer x.
func ConvS_uint64(x int64, at string) uint64 {
	if x < 0 {
		panic(&arithFault{x, 7, 'c', at})
	}
	return uint64(x)
}
