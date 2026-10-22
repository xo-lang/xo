//go:build windows

// Command pdbframes prints the traceback of a native Xo program's crash
// report (decision 0343) from the program's PDB alone (decision 0344),
// through Windows' own PDB reader, dbghelp.dll: each frame's function,
// file and line, the calls inlined at a pc first, as the runtime's
// traceback prints them, ending at main. scripts/ci/windows-pdb.sh
// compares its output with the report's own traceback, which the runtime
// names from the DWARF.
//
//	pdbframes PROG.exe < REPORT
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
	dbghelp                  = syscall.NewLazyDLL("dbghelp.dll")
	symSetOptions            = dbghelp.NewProc("SymSetOptions")
	symInitializeW           = dbghelp.NewProc("SymInitializeW")
	symLoadModuleExW         = dbghelp.NewProc("SymLoadModuleExW")
	symFromAddrW             = dbghelp.NewProc("SymFromAddrW")
	symGetLineFromAddrW64    = dbghelp.NewProc("SymGetLineFromAddrW64")
	symAddrIncludeInline     = dbghelp.NewProc("SymAddrIncludeInlineTrace")
	symQueryInlineTrace      = dbghelp.NewProc("SymQueryInlineTrace")
	symFromInlineContextW    = dbghelp.NewProc("SymFromInlineContextW")
	symGetLineFromInlineCtxW = dbghelp.NewProc("SymGetLineFromInlineContextW")
)

// the preferred base Xo's PE linker gives every image
const base = 0x140000000

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: pdbframes PROG.exe < REPORT")
		os.Exit(2)
	}
	exe, _ := filepath.Abs(os.Args[1])
	loaded, pcs, err := report(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pdbframes:", err)
		os.Exit(1)
	}
	h := uintptr(0x584f) // any value identifies the session
	// SYMOPT_UNDNAME | SYMOPT_DEFERRED_LOADS | SYMOPT_LOAD_LINES |
	// SYMOPT_FAIL_CRITICAL_ERRORS | SYMOPT_NO_PROMPTS
	symSetOptions.Call(0x2 | 0x4 | 0x10 | 0x200 | 0x80000)
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
	return name == "main"
}

// symbol is a SYMBOL_INFOW (88 bytes) with room for its name.
type symbol struct {
	b [88 + 2*1024]byte
}

func newSymbol() *symbol {
	s := &symbol{}
	*(*uint32)(unsafe.Pointer(&s.b[0])) = 88
	*(*uint32)(unsafe.Pointer(&s.b[80])) = 1024
	return s
}

func (s *symbol) name() string {
	n := *(*uint32)(unsafe.Pointer(&s.b[76]))
	if n > 1024 {
		n = 1024
	}
	u := unsafe.Slice((*uint16)(unsafe.Pointer(&s.b[84])), n)
	return syscall.UTF16ToString(u)
}

func nameAt(h uintptr, a uint64) string {
	s := newSymbol()
	var disp uint64
	if r, _, _ := symFromAddrW.Call(h, uintptr(a), uintptr(unsafe.Pointer(&disp)), uintptr(unsafe.Pointer(&s.b[0]))); r == 0 {
		return fmt.Sprintf("?+0x%x", a-base)
	}
	return s.name()
}

func nameCtx(h uintptr, a uint64, ctx uint32) string {
	s := newSymbol()
	var disp uint64
	if r, _, _ := symFromInlineContextW.Call(h, uintptr(a), uintptr(ctx), uintptr(unsafe.Pointer(&disp)), uintptr(unsafe.Pointer(&s.b[0]))); r == 0 {
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
