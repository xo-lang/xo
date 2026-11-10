package rt

import "math"

// The integer types' bounds, the prelude's type constants Int.max,
// Int.min, ... (decision 0587).
const (
	INT_MAX    int64  = math.MaxInt64
	INT_MIN    int64  = math.MinInt64
	INT8_MAX   int8   = math.MaxInt8
	INT8_MIN   int8   = math.MinInt8
	INT16_MAX  int16  = math.MaxInt16
	INT16_MIN  int16  = math.MinInt16
	INT32_MAX  int32  = math.MaxInt32
	INT32_MIN  int32  = math.MinInt32
	INT64_MAX  int64  = math.MaxInt64
	INT64_MIN  int64  = math.MinInt64
	UINT8_MAX  uint8  = math.MaxUint8
	UINT8_MIN  uint8  = 0
	UINT16_MAX uint16 = math.MaxUint16
	UINT16_MIN uint16 = 0
	UINT32_MAX uint32 = math.MaxUint32
	UINT32_MIN uint32 = 0
	UINT64_MAX uint64 = math.MaxUint64
	UINT64_MIN uint64 = 0
)

// Float's type constants Float.inf, Float.pi, ... (decision 0592): Go
// variables, not constants, since Go folds constant expressions exactly
// (`Float.max * 2.0` is a Go compile error, +Inf in Xo) and has no
// infinite or NaN constant.
var (
	FLOAT_INF              = math.Inf(1)
	FLOAT_NAN              = math.NaN()
	FLOAT_PI               = math.Pi
	FLOAT_E                = math.E
	FLOAT_MAX              = math.MaxFloat64
	FLOAT_MIN              = -math.MaxFloat64
	FLOAT_SMALLEST_NONZERO = math.SmallestNonzeroFloat64
)

// Rune's type constants Rune.max and Rune.max_ascii (decision 0595).
const (
	RUNE_MAX       Rune = 0x10FFFF
	RUNE_MAX_ASCII Rune = 0x7F
)
