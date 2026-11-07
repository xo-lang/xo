package rt

import "strconv"

// M_display is the text of std/otlp's ExportErr (std/native/otlp.xo exporterr_display).
func (e Otlp_ExportErr) M_display(*Task) string {
	switch p := e.P.(type) {
	case Otlp_ExportErr_Transport:
		return "export: " + p.F_msg
	case Otlp_ExportErr_Rejected:
		return "export rejected: " + strconv.FormatInt(p.F_status, 10) + " " + p.F_body
	case Otlp_ExportErr_GaveUp:
		return "export gave up after " + strconv.FormatInt(p.F_tries, 10) + " tries: " + strconv.FormatInt(p.F_status, 10)
	}
	return ""
}
