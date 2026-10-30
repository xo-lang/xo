//go:build windows

package rt

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var procGetDiskFreeSpaceExW = kernel32.NewProc("GetDiskFreeSpaceExW")

// statfs is Fs.disk_usage's GetDiskFreeSpaceExW (decision 0426), of a
// file's directory, as the call takes only directories; Windows has no
// inode counts.
func statfs(op string) (DiskUsage, error) {
	fi, err := os.Stat(op)
	if err != nil {
		return DiskUsage{}, err
	}
	dir := op
	if !fi.IsDir() {
		dir = filepath.Dir(op)
	}
	p, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return DiskUsage{}, err
	}
	var avail, total, free uint64
	r, _, e := procGetDiskFreeSpaceExW.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&avail)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&free)))
	if r == 0 {
		return DiskUsage{}, &os.PathError{Op: "statfs", Path: op, Err: e}
	}
	return DiskUsage{F_total: int64(total), F_free: int64(free), F_available: int64(avail)}, nil
}
