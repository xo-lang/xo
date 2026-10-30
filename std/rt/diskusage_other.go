//go:build !linux && !darwin && !windows

package rt

import "errors"

// statfs: no disk usage on this target (wasip1, js, plan9, the BSDs for
// now; decision 0426).
func statfs(string) (DiskUsage, error) {
	return DiskUsage{}, errors.New("disk usage is not supported on this system")
}
