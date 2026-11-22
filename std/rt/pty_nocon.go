//go:build !windows

package rt

import (
	"os"
	"time"
)

// Outside Windows a Pty is a master and slave pair; ptyCon is Windows'
// pseudo console (pty_windows.go) and never made here.

const ptyConOS = false

type ptyCon struct{}

func ptyConOpen() (*ptyCon, *os.File, error)                     { return nil, nil, nil }
func (*ptyCon) read(*Task, []byte, time.Time) (int, bool, error) { return 0, false, nil }
func (*ptyCon) write([]byte) error                               { return nil }
func (*ptyCon) size() Pty_Size                                   { return Pty_Size{} }
func (*ptyCon) resize(Pty_Size) error                            { return nil }
func (*ptyCon) close()                                           {}
func ptyConSpawn(tk *Task, p *ptyFile, c *procCmd, all bool) Result[Child, ProcErr] {
	return Err[Child](procIo(ptyUnsupported))
}
