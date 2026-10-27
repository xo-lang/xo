package rt

import (
	"math"
	"strconv"
)

// std/uuid on the Go backend (decision 0282): compiled from
// std/native/uuid.xo since decision 0312 (std/go_compiled.txt). The
// display methods stay here, as the runtime calls them through
// interfaces.

func (e Uuid_UuidErr) M_display(*Task) string { return e.F_msg }

// M_debug is `Uuid("<text>")`, as when a Uuid was its text.
func (u Uuid_Uuid) M_debug(t *Task) string { return "Uuid(" + strconv.Quote(u.M_display(t)) + ")" }

// M_display is the canonical text: the stored words have their top bit
// flipped (std/native/uuid.xo TOP).
func (u Uuid_Uuid) M_display(t *Task) string {
	return Internal_uuid_text_words(t, u.F_hi^math.MinInt64, u.F_lo^math.MinInt64)
}
