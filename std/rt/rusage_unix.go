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

// noteChild: the kernel keeps the waited children's usage on Unix.
func noteChild(*os.ProcessState) {}
