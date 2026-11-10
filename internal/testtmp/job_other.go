//go:build !windows

package testtmp

// killChildrenWithProcess is Windows' job object (job_windows.go); unix
// children end through Guard's lifeline.
func killChildrenWithProcess() {}
