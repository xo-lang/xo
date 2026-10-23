package rt

import "strconv"

// Plain error messages (decision 0358): the same texts as the native
// functions (httperr_display and the others in std/native/http.xo, tls.xo,
// smtp.xo, template.xo). Variants without fields are told by Tag
// (declaration order, from 1).

// M_display is `e.display()`.
func (v Http_HttpErr) M_display(*Task) string {
	if p, ok := v.P.(Http_HttpErr_Io); ok {
		return p.F_msg
	}
	return "canceled"
}

// M_display is `e.display()`.
func (v Http_ClientErr) M_display(*Task) string {
	switch p := v.P.(type) {
	case Http_ClientErr_InvalidUrl:
		return "invalid URL: " + p.F_msg
	case Http_ClientErr_Connect:
		return p.F_msg
	case Http_ClientErr_Protocol:
		return "protocol error: " + p.F_msg
	}
	if v.Tag == 3 {
		return "request timed out"
	}
	return "canceled"
}

// M_display is `e.display()`.
func (v Http_UrlErr) M_display(*Task) string { return v.F_msg }

// M_display is `e.display()`.
func (v Http_CookieErr) M_display(*Task) string { return v.F_msg }

// M_display is `e.display()`.
func (v Http_TlsErr) M_display(*Task) string { return v.F_msg }

// M_display is `e.display()`.
func (v Tls_TlsErr) M_display(*Task) string { return v.F_msg }

// M_display is `e.display()`.
func (v Template_TemplateErr) M_display(*Task) string { return v.F_msg }

// M_display is `e.display()`.
func (v Smtp_SmtpErr) M_display(*Task) string {
	if v.F_code == 0 {
		return v.F_msg
	}
	return strconv.FormatInt(v.F_code, 10) + " " + v.F_msg
}
