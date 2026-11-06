package rt

// Status.display of std/grpc (decision 0535): grpc-go's error text.
func (v Grpc_Status) M_display(t *Task) string {
	return "rpc error: code = " + v.F_code.M_display(t) + " desc = " + v.F_message
}
