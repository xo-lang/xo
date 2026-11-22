//go:build !(darwin || freebsd || (linux && (amd64 || arm64 || riscv64)))

package rt

import (
	"errors"
	"os"
	"os/exec"
	"time"
)

var errPty = errors.New(ptyUnsupported)

func ptyOpen() (m, s *os.File, path string, err error) { return nil, nil, "", errPty }
func ptyOpenSlave(string) (*os.File, error)            { return nil, errPty }
func ptyGetSize(*os.File) (Pty_Size, error)            { return Pty_Size{}, errPty }
func ptySetSize(*os.File, Pty_Size) error              { return errPty }
func ptySetRaw(*os.File, bool, []byte) ([]byte, error) { return nil, errPty }
func ptyAttach(*exec.Cmd, *os.File, bool)              {}

const ptySupported = false

func ptySetDeadline(*os.File, time.Time) {}

func ptyRelease(s *os.File, _ int) { s.Close() }

func ptyRead(*Task, *os.File, []byte, time.Time) (int, bool, error) { return 0, false, errPty }
