package main

import (
	"encoding/binary"
	"syscall"
)

// physMem is sysctl hw.memsize, as std/debug's mem_total on macOS.
func physMem() int64 {
	s, err := syscall.Sysctl("hw.memsize")
	if err != nil {
		return 0
	}
	b := []byte(s)
	for len(b) < 8 {
		b = append(b, 0)
	}
	return int64(binary.LittleEndian.Uint64(b))
}
