package rt

import "net/mail"

// std/mail (std.md 25, decision 0282): Go's net/mail addresses, as
// std/native/mail.xo ports them.

func (e Mail_MailErr) M_display(*Task) string { return e.F_msg }

func (a Mail_Address) M_display(*Task) string {
	return (&mail.Address{Name: a.F_name, Address: a.F_address}).String()
}

func Mail_parse_address(_ *Task, s string) Result[Mail_Address, Mail_MailErr] {
	a, err := mail.ParseAddress(s)
	if err != nil {
		return Err[Mail_Address](Mail_MailErr{F_msg: err.Error()})
	}
	return Ok[Mail_Address, Mail_MailErr](Mail_Address{F_name: textOf(a.Name), F_address: textOf(a.Address)})
}

func Mail_parse_address_list(_ *Task, s string) Result[List[Mail_Address], Mail_MailErr] {
	as, err := mail.ParseAddressList(s)
	if err != nil {
		return Err[List[Mail_Address]](Mail_MailErr{F_msg: err.Error()})
	}
	out := make([]Mail_Address, len(as))
	for i, a := range as {
		out[i] = Mail_Address{F_name: textOf(a.Name), F_address: textOf(a.Address)}
	}
	return Ok[List[Mail_Address], Mail_MailErr](ListFromSlice(out))
}
