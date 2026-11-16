package main

import "syscall"

// physMem is sysinfo(2)'s totalram times its unit, as std/debug's
// mem_total on Linux.
func physMem() int64 {
	var si syscall.Sysinfo_t
	if syscall.Sysinfo(&si) != nil {
		return 0
	}
	u := int64(si.Unit)
	if u == 0 {
		u = 1
	}
	return int64(si.Totalram) * u
}
