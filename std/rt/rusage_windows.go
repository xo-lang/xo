//go:build windows

package rt

import (
	"os"
	"sync"
	"syscall"
	"unsafe"
)

var procK32GetProcessMemoryInfo = kernel32.NewProc("K32GetProcessMemoryInfo")

// childTotals is what Windows does not keep (Unix's RUSAGE_CHILDREN):
// the waited children's user and kernel time, added by noteChild. Their
// peak working set is gone with the process handle os/exec closed, so
// max_rss stays 0 here (the native runtime reads it before closing).
var childTotals struct {
	sync.Mutex
	u ProcUsage
}

func filetimeNs(f syscall.Filetime) int64 {
	return (int64(f.HighDateTime)<<32 | int64(f.LowDateTime)) * 100
}

// selfUsage is Proc.usage (decision 0426): GetProcessTimes and the peak
// working set of K32GetProcessMemoryInfo.
func selfUsage() ProcUsage {
	h, _ := syscall.GetCurrentProcess()
	var c, e, k, u syscall.Filetime
	var out ProcUsage
	if syscall.GetProcessTimes(h, &c, &e, &k, &u) == nil {
		out.F_user = Duration(filetimeNs(u))
		out.F_system = Duration(filetimeNs(k))
	}
	// PROCESS_MEMORY_COUNTERS: cb, PageFaultCount, PeakWorkingSetSize, ...
	var pmc [9]uint64
	pmc[0] = uint64(unsafe.Sizeof(pmc))
	if r, _, _ := procK32GetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&pmc[0])), unsafe.Sizeof(pmc)); r != 0 {
		out.F_max_rss = int64(pmc[1])
	}
	return out
}

func childrenUsage() ProcUsage {
	childTotals.Lock()
	defer childTotals.Unlock()
	return childTotals.u
}

// noteChild adds a waited child's times (nil: it never started).
func noteChild(ps *os.ProcessState) {
	if ps == nil {
		return
	}
	ru, ok := ps.SysUsage().(*syscall.Rusage)
	if !ok || ru == nil {
		return
	}
	childTotals.Lock()
	childTotals.u.F_user += Duration(filetimeNs(ru.UserTime))
	childTotals.u.F_system += Duration(filetimeNs(ru.KernelTime))
	childTotals.Unlock()
}
