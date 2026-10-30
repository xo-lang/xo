//go:build linux

package rt

import "syscall"

// statfs is Fs.disk_usage's statfs(2) (decision 0426): blocks count
// f_frsize bytes (f_bsize before Linux 2.6), as the native path.
func statfs(op string) (DiskUsage, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(op, &st); err != nil {
		return DiskUsage{}, err
	}
	bs := int64(st.Frsize)
	if bs <= 0 {
		bs = int64(st.Bsize)
	}
	return DiskUsage{F_total: int64(st.Blocks) * bs, F_free: int64(st.Bfree) * bs, F_available: int64(st.Bavail) * bs, F_files: int64(st.Files), F_files_free: int64(st.Ffree)}, nil
}
