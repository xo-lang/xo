# A deleted XO_LLRT_C name or -D flag still set by a test tests nothing

Status: Observed (2026-10-13, the zero-C audit refresh,
bench/results/zero-c-audit-refresh-2026-10-13.md, "The switch table").
Fixed 2026-10-14 (audit item W0): an unknown name now fails the build.

Symptom: a differential test that compares a runtime piece's Xo path
with its C path keeps passing after decision 0198 deleted the C path,
because both of its builds are now the Xo path.
internal/llvm/threads_test.go (`-DXO_RC_C`, `-DXO_ALLOC_C`),
ints_test.go, tz_test.go, digest_test.go, scan_test.go, sysio_test.go,
http1_diff_test.go, and cmd/xo's glob_test.go and gen_diff_test.go set
deleted names; compiler/xo/link.xo's io_flags and
internal/xotool/cc.go's IOFlags turned their symbols into -D flags that
guarded no C.

Cause: an unknown `XO_LLRT_C` name was ignored (compiler/check/load.xo
falls back to `<name>.xo` when `<name>_c.xo` is missing), and an
undefined -D macro compiles nothing. Neither failed loudly.

Fix: compiler/xo/main.xo llrt_c_names fails a build (`xo: XO_LLRT_C=rc:
no C path named rc ...`, exit 1) on a name with no
`std/native/llrt/<name>_c.xo` (`all` aside); cmd/xo TestLLRTCUnknownName
holds it. The tests above dropped their dead variants, and io_flags and
IOFlags dropped the dead -D entries. -D flags still fail silently: when
a C path is deleted, grep internal/, cmd/, bench/, and compiler/ for its
`-DXO_<NAME>_C` flag and drop each use in the same change. The live
names are the `std/native/llrt/*_c.xo` files (net, sched, text on
2026-10-14).
