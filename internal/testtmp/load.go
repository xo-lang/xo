package testtmp

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Load1 is the host's one minute load average (`sysctl -n vm.loadavg` on
// darwin, /proc/loadavg on Linux), and false where it cannot be read.
// Timing tests use it to tell a broken bound from a loaded machine.
func Load1() (float64, bool) {
	var s string
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
		if err != nil {
			return 0, false
		}
		s = strings.Trim(strings.TrimSpace(string(out)), "{} ")
	case "linux":
		b, err := os.ReadFile("/proc/loadavg")
		if err != nil {
			return 0, false
		}
		s = string(b)
	default:
		return 0, false
	}
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0, false
	}
	v, err := strconv.ParseFloat(f[0], 64)
	return v, err == nil
}

// Loaded reports whether the host's one minute load is above its CPU
// count, with the load: a timing bound measured then says little.
func Loaded() (bool, float64) {
	l, ok := Load1()
	return ok && l > float64(runtime.NumCPU()), l
}
