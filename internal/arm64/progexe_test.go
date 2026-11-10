package arm64

import "runtime"

// progExe is the -o name the tests build to: on Windows xo adds .exe to an
// -o with no extension (compiler/xo cache.xo), so a test reading "prog"
// back found no file (winarm VM, 2026-11-09).
var progExe = func() string {
	if runtime.GOOS == "windows" {
		return "prog.exe"
	}
	return "prog"
}()
