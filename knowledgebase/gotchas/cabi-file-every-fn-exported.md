# Every top-level function of a cabi file is a C ABI export

Status: Observed (2026-11-12, std/native/llrt/cabi_log.xo, decision 0617 C)

Symptom: building xo failed with `error XO1201: not available on the
native backends (arm64, amd64, LLVM) yet: a C ABI export with a
`log.Handler?` result (support is planned)`, for a helper function that
was never meant to be exported.

Cause: a `cabi_*.xo` file in std/native/llrt is the C ABI surface: every
top-level function in it is exported (with its name as the symbol),
`pub` or not, and an export's parameters and result must be C ABI types.
The handler's release (`log_free`, called by atexit through
`rt.fn_addr`) sat in cabi_log.xo with Xo-typed helpers beside it.

Fix: keep only the exported functions in the cabi file and move their
helpers to an ordinary llrt module (std/native/llrt/loghand.xo here),
which the cabi file and its callers import.
