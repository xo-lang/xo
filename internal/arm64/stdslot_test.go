package arm64

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// stdBuildPeak is one std build's peak memory with headroom: `xo dev
// stdcache --conc=0` peaked at 1.39 GB on linux/arm64 (2026-11-06, three
// cold runs; its `xo dev stdobj` child compiles std), and a std build
// runs two such builds at once (its .stdobj-prog/x and .stdobj-prog/xf:
// the full suite's peak on linux/arm64 held ten of
// 1.1 to 1.3 GB for five slots of 2 GiB, 14,976 MB; 2026-11-07).
const stdBuildPeak = 3 << 30

var stdSlots struct {
	once sync.Once
	ch   chan struct{}
}

// stdRun runs a std build (run is an exec.Cmd's Output or CombinedOutput)
// in one of a few slots. The package's parallel tests start about twenty
// std builds; at once they passed a 22 GB container's memory limit, and
// the kernel killed std's object build (`xo dev stdobj exit 137`,
// 2026-11-06, gotchas/parallel-std-builds-oom.md).
func stdRun(run func() ([]byte, error)) ([]byte, error) {
	stdSlots.once.Do(func() { stdSlots.ch = make(chan struct{}, stdBuildSlots()) })
	stdSlots.ch <- struct{}{}
	defer func() { <-stdSlots.ch }()
	return run()
}

// stdBuildSlots is XO_TEST_STD_BUILDS, else as many std builds as half
// the memory limit holds (the other half for the package's other tests),
// at least one.
func stdBuildSlots() int {
	if n, err := strconv.Atoi(os.Getenv("XO_TEST_STD_BUILDS")); err == nil && n > 0 {
		return n
	}
	n := int(memLimit() / 2 / stdBuildPeak)
	if n < 1 {
		n = 1
	}
	return n
}

// memLimit is the memory the tests may use: HEAVY_MAX_GB when set (the
// Mac's heavy.sh stops a job's process group past it; the Mac has no
// cgroup), else the cgroup's memory.max on Linux, else MemTotal; 16 GiB
// (heavy.sh's default cap) where none is read.
func memLimit() int64 {
	if gb, err := strconv.Atoi(os.Getenv("HEAVY_MAX_GB")); err == nil && gb > 0 {
		return int64(gb) << 30
	}
	if runtime.GOOS == "linux" {
		if b, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
			if n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); err == nil && n > 0 {
				return n
			}
		}
		if b, err := os.ReadFile("/proc/meminfo"); err == nil {
			for _, l := range strings.Split(string(b), "\n") {
				if f := strings.Fields(l); len(f) >= 2 && f[0] == "MemTotal:" {
					if kb, err := strconv.ParseInt(f[1], 10, 64); err == nil {
						return kb << 10
					}
				}
			}
		}
	}
	return 16 << 30
}
