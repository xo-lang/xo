package rt

// Grpc_pure1 is std/grpc's pure1 (decision 0703): the Go backend has no
// effects at run time, so the function is returned as it is.
func Grpc_pure1[A, R any](_ *Task, f func(*Task, A) Result[R, Never]) func(*Task, A) Result[R, Never] {
	return f
}
