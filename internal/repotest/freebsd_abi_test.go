package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// freebsdCalls maps each function of std/native/llrt/freebsd.xo that
// makes a system call to the call's name in sys/sys/syscall.h.
var freebsdCalls = map[string]string{
	"write_at":      "write",
	"write":         "write",
	"read":          "read",
	"read_at":       "read",
	"exit":          "exit",
	"mmap":          "mmap",
	"munmap":        "munmap",
	"mprotect":      "mprotect",
	"madvise":       "madvise",
	"getpid":        "getpid",
	"set_fsbase":    "sysarch",
	"clock_gettime": "clock_gettime",
	"nanosleep_at":  "nanosleep",
	"umtx_op":       "_umtx_op",
	"thr_new":       "thr_new",
	"thr_self":      "thr_self",
	"thr_exit":      "thr_exit",
	"thr_kill":      "thr_kill",
	"open":          "openat",
	"close":         "close",
	"fcntl":         "fcntl",
	"pipe2":         "pipe2",
	"kqueue":        "kqueue",
	"kevent":        "kevent",
	"sigaction":     "sigaction",
	"sigprocmask":   "sigprocmask",
	"fsync":         "fsync",
	"stat":          "fstatat",
	"fstat":         "fstat",
	"random_at":     "getrandom",
	"random":        "getrandom",
	"getcwd":        "__getcwd",
	"mkdir":         "mkdirat",
	"unlink":        "unlinkat",
	"rename":        "renameat",
	"symlink":       "symlinkat",
	"readlink":      "readlinkat",
	"chmod":         "fchmodat",
	"utimens":       "utimensat",
	"pread_at":      "pread",
	"pwrite_at":     "pwrite",
	"ftruncate":     "ftruncate",
	"lseek":         "lseek",
	"flock":         "flock",
	"dir_list":      "getdirentries",
}

var (
	freebsdConst = regexp.MustCompile(`(?m)^(?:pub )?const ([A-Z_]+): Int = (-?\d+)$`)
	freebsdFunc  = regexp.MustCompile(`(?m)^(?:pub )?fn ([a-z_0-9]+)\(`)
	freebsdSys6  = regexp.MustCompile(`rt\.sys6\((\d+),`)
)

// TestFreeBSDABI checks std/native/llrt/freebsd.xo against FreeBSD's own
// headers (decision 0573): every constant the file declares, and the
// system call number of every function that makes one, equal
// testdata/freebsd-abi.txt, which scripts/freebsdabi/gen.sh computes with
// clang from a release's checksummed src.txz for amd64 and arm64. Code
// only a FreeBSD kernel can prove (thr_new's and _umtx_op's behavior, the
// poller) waits for the VM lane.
func TestFreeBSDABI(t *testing.T) {
	root := filepath.Join("..", "..")
	abi := map[string][2]int{}
	data, err := os.ReadFile(filepath.Join("testdata", "freebsd-abi.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || strings.HasPrefix(f[0], "#") {
			continue
		}
		a, err1 := strconv.Atoi(f[1])
		b, err2 := strconv.Atoi(f[2])
		if err1 != nil || err2 != nil {
			t.Fatalf("freebsd-abi.txt: %q", line)
		}
		abi[f[0]] = [2]int{a, b}
	}
	srcb, err := os.ReadFile(filepath.Join(root, "std", "native", "llrt", "freebsd.xo"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(srcb)
	consts := 0
	for _, m := range freebsdConst.FindAllStringSubmatch(src, -1) {
		v, _ := strconv.Atoi(m[2])
		want, ok := abi[m[1]]
		if !ok {
			t.Errorf("freebsd.xo const %s: not in freebsd-abi.txt (add it to scripts/freebsdabi/gen.sh's facts)", m[1])
			continue
		}
		if v != want[0] || v != want[1] {
			t.Errorf("freebsd.xo const %s = %d; FreeBSD's headers: %d on amd64, %d on arm64", m[1], v, want[0], want[1])
		}
		consts++
	}
	// each function's body: from its fn line to the next fn line
	fns := freebsdFunc.FindAllStringSubmatchIndex(src, -1)
	calls := 0
	for i, m := range fns {
		name := src[m[2]:m[3]]
		end := len(src)
		if i+1 < len(fns) {
			end = fns[i+1][0]
		}
		s := freebsdSys6.FindStringSubmatch(src[m[0]:end])
		if s == nil {
			continue
		}
		call, ok := freebsdCalls[name]
		if !ok {
			t.Errorf("freebsd.xo fn %s makes system call %s: name its call in freebsdCalls", name, s[1])
			continue
		}
		n, _ := strconv.Atoi(s[1])
		if want := abi["SYS_"+call]; n != want[0] {
			t.Errorf("freebsd.xo fn %s: system call %d; SYS_%s is %d", name, n, call, want[0])
		}
		calls++
	}
	if consts < 30 || calls < 20 {
		t.Errorf("checked %d constants and %d system calls: the patterns no longer match freebsd.xo", consts, calls)
	}
}
