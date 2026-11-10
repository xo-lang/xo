# Go's syscall on windows/arm64 returns x0 only

Status: Observed 2026-11-10 (Xo Lang 2, the windows DLL test of decision
0574).

Symptom: a Go caller of an Xo DLL on winarm got an empty string from an
export returning XoStr, while the same test passed on windows/amd64.

Cause: on windows/arm64 a 16 byte struct comes back in x0 and x1, but
Go's syscall.(*Proc).Call returns r2 from another register, not x1
(go/src/internal/runtime/syscall/windows/asm_windows_arm64.s, "TODO(rsc)
floating point like amd64 in StdCallInfo_R2?"): the length read as 0.
Go's syscall also does not return a double result (xmm0, d0) on either
architecture.

Fix: in a Go test caller, read an XoBytes or XoStr result's bytes up to
the NUL that c_bytes_out puts past them (or test the length through C),
and check Float results through C or another export. A test file named
`*_windows_test.go` builds only for GOOS=windows: name a cross-build
test file otherwise (cmd/xo clib_dll_test.go).
