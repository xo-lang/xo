# Generated-code entries: direct calls of the Xo implementations (2026-10-15)

Machine: the development Mac (Apple silicon, arm64), the zero-C push's
branch zc-b19 on zero-c. Instructions retired from `/usr/bin/time -l`,
the least of 5 runs of each binary (11 for the conc rerun below). A is
the compiler before the change (zero-c at 2ca82cdc), B after; both read
the same std. Script: a scratch A/B driver building bench/perf/alloc,
bench/perf/text, and bench/perf/conc/xo/{spawn,fanout,pingpong,mutex}
with each compiler on each backend, checking the outputs' checksums agree.

## Batch E1

The LLVM emitter calls obj.xo's list and string entries through their
exports (xo_x_obj_rl_free_n, rl_share, rc_defer_n, rl_new_n, rs_concat;
inlined at the call, `#6`, as the C forwarders' XO_INLINE_CALL did), and
both emitters call heapos.xo's platform calls (rt.heap_big,
heap_big_resize, heap_big_release, heap_grow, heap_lock, heap_unlock)
and anyval.xo (rt.any_new, any_get) through their exports; on LLVM those
calls are `nounwind noinline` (`#8`, gotchas/llvm-direct-export-call-inlines-slow-path.md:
without it, alloc tasks 1.10 and strings 1.03). The C forwarders stay
for code of earlier compilers (the end pass's seed).

| Program | Workload | Backend | A instr | B instr | B/A |
|---|---|---|---|---|---|
| alloc | small | arm64 | 290,064,320 | 290,156,480 | 1.000 |
| alloc | mixed | arm64 | 470,493,040 | 470,471,509 | 1.000 |
| alloc | grow | arm64 | 65,093,249 | 65,078,851 | 1.000 |
| alloc | strings | arm64 | 301,115,752 | 301,008,323 | 1.000 |
| alloc | maps | arm64 | 301,554,527 | 301,597,565 | 1.000 |
| alloc | tasks | arm64 | 879,503,906 | 879,004,720 | 0.999 |
| alloc | small | llvm | 249,247,174 | 245,316,399 | 0.984 |
| alloc | mixed | llvm | 461,876,135 | 461,511,943 | 0.999 |
| alloc | grow | llvm | 63,813,460 | 63,749,132 | 0.999 |
| alloc | strings | llvm | 257,192,390 | 244,458,611 | 0.950 |
| alloc | maps | llvm | 266,334,668 | 265,643,932 | 0.997 |
| alloc | tasks | llvm | 787,041,190 | 782,774,052 | 0.995 |
| text | parse_int | arm64 | 44,813,918 | 44,813,229 | 1.000 |
| text | quote | arm64 | 128,681,632 | 128,818,419 | 1.001 |
| text | split | arm64 | 96,474,601 | 96,637,984 | 1.002 |
| text | lines | arm64 | 154,227,408 | 154,156,188 | 1.000 |
| text | replace | arm64 | 80,444,453 | 80,454,986 | 1.000 |
| text | int_fmt | arm64 | 77,807,798 | 77,953,668 | 1.002 |
| text | base64 | arm64 | 165,172,864 | 165,326,307 | 1.001 |
| text | hex | arm64 | 99,564,980 | 99,852,885 | 1.003 |
| text | parse_int | llvm | 43,559,191 | 43,066,676 | 0.989 |
| text | quote | llvm | 102,662,744 | 100,133,107 | 0.975 |
| text | split | llvm | 81,352,294 | 80,818,522 | 0.993 |
| text | lines | llvm | 138,444,491 | 137,896,885 | 0.996 |
| text | replace | llvm | 57,196,127 | 56,740,196 | 0.992 |
| text | int_fmt | llvm | 61,586,246 | 61,104,385 | 0.992 |
| text | base64 | llvm | 123,534,496 | 123,083,845 | 0.996 |
| text | hex | llvm | 83,629,398 | 83,134,057 | 0.994 |
| conc_spawn | - | arm64 | 665,729,726 | 674,648,412 | 1.013 |
| conc_spawn | - | llvm | 577,198,765 | 586,943,764 | 1.017 |
| conc_fanout | - | arm64 | 325,907,056 | 334,216,354 | 1.025 |
| conc_fanout | - | llvm | 280,591,335 | 277,351,652 | 0.988 |
| conc_pingpong | - | arm64 | 417,282,772 | 469,484,526 | 1.125 |
| conc_pingpong | - | llvm | 389,656,439 | 388,850,956 | 0.998 |
| conc_mutex | - | arm64 | 159,418,205 | 157,098,754 | 0.985 |
| conc_mutex | - | llvm | 104,413,006 | 115,293,126 | 1.104 |

Noise: an A against A run (the same compiler on both sides) gave 0.999 to
1.002 on alloc and text and 0.875 to 1.020 on conc (threads). The conc
rows are within that band; a rerun with 11 runs each gave 0.924 to 1.079,
and conc mutex on LLVM interleaved 21 times gave a least of 1.023 and a
median of 0.990 of A. Nothing passes the 5% bar; LLVM alloc strings
(0.950) and text quote (0.975) gain from rs_concat's direct call.
