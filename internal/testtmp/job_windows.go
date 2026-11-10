//go:build windows

package testtmp

import (
	"sync"
	"syscall"
	"unsafe"
)

// Windows has no process group a dead test's children leave with, and
// Guard's lifeline wrapper is unix only: a winarm run's timeout ended
// xo.test.exe and left nine xo.exe children spinning for seven hours
// (2026-11-10, gotchas/windows-test-children-outlive-timeout.md). So the
// test process puts itself in a job object that kills every process in it
// when its last handle closes; the handle is never closed, so that is
// when this process ends, by a timeout, a kill or a normal exit. Children
// started later are in the job too (Windows 8 and later nest jobs).

var jobOnce sync.Once

// every test binary that imports testtmp (the commands its tests start
// without Guard too)
func init() { killChildrenWithProcess() }

type basicLimit struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type ioCounters struct {
	ReadOperationCount, WriteOperationCount, OtherOperationCount uint64
	ReadTransferCount, WriteTransferCount, OtherTransferCount    uint64
}

type extendedLimit struct {
	Basic                 basicLimit
	IO                    ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

const (
	jobObjectExtendedLimitInformation = 9
	jobObjectLimitKillOnJobClose      = 0x2000
)

// killChildrenWithProcess puts this process in a kill-on-close job, once;
// a failure leaves things as they were.
func killChildrenWithProcess() {
	jobOnce.Do(func() {
		k := syscall.NewLazyDLL("kernel32.dll")
		create := k.NewProc("CreateJobObjectW")
		set := k.NewProc("SetInformationJobObject")
		assign := k.NewProc("AssignProcessToJobObject")
		cur := k.NewProc("GetCurrentProcess")
		job, _, _ := create.Call(0, 0)
		if job == 0 {
			return
		}
		var info extendedLimit
		info.Basic.LimitFlags = jobObjectLimitKillOnJobClose
		if r, _, _ := set.Call(job, jobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info)); r == 0 {
			return
		}
		self, _, _ := cur.Call()
		assign.Call(job, self)
	})
}
