package rt

// M_display is the message of x/net/idna's error (std/native/idna.xo
// idnaerr_display, which the Go backend compiles for the other functions).
func (e Idna_IdnaErr) M_display(*Task) string { return e.F_msg }
