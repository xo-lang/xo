package rt

// std/uuid on the Go backend (decision 0282): compiled from
// std/native/uuid.xo since decision 0312 (std/go_compiled.txt). The
// display methods stay here, as the runtime calls them through
// interfaces.

func (e Uuid_UuidErr) M_display(*Task) string { return e.F_msg }

func (u Uuid_Uuid) M_display(*Task) string { return u.F0 }
