//go:build darwin || freebsd || (linux && (amd64 || arm64 || riscv64))

package rt

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"unsafe"
)

// ptyIoctl goes through SyscallConn, not f.Fd(): Fd puts the master in
// blocking mode, and a blocking master's read ignores its deadline
// (linux, where the runtime poller holds it).
func ptyIoctl(f *os.File, req, arg uintptr) error {
	c, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var e syscall.Errno
	if err := c.Control(func(fd uintptr) {
		_, _, e = syscall.Syscall(syscall.SYS_IOCTL, fd, req, arg)
	}); err != nil {
		return err
	}
	if e != 0 {
		return e
	}
	return nil
}

// ptyOpen opens /dev/ptmx (posix_openpt, grantpt, unlockpt, ptsname) and
// the slave, kept open (gotchas/pty-darwin-master-and-ctty.md).
func ptyOpen() (m, s *os.File, path string, err error) {
	m, err = ptyOpenMaster()
	if err != nil {
		return nil, nil, "", err
	}
	if runtime.GOOS == "darwin" {
		var name [128]byte
		if err = ptyIoctl(m, 0x20007454, 0); err == nil { // TIOCPTYGRANT
			if err = ptyIoctl(m, 0x20007452, 0); err == nil { // TIOCPTYUNLK
				err = ptyIoctl(m, 0x40807453, uintptr(unsafe.Pointer(&name[0]))) // TIOCPTYGNAME
			}
		}
		for i, b := range name {
			if b == 0 {
				path = string(name[:i])
				break
			}
		}
	} else if runtime.GOOS == "freebsd" {
		var n uint32
		err = ptyIoctl(m, 0x4004740f, uintptr(unsafe.Pointer(&n))) // TIOCGPTN
		path = "/dev/pts/" + strconv.Itoa(int(n))
	} else {
		var unlock int32
		var n uint32
		if err = ptyIoctl(m, 0x40045431, uintptr(unsafe.Pointer(&unlock))); err == nil { // TIOCSPTLCK
			err = ptyIoctl(m, 0x80045430, uintptr(unsafe.Pointer(&n))) // TIOCGPTN
		}
		path = "/dev/pts/" + strconv.Itoa(int(n))
	}
	if err == nil {
		s, err = ptyOpenSlave(path)
	}
	if err != nil {
		m.Close()
		return nil, nil, "", err
	}
	return m, s, path, nil
}

// ptyOpenSlave opens the slave blocking, outside the runtime poller:
// os.OpenFile would set O_NONBLOCK on the open file description the child
// gets as its streams, and FreeBSD flushes a terminal's unread output when
// a non-blocking descriptor makes its last close, so a child that wrote and
// exited before the master read lost its output
// (gotchas/pty-darwin-master-and-ctty.md).
func ptyOpenSlave(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(fd), path), nil
}

func ptyGetSize(f *os.File) (Pty_Size, error) {
	var ws struct{ row, col, x, y uint16 }
	if err := ptyIoctl(f, syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&ws))); err != nil {
		return Pty_Size{}, err
	}
	return Pty_Size{F_rows: int64(ws.row), F_cols: int64(ws.col), F_x_pixels: int64(ws.x), F_y_pixels: int64(ws.y)}, nil
}

func ptySetSize(f *os.File, s Pty_Size) error {
	ws := struct{ row, col, x, y uint16 }{uint16(s.F_rows), uint16(s.F_cols), uint16(s.F_x_pixels), uint16(s.F_y_pixels)}
	return ptyIoctl(f, syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&ws)))
}

// ptySetRaw sets cfmakeraw's flags (on) or restores saved (off): the
// termios to restore later.
func ptySetRaw(f *os.File, on bool, saved []byte) ([]byte, error) {
	var t syscall.Termios
	size := unsafe.Sizeof(t)
	if !on {
		if saved == nil {
			return nil, nil
		}
		copy(unsafe.Slice((*byte)(unsafe.Pointer(&t)), size), saved)
		return nil, ptyIoctl(f, ioctlSetTermios, uintptr(unsafe.Pointer(&t)))
	}
	if err := ptyIoctl(f, ioctlGetTermios, uintptr(unsafe.Pointer(&t))); err != nil {
		return saved, err
	}
	old := saved
	if old == nil {
		old = append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(&t)), size)...)
	}
	t.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	t.Oflag &^= syscall.OPOST
	t.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	t.Cflag &^= syscall.CSIZE | syscall.PARENB
	t.Cflag |= syscall.CS8
	t.Cc[syscall.VMIN] = 1
	t.Cc[syscall.VTIME] = 0
	if err := ptyIoctl(f, ioctlSetTermios, uintptr(unsafe.Pointer(&t))); err != nil {
		return saved, err
	}
	return old, nil
}

// ptyAttach makes tty the child's controlling terminal in a new session
// (Setsid, Setctty): on its three streams too (all), else as descriptor 3
// (Go's Ctty is a child descriptor).
func ptyAttach(x *exec.Cmd, tty *os.File, all bool) {
	if x.SysProcAttr == nil {
		x.SysProcAttr = &syscall.SysProcAttr{}
	}
	x.SysProcAttr.Setpgid = false
	x.SysProcAttr.Setsid = true
	x.SysProcAttr.Setctty = true
	ptyTraceExec(x.SysProcAttr)
	if all {
		x.Stdin, x.Stdout, x.Stderr = tty, tty, tty
		x.SysProcAttr.Ctty = 0
		return
	}
	x.ExtraFiles = append(x.ExtraFiles, tty)
	x.SysProcAttr.Ctty = 2 + len(x.ExtraFiles)
}

const ptySupported = true
