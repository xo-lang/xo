//go:build !windows && !darwin && !linux && !freebsd && !netbsd && !openbsd && !dragonfly

package rt

import "os"

// No process usage on this target (wasip1, js, plan9): zeros (decision
// 0426).
func selfUsage() ProcUsage       { return ProcUsage{} }
func childrenUsage() ProcUsage   { return ProcUsage{} }
func noteChild(*os.ProcessState) {}
