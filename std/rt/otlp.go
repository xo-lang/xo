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
	case Otlp_ExportErr_RequestTooLarge:
		return "request body too large: exceeded " + strconv.FormatInt(p.F_limit, 10) + " bytes"
	case Otlp_ExportErr_ResponseTooLarge:
		return "response body too large: exceeded " + strconv.FormatInt(p.F_limit, 10) + " bytes"
	case Otlp_ExportErr_Partial:
		msg := p.F_msg
		if msg == "" {
			msg = "empty message"
		}
		return "OTLP partial success: " + msg + " (" + strconv.FormatInt(p.F_rejected, 10) + " " + p.F_kind + " rejected)"
	case Otlp_ExportErr_Grpc:
		status := "rpc error: code = " + grpcCodeName(p.F_code) + " desc = " + p.F_message
		if p.F_tries > 1 {
			return "export gave up after " + strconv.FormatInt(p.F_tries, 10) + " tries: " + status
		}
		return status
	}
	return ""
}

// grpcCodeName is grpc-go's codes.Code.String(): the name, or Code(N).
func grpcCodeName(c int64) string {
	names := []string{"OK", "Canceled", "Unknown", "InvalidArgument", "DeadlineExceeded", "NotFound", "AlreadyExists", "PermissionDenied", "ResourceExhausted", "FailedPrecondition", "Aborted", "OutOfRange", "Unimplemented", "Internal", "Unavailable", "DataLoss", "Unauthenticated"}
	if c >= 0 && c < int64(len(names)) {
		return names[c]
	}
	return "Code(" + strconv.FormatInt(c, 10) + ")"
}
