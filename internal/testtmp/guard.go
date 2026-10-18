package testtmp

import (
	"os"
	"os/exec"
	"runtime"
	"sync"
)

// Guard makes cmd, which the caller starts (it must have no ExtraFiles),
// a child that cannot outlive this test process, for runners that have no
// testing.T at hand: it runs behind the lifeline wrapper
// (internal/testtmp/lifeline) on one lifeline per process, whose write
// end only this process holds and never closes, so it ends only when the
// process does, a `go test -timeout` panic and a kill included, which run
// no deferred cleanup. Then the program's process group gets SIGTERM and,
// after a grace period, SIGKILL (four programs that ignore SIGTERM ran 27
// hours after their test, 2026-10-18). cmd.Process is still the program
// itself. Where the wrapper cannot be built, and on Windows, cmd is
// returned as it is. Lifeline is the per test form, which also ends the
// child with its test.
func Guard(cmd *exec.Cmd) *exec.Cmd {
	if runtime.GOOS == "windows" || len(cmd.ExtraFiles) != 0 {
		return cmd
	}
	r := processLifeline()
	if r == nil {
		return cmd
	}
	wrapper, err := lifelineBinary()
	if err != nil {
		return cmd
	}
	cmd.Args = append([]string{wrapper, cmd.Path}, cmd.Args[1:]...)
	cmd.Path = wrapper
	cmd.ExtraFiles = []*os.File{r}
	return cmd
}

var procLife struct {
	once sync.Once
	r, w *os.File
}

// processLifeline is the read end of the process's lifeline (nil when the
// pipe cannot be made); the write end stays open for the process's life.
func processLifeline() *os.File {
	procLife.once.Do(func() {
		r, w, err := os.Pipe()
		if err == nil {
			procLife.r, procLife.w = r, w
		}
	})
	return procLife.r
}
