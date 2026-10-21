package rt

import "net/mail"

// The display methods of std/mail's values (std.md 25): the runtime calls
// them through interfaces, so they stay Go methods; every function is
// std/native/mail.xo on the Go backend too (decision 0312).

func (e Mail_MailErr) M_display(*Task) string { return e.F_msg }

func (a Mail_Address) M_display(*Task) string {
	return (&mail.Address{Name: a.F_name, Address: a.F_address}).String()
}
