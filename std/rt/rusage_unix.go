//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package rt

import (
	"os"
	"runtime"
	"syscall"
)

// selfUsage and childrenUsage are Proc.usage and children_usage
// (decision 0426): getrusage(2), ru_maxrss in bytes (KiB on every Unix
// but macOS).
func selfUsage() ProcUsage     { return rusage(syscall.RUSAGE_SELF) }
func childrenUsage() ProcUsage { return rusage(syscall.RUSAGE_CHILDREN) }

func rusage(who int) ProcUsage {
	var ru syscall.Rusage
	if syscall.Getrusage(who, &ru) != nil {
		return ProcUsage{}
	}
	rss := int64(ru.Maxrss)
	if runtime.GOOS != "darwin" {
		rss *= 1024
	}
	return ProcUsage{F_user: Duration(ru.Utime.Nano()), F_system: Duration(ru.Stime.Nano()), F_max_rss: rss}
}

// noteChild is a waited child's own usage (decision 0426's addendum:
// ProcOutput.usage, Child.wait_usage), from its wait4 rusage; the kernel
// keeps the children's totals on Unix. Zeros when it never started.
func noteChild(ps *os.ProcessState) ProcUsage {
	if ps == nil {
		return ProcUsage{}
	}
	ru, ok := ps.SysUsage().(*syscall.Rusage)
	if !ok || ru == nil {
		return ProcUsage{}
	}
	rss := int64(ru.Maxrss)
	if runtime.GOOS != "darwin" {
		rss *= 1024
	}
	return ProcUsage{F_user: Duration(ru.Utime.Nano()), F_system: Duration(ru.Stime.Nano()), F_max_rss: rss}
}
