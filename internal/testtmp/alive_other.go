//go:build !unix

package testtmp

// alive reports true where the check is not implemented, so a sweep
// never removes a directory it cannot prove abandoned.
func alive(pid int) bool { return true }
