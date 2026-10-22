//go:build windows

// Command pdbframes prints the traceback of a native Xo program's crash
// report (decision 0343) from the program's PDB alone (decision 0344),
// through Windows' own PDB reader, dbghelp.dll: each frame's function,
// file and line, the calls inlined at a pc first, as the runtime's
// traceback prints them, ending at main. scripts/ci/windows-pdb.sh
// compares its output with the report's own traceback, which the runtime
// names from the DWARF.
//
//	pdbframes [-locals] PROG.exe < REPORT
//
// $PDBFRAMES_DBGHELP names the dbghelp.dll to load (the Windows SDK
// debugger's, beside cdb.exe), else Windows' own. On standard error: the
// dbghelp loaded, the module's symbol type and PDB (SymGetModuleInfoW64),
// and the first failed lookup's error.
//
// With -locals each physical frame's variables follow it, as dbghelp
// finds them at the frame's pc (SymSetContext, SymEnumSymbols): `      NAME
// (param) reg R` for a register, `[reg R+OFF]` for memory.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

var (
	dbghelp                  = syscall.NewLazyDLL(dbghelpPath())
	symSetOptions            = dbghelp.NewProc("SymSetOptions")
	symInitializeW           = dbghelp.NewProc("SymInitializeW")
	symLoadModuleExW         = dbghelp.NewProc("SymLoadModuleExW")
	symFromAddrW             = dbghelp.NewProc("SymFromAddrW")
	symGetLineFromAddrW64    = dbghelp.NewProc("SymGetLineFromAddrW64")
	symAddrIncludeInline     = dbghelp.NewProc("SymAddrIncludeInlineTrace")
	symQueryInlineTrace      = dbghelp.NewProc("SymQueryInlineTrace")
	symFromInlineContextW    = dbghelp.NewProc("SymFromInlineContextW")
	symGetLineFromInlineCtxW = dbghelp.NewProc("SymGetLineFromInlineContextW")
	symSetContext            = dbghelp.NewProc("SymSetContext")
	symEnumSymbolsW          = dbghelp.NewProc("SymEnumSymbolsW")
	symGetModuleInfoW64      = dbghelp.NewProc("SymGetModuleInfoW64")
	symUnloadModule64        = dbghelp.NewProc("SymUnloadModule64")
)

// imageSize is the PE image's SizeOfImage (0 when unreadable).
func imageSize(exe string) uint32 {
	b, err := os.ReadFile(exe)
	if err != nil || len(b) < 64 {
		return 0
	}
	pe := int(*(*uint32)(unsafe.Pointer(&b[60])))
	if pe+24+60 > len(b) {
		return 0
	}
	return *(*uint32)(unsafe.Pointer(&b[pe+24+56]))
}

// failed: whether a lookup's error was printed already
var failed bool

func dbghelpPath() string {
	if p := os.Getenv("PDBFRAMES_DBGHELP"); p != "" {
		return p
	}
	return "dbghelp.dll"
}

var locals bool

// the preferred base Xo's PE linker gives every image
const base = 0x140000000

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "-locals" {
		locals = true
		args = args[1:]
	}
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: pdbframes [-locals] PROG.exe < REPORT")
		os.Exit(2)
	}
	exe, _ := filepath.Abs(args[0])
	loaded, pcs, err := report(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pdbframes:", err)
		os.Exit(1)
	}
	h := uintptr(0x584f) // any value identifies the session
	// SYMOPT_UNDNAME | SYMOPT_LOAD_LINES | SYMOPT_FAIL_CRITICAL_ERRORS |
	// SYMOPT_NO_PROMPTS (not deferred: the PDB loads, or fails, here)
	symSetOptions.Call(0x2 | 0x10 | 0x200 | 0x80000)
	dir, _ := syscall.UTF16PtrFromString(filepath.Dir(exe))
	if r, _, e := symInitializeW.Call(h, uintptr(unsafe.Pointer(dir)), 0); r == 0 {
		fmt.Fprintln(os.Stderr, "pdbframes: SymInitializeW:", e)
		os.Exit(1)
	}
	img, _ := syscall.UTF16PtrFromString(exe)
	if r, _, e := symLoadModuleExW.Call(h, 0, uintptr(unsafe.Pointer(img)), 0, base, 0, 0, 0); r == 0 {
		fmt.Fprintln(os.Stderr, "pdbframes: SymLoadModuleExW:", e)
		os.Exit(1)
	}
	if moduleInfo(h) != 3 {
		// no PDB through the image's debug directory: the PDB beside it,
		// loaded at the image's place
		symUnloadModule64.Call(h, base)
		pdb, _ := syscall.UTF16PtrFromString(strings.TrimSuffix(exe, filepath.Ext(exe)) + ".pdb")
		if r, _, e := symLoadModuleExW.Call(h, 0, uintptr(unsafe.Pointer(pdb)), 0, base, uintptr(imageSize(exe)), 0, 0); r == 0 {
			fmt.Fprintln(os.Stderr, "pdbframes: SymLoadModuleExW of the PDB:", e)
		}
		moduleInfo(h)
	}
	fmt.Println("traceback:")
	for i, pc := range pcs {
		// the image at its preferred base; a caller's line is its call's
		a := uint64(pc - loaded + base)
		if i > 0 {
			a--
		}
		if frame(h, a) {
			break
		}
	}
}

// frame prints the frames at a, the inlined calls first; whether it
// was main's.
func frame(h uintptr, a uint64) bool {
	n, _, _ := symAddrIncludeInline.Call(h, uintptr(a))
	if n > 0 {
		var ctx, idx uint32
		if r, _, _ := symQueryInlineTrace.Call(h, uintptr(a), 0, uintptr(a), uintptr(a), uintptr(unsafe.Pointer(&ctx)), uintptr(unsafe.Pointer(&idx))); r != 0 {
			for k := uint32(0); k < uint32(n); k++ {
				name := nameCtx(h, a, ctx+k)
				file, line := lineCtx(h, a, ctx+k)
				fmt.Printf("  %s (%s:%d) [inlined]\n", name, file, line)
			}
		}
	}
	name := nameAt(h, a)
	file, line := lineAt(h, a)
	if name == "xo_entry" {
		name = "main"
	}
	if file == "" {
		fmt.Printf("  %s\n", name)
	} else {
		fmt.Printf("  %s (%s:%d)\n", name, file, line)
	}
	if locals {
		printLocals(h, a)
	}
	return name == "main"
}

// printLocals prints the variables dbghelp finds at a.
func printLocals(h uintptr, a uint64) {
	// IMAGEHLP_STACK_FRAME: InstructionOffset first (the rest zero)
	var frame [20]uint64
	frame[0] = a
	symSetContext.Call(h, uintptr(unsafe.Pointer(&frame[0])), 0)
	var lines []string
	cb := syscall.NewCallback(func(info, size, ctx uintptr) uintptr {
		// (a pointer dbghelp passes: not Go memory)
		s := (*symbol)(unsafe.Add(unsafe.Pointer(nil), info))
		flags := *(*uint32)(unsafe.Pointer(&s.bytes()[40]))
		reg := *(*uint32)(unsafe.Pointer(&s.bytes()[64]))
		addr := *(*uint64)(unsafe.Pointer(&s.bytes()[56]))
		kind := ""
		if flags&0x40 != 0 {
			kind = " (param)"
		}
		where := ""
		switch {
		case flags&0x10 != 0:
			where = fmt.Sprintf("[reg %d%+d]", reg, int64(addr))
		case flags&0x8 != 0:
			where = fmt.Sprintf("reg %d", reg)
		}
		lines = append(lines, fmt.Sprintf("      %s%s %s", s.name(), kind, where))
		return 1
	})
	mask, _ := syscall.UTF16PtrFromString("*")
	symEnumSymbolsW.Call(h, 0, uintptr(unsafe.Pointer(mask)), cb, 0)
	for _, l := range lines {
		fmt.Println(l)
	}
}

// moduleInfo prints which dbghelp is loaded and what it found for the
// module: its symbol type (SymPdb 3, SymExport 4, SymDeferred 5, ...) and
// PDB (IMAGEHLP_MODULEW64, 3264 bytes: SymType at 32, LoadedPdbName at
// 1124, PdbUnmatched at 3224).
func moduleInfo(h uintptr) uint32 {
	if err := dbghelp.Load(); err == nil {
		var buf [520]uint16
		k32 := syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleFileNameW")
		n, _, _ := k32.Call(dbghelp.Handle(), uintptr(unsafe.Pointer(&buf[0])), 520)
		fmt.Fprintf(os.Stderr, "pdbframes: dbghelp %s\n", syscall.UTF16ToString(buf[:n]))
	}
	var mu [3264 / 8]uint64
	m := (*[3264]byte)(unsafe.Pointer(&mu[0]))
	*(*uint32)(unsafe.Pointer(&m[0])) = 3264
	r, _, e := symGetModuleInfoW64.Call(h, base, uintptr(unsafe.Pointer(&m[0])))
	if r == 0 {
		fmt.Fprintf(os.Stderr, "pdbframes: SymGetModuleInfoW64: %v\n", e)
		return 0
	}
	pdb := syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(&m[1124])), 256))
	t := *(*uint32)(unsafe.Pointer(&m[32]))
	fmt.Fprintf(os.Stderr, "pdbframes: symbol type %d, pdb %q, unmatched %d\n", t, pdb, *(*uint32)(unsafe.Pointer(&m[3224])))
	return t
}

// symbol is a SYMBOL_INFOW (88 bytes) with room for its name, 8 byte
// aligned (its 64 bit fields).
type symbol struct {
	u [(88 + 2*1024) / 8]uint64
}

// b is the symbol's bytes.
func (s *symbol) bytes() *[88 + 2*1024]byte { return (*[88 + 2*1024]byte)(unsafe.Pointer(&s.u[0])) }

func newSymbol() *symbol {
	s := &symbol{}
	*(*uint32)(unsafe.Pointer(&s.bytes()[0])) = 88
	*(*uint32)(unsafe.Pointer(&s.bytes()[80])) = 1024
	return s
}

func (s *symbol) name() string {
	n := *(*uint32)(unsafe.Pointer(&s.bytes()[76]))
	if n > 1024 {
		n = 1024
	}
	u := unsafe.Slice((*uint16)(unsafe.Pointer(&s.bytes()[84])), n)
	return syscall.UTF16ToString(u)
}

func nameAt(h uintptr, a uint64) string {
	s := newSymbol()
	var disp uint64
	if r, _, e := symFromAddrW.Call(h, uintptr(a), uintptr(unsafe.Pointer(&disp)), uintptr(unsafe.Pointer(&s.bytes()[0]))); r == 0 {
		if !failed {
			failed = true
			fmt.Fprintf(os.Stderr, "pdbframes: SymFromAddrW(0x%x): %v\n", a, e)
		}
		return fmt.Sprintf("?+0x%x", a-base)
	}
	return s.name()
}

func nameCtx(h uintptr, a uint64, ctx uint32) string {
	s := newSymbol()
	var disp uint64
	if r, _, _ := symFromInlineContextW.Call(h, uintptr(a), uintptr(ctx), uintptr(unsafe.Pointer(&disp)), uintptr(unsafe.Pointer(&s.bytes()[0]))); r == 0 {
		return "?"
	}
	return s.name()
}

// lineW is an IMAGEHLP_LINEW64 (40 bytes).
type lineW struct {
	size uint32
	_    uint32
	key  uintptr
	line uint32
	_    uint32
	file *uint16
	addr uint64
}

func (l *lineW) result() (string, int) {
	if l.file == nil {
		return "", 0
	}
	var u []uint16
	for p := unsafe.Pointer(l.file); *(*uint16)(p) != 0; p = unsafe.Add(p, 2) {
		u = append(u, *(*uint16)(p))
	}
	return filepath.Base(strings.ReplaceAll(syscall.UTF16ToString(u), "\\", "/")), int(l.line)
}

func lineAt(h uintptr, a uint64) (string, int) {
	l := lineW{size: 40}
	var disp uint32
	if r, _, _ := symGetLineFromAddrW64.Call(h, uintptr(a), uintptr(unsafe.Pointer(&disp)), uintptr(unsafe.Pointer(&l))); r == 0 {
		return "", 0
	}
	return l.result()
}

func lineCtx(h uintptr, a uint64, ctx uint32) (string, int) {
	l := lineW{size: 40}
	var disp uint32
	if r, _, _ := symGetLineFromInlineCtxW.Call(h, uintptr(a), uintptr(ctx), 0, uintptr(unsafe.Pointer(&disp)), uintptr(unsafe.Pointer(&l))); r == 0 {
		return "", 0
	}
	return l.result()
}

// report reads the image's load address and the frames' pcs (a run
// `pc (xN)` expanded) from a crash report.
func report(f *os.File) (uint64, []uint64, error) {
	var loaded uint64
	var pcs []uint64
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(l, "pc ") {
			if _, b, ok := strings.Cut(l, "(image base "); ok {
				loaded, _ = strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(b, "0x"), ")"), 16, 64)
			}
		}
		if strings.HasPrefix(l, "frames:") {
			for _, t := range strings.Fields(l[7:]) {
				if strings.HasPrefix(t, "0x") {
					v, _ := strconv.ParseUint(t[2:], 16, 64)
					pcs = append(pcs, v)
				} else if strings.HasPrefix(t, "(x") && len(pcs) > 0 {
					n, _ := strconv.Atoi(strings.TrimSuffix(t[2:], ")"))
					for k := 1; k < n; k++ {
						pcs = append(pcs, pcs[len(pcs)-1])
					}
				}
			}
		}
	}
	if loaded == 0 || len(pcs) == 0 {
		return 0, nil, fmt.Errorf("no `pc ... (image base B)` or `frames:` line")
	}
	return loaded, pcs, nil
}
