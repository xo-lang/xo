# Runtime helpers share a namespace with the C tests that include them

Status: Observed (2026-10-08, decision 0074)

Symptom: `go test ./internal/llvm` failed to compile
`std/llrt/testdata/rc_threads.c`: `too few arguments to function call,
expected 3, have 1` for `mkstr`, after a helper of that name was added
to the runtime.

Cause: the C tests (`rc_threads.c`, `tasks.c`, `evloop.c`, and the
`json_diff.c` and `http_diff.c` drivers) `#include "../xo_rt.c"`, which
includes `xo_std.c`, `xo_json.c`, and `xo_http.c`; a `static` helper of
the runtime collides with a test's own helper of the same name.

Fix: the helpers of the std intrinsics are prefixed (`n_`, `nstr`,
`nbuf_`, `j_`, `h_`, `hp_`, `hw_`, `hc_`); keep new ones prefixed, and
build the C tests (`go test -short ./internal/llvm`) after adding one.
