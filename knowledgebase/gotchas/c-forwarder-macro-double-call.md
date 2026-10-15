# A C forwarder macro that names its argument twice calls the export twice

Status: Observed (2026-10-15, the zero-C audit's B22 and B23 remnants)

Symptom: after xo_rt.c's network functions became forwarders to Xo
exports, the network goldens hung (net_echo), crashed in listen
(unix_kill), or failed (http_*), on arm64 and LLVM alike.

Cause: the forwarders used macros like
`#define NC_HANDLE(e) ((e) ? (void *)(intptr_t)(e) : xo_h_zero())`
with the export call as the argument. The macro expands `e` twice, so
every forwarder called the Xo export twice: two listens on one address,
two accepts, two reads (the first read's data lost).

Fix: forward through a `static inline` function taking the export's
result (`nc_handle(int64_t r)`, `nc_list(int64_t r, int64_t n)` in
xo_rt.c), never a macro over an expression with effects.
