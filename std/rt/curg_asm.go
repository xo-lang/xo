//go:build amd64 || arm64

package rt

// curG identifies the running goroutine: the address of its runtime g
// record, read from the g register (arm64) or thread local storage
// (amd64). A g record is reused only after its goroutine exited, so two
// live goroutines never share a value. Go callbacks use it to tell a call
// on the goroutine of the task that passed them from a call on another
// goroutine (gointerop.go).
func curG() uintptr
