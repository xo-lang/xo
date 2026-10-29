package rt

// std/rate on the Go backend: compiled from std/native/rate.xo
// (std/go_compiled.txt, decision 0312). The display method stays here,
// as the runtime calls it through interfaces.

import (
	"math"
	"strconv"
)

// Rate_INF is rate.INF, which programs compiled for the Go backend name.
const Rate_INF = math.MaxFloat64

// M_display is `rate: Wait(n=N) exceeds limiter's burst B` or
// `rate: canceled` (std/native/rate.xo rateerr_display).
func (e Rate_RateErr) M_display(*Task) string {
	if a, ok := e.P.(Rate_RateErr_Exceeds); ok && e.Tag == 1 {
		return "rate: Wait(n=" + strconv.FormatInt(a.F_n, 10) + ") exceeds limiter's burst " + strconv.FormatInt(a.F_burst, 10)
	}
	return "rate: canceled"
}
