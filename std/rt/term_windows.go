//go:build windows

package rt

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleMode       = kernel32.NewProc("SetConsoleMode")
	procGetConsoleScreenInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
)

const enableEchoInput = 0x4

func isTerminal(f *os.File) bool {
	var mode uint32
	return syscall.GetConsoleMode(syscall.Handle(f.Fd()), &mode) == nil
}

// termWidth is the columns of f's console window, or 0.
func termWidth(f *os.File) int {
	var info struct {
		size, cursor          [2]int16
		attrs                 uint16
		left, top, right, bot int16
		max                   [2]int16
	}
	r, _, _ := procGetConsoleScreenInfo.Call(f.Fd(), uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return 0
	}
	return int(info.right-info.left) + 1
}

// echoOff turns the console's echo off; nil when f is not a console.
func echoOff(f *os.File) func() {
	h := syscall.Handle(f.Fd())
	var mode uint32
	if syscall.GetConsoleMode(h, &mode) != nil {
		return nil
	}
	if r, _, _ := procSetConsoleMode.Call(uintptr(h), uintptr(mode&^enableEchoInput)); r == 0 {
		return nil
	}
	return func() { procSetConsoleMode.Call(uintptr(h), uintptr(mode)) }
}
