# A deleted XO_LLRT_C name or -D flag still set by a test tests nothing

Status: Observed (2026-10-13, the zero-C audit refresh,
bench/results/zero-c-audit-refresh-2026-10-13.md, "The switch table").

Symptom: a differential test that compares a runtime piece's Xo path
with its C path keeps passing after decision 0198 deleted the C path,
because both of its builds are now the Xo path.
internal/llvm/threads_test.go:205 (`-DXO_RC_C`), 234 and 246
(`-DXO_ALLOC_C`), ints_test.go:237, tz_test.go:217, digest_test.go:454,
scan_test.go:181, sysio_test.go:33, and http1_diff_test.go:279 still set
deleted names; selfhost/xo/link.xo:308-369 and 380-393 and
internal/xotool/cc.go:55-109 and 119-133 still turn their symbols into
-D flags that guard no C.

Cause: an unknown `XO_LLRT_C` name is ignored (selfhost/check/load.xo:380
falls back to `<name>.xo` when `<name>_c.xo` is missing), and an
undefined -D macro compiles nothing. Neither fails loudly.

Fix: when a C path is deleted, grep internal/, cmd/, bench/, and
selfhost/ for its `XO_LLRT_C` name and its `-DXO_<NAME>_C` flag and drop
or rewrite each use in the same change. The live names are the
`std/native/llrt/*_c.xo` files (net, sched, text on 2026-10-13).
