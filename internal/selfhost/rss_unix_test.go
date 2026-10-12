//go:build unix

package selfhost

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// maxRSS is the peak resident size of a finished process in bytes, or 0
// when the system does not report it (ru_maxrss is bytes on darwin,
// kilobytes on linux and the BSDs).
func maxRSS(ps *os.ProcessState) int64 {
	if ps == nil {
		return 0
	}
	ru, ok := ps.SysUsage().(*syscall.Rusage)
	if !ok {
		return 0
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
		return int64(ru.Maxrss)
	}
	return int64(ru.Maxrss) << 10
}

// totalMemory is the physical memory of the host in bytes, or 0 when
// unknown (darwin: sysctl hw.memsize; linux: MemTotal of /proc/meminfo).
func totalMemory() int64 {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0
		}
		n, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		return n
	case "linux":
		f, err := os.Open("/proc/meminfo")
		if err != nil {
			return 0
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			// MemTotal:       65011712 kB
			if fs := strings.Fields(sc.Text()); len(fs) >= 2 && fs[0] == "MemTotal:" {
				n, _ := strconv.ParseInt(fs[1], 10, 64)
				return n << 10
			}
		}
	}
	return 0
}
