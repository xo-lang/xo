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

## Batch E2

Both emitters call obj.xo's exports for a string's, box's, and closure
environment's block (xo_x_obj_rc_alloc, xo_x_obj_rc_free) and a
handle's free at zero (xo_x_obj_h_free) instead of the C entries
xo_rc_alloc, xo_rc_free, and xo_h_free (kept for code of earlier
compilers). obj.rc_free_n is rl_free_n with no elements to drop, which
reuses its fast path (the one thread's heap word and free1 inlined; a
third call site of free1 would stop the inliner copying it). On LLVM
the calls are `nounwind noinline` (`#8`): inlined into every drop
helper, text split and lines took 1.05 to 1.07 times the instructions;
with rl_free_n inlined at the call (as the list drop does), 1.05 to
1.07 again. A is zero-c at 51dec8c7, B adds E2:

| Program | Workload | Backend | A instr | B instr | B/A |
|---|---|---|---|---|---|
| alloc | small | arm64 | 290,987,034 | 289,223,357 | 0.994 |
| alloc | mixed | arm64 | 471,449,577 | 471,220,265 | 1.000 |
| alloc | grow | arm64 | 65,861,410 | 65,870,044 | 1.000 |
| alloc | strings | arm64 | 301,902,919 | 300,699,815 | 0.996 |
| alloc | maps | arm64 | 302,314,615 | 302,182,054 | 1.000 |
| alloc | tasks | arm64 | 886,180,459 | 887,795,059 | 1.002 |
| alloc | small | llvm | 246,268,308 | 246,035,558 | 0.999 |
| alloc | mixed | llvm | 462,171,612 | 462,254,804 | 1.000 |
| alloc | grow | llvm | 64,581,630 | 64,552,387 | 1.000 |
| alloc | strings | llvm | 245,140,182 | 249,727,315 | 1.019 |
| alloc | maps | llvm | 266,426,273 | 266,414,270 | 1.000 |
| alloc | tasks | llvm | 784,886,493 | 785,779,078 | 1.001 |
| text | parse_int | arm64 | 45,608,208 | 45,701,292 | 1.002 |
| text | quote | arm64 | 129,642,512 | 129,312,636 | 0.997 |
| text | split | arm64 | 97,472,707 | 96,808,756 | 0.993 |
| text | lines | arm64 | 155,020,653 | 153,913,698 | 0.993 |
| text | replace | arm64 | 81,415,535 | 81,120,221 | 0.996 |
| text | int_fmt | arm64 | 78,762,787 | 78,418,218 | 0.996 |
| text | base64 | arm64 | 166,210,939 | 165,734,698 | 0.997 |
| text | hex | arm64 | 100,493,905 | 100,038,016 | 0.995 |
| text | parse_int | llvm | 43,828,667 | 43,845,895 | 1.000 |
| text | quote | llvm | 100,901,577 | 101,380,214 | 1.005 |
| text | split | llvm | 81,615,399 | 83,138,114 | 1.019 |
| text | lines | llvm | 138,638,642 | 141,448,160 | 1.020 |
| text | replace | llvm | 57,400,286 | 58,001,288 | 1.010 |
| text | int_fmt | llvm | 61,854,080 | 62,904,696 | 1.017 |
| text | base64 | llvm | 123,863,515 | 124,781,203 | 1.007 |
| text | hex | llvm | 83,908,566 | 84,822,647 | 1.011 |
| conc_spawn | - | arm64 | 620,574,104 | 626,850,224 | 1.010 |
| conc_spawn | - | llvm | 500,465,042 | 515,945,216 | 1.031 |
| conc_fanout | - | arm64 | 298,325,256 | 306,638,015 | 1.028 |
| conc_fanout | - | llvm | 241,186,897 | 264,813,246 | 1.098 |
| conc_pingpong | - | arm64 | 379,637,636 | 384,670,402 | 1.013 |
| conc_pingpong | - | llvm | 319,175,314 | 314,308,115 | 0.985 |
| conc_mutex | - | arm64 | 136,032,241 | 146,676,056 | 1.078 |
| conc_mutex | - | llvm | 105,194,257 | 109,434,310 | 1.040 |

conc stays in its noise band: a rerun with 11 runs each gave 0.940 to
1.041 except conc mutex on arm64 (1.120), whose interleaved 21 runs give
medians of 128.1M (A) and 129.1M (B), spread 80M to 200M. LLVM keeps
about 2% on string heavy workloads (text split, lines, alloc strings
1.019 to 1.020), inside the 5% bar; arm64 is at or under A.

## Batch E3

rt.rl_empty and rt.h_zero (the immortal empty list a `[]` starts as, and
the placeholders an unwinding operation returns) are the addresses of
xo_rt.c's xo_empty_list and xo_zero_handle, emitted inline (arm64
adrp and add, LLVM an external global), instead of calls of xo_rl_empty
and xo_h_zero (kept for code of earlier compilers). internal/arm64's
sieve assembly golden changed by exactly that (`bl _xo_rl_empty; mov`
to `adrp; add`). A is zero-c at 83992919, B adds E3:

| Program | Workload | Backend | A instr | B instr | B/A |
|---|---|---|---|---|---|
| alloc | small | arm64 | 289,580,194 | 283,727,080 | 0.980 |
| alloc | mixed | arm64 | 471,592,128 | 470,795,240 | 0.998 |
| alloc | grow | arm64 | 65,861,676 | 65,871,051 | 1.000 |
| alloc | strings | arm64 | 300,408,153 | 300,373,234 | 1.000 |
| alloc | maps | arm64 | 302,445,300 | 301,162,434 | 0.996 |
| alloc | tasks | arm64 | 889,421,524 | 883,497,433 | 0.993 |
| alloc | small | llvm | 246,650,231 | 246,981,004 | 1.001 |
| alloc | mixed | llvm | 462,346,603 | 462,443,474 | 1.000 |
| alloc | grow | llvm | 64,544,686 | 64,555,611 | 1.000 |
| alloc | strings | llvm | 249,745,593 | 249,829,669 | 1.000 |
| alloc | maps | llvm | 266,529,513 | 266,570,792 | 1.000 |
| alloc | tasks | llvm | 783,173,984 | 782,754,287 | 0.999 |
| text | parse_int | arm64 | 45,604,678 | 45,633,749 | 1.001 |
| text | quote | arm64 | 129,330,245 | 129,386,218 | 1.000 |
| text | split | arm64 | 96,834,231 | 96,660,346 | 0.998 |
| text | lines | arm64 | 154,001,877 | 153,926,309 | 1.000 |
| text | replace | arm64 | 81,164,744 | 81,100,423 | 0.999 |
| text | int_fmt | arm64 | 78,289,810 | 78,236,834 | 0.999 |
| text | base64 | arm64 | 165,631,606 | 165,578,020 | 1.000 |
| text | hex | arm64 | 100,038,765 | 99,971,243 | 0.999 |
| text | parse_int | llvm | 43,707,901 | 43,779,858 | 1.002 |
| text | quote | llvm | 101,467,351 | 101,439,273 | 1.000 |
| text | split | llvm | 83,083,217 | 83,101,459 | 1.000 |
| text | lines | llvm | 141,468,996 | 141,460,490 | 1.000 |
| text | replace | llvm | 58,032,586 | 58,022,768 | 1.000 |
| text | int_fmt | llvm | 62,919,437 | 62,889,288 | 1.000 |
| text | base64 | llvm | 124,888,390 | 124,863,753 | 1.000 |
| text | hex | llvm | 84,922,017 | 84,952,410 | 1.000 |
| conc_spawn | - | arm64 | 667,803,244 | 676,961,590 | 1.014 |
| conc_spawn | - | llvm | 579,688,989 | 582,925,044 | 1.006 |
| conc_fanout | - | arm64 | 321,334,434 | 323,097,181 | 1.005 |
| conc_fanout | - | llvm | 277,879,514 | 274,114,904 | 0.986 |
| conc_pingpong | - | arm64 | 393,053,621 | 387,506,007 | 0.986 |
| conc_pingpong | - | llvm | 322,961,913 | 317,072,773 | 0.982 |
| conc_mutex | - | arm64 | 164,105,989 | 148,421,429 | 0.904 |
| conc_mutex | - | llvm | 124,030,043 | 97,437,276 | 0.786 |

conc mutex's 0.904 and 0.786 are within its run to run spread (80M to
200M instructions on arm64, above).


## Batch N1 (net, scope, shutdown and real-time forwarders)

Both emitters call the existing exports for nine entries through one
mapping, selfhost/ssa/exports.xo rt_export: rt.scope_deadline,
scope_timed_out, stop_requested, task_stopping (xo_x_stop_stopping),
realtime_enter and realtime_exit (xo_x_rt_enter, rt_exit),
listener_close, conn_write and conn_close (xo_x_nc_*). Owner rule of
2026-10-15: per batch only hello on both backends and -short (plus the
14 goldens that use these calls, on LLVM and arm64); the A/B, the full
goldens and the fixed point run once before the next seed.

C deletable after the next seed (std/llrt/xo_rt.c): xo_scope_deadline,
xo_scope_timed_out (and their xo_x_ prototypes), xo_stop_requested,
xo_task_stopping, xo_realtime_enter, xo_realtime_exit,
xo_listener_close, xo_conn_write, xo_conn_close.

## Batch N2 (the rest of the network)

rt.net_listen, net_dial and listener_accept call xo_x_nc_listen,
nc_dial and nc_listener_accept; rt.listener_addr, conn_read, net_status
and net_msg call new exports (cabi_net.xo nc_listener_addr_list,
nc_conn_read_list, nc_status, nc_msg) that build the List[Str] of one
in netconn.xo (list_of with no length slot, str_list). A 0 result of
the five that return one is the zero handle or the empty list, chosen
inline (LLVM select, arm64 cmp and csel; ssa/exports.xo
rt_export_none). A probe of every NetErr message path (bad dial and
listen addresses, read, write and accept after close) prints the same
on main and the branch on both backends; the generated code of the
probe calls none of the C net entries (20 calls on main). Tests: hello
on both backends, -short, and the network goldens on LLVM and arm64.

C deletable after the next seed (std/llrt/xo_rt.c): xo_net_status,
xo_net_msg, str_list, nc_handle, nc_list, xo_net_listen, xo_net_dial,
xo_listener_addr, xo_listener_accept, xo_conn_read; then the
xo_listener and xo_conn typedefs and asserts if nothing else uses them,
and cabi_net.xo's nc_listener_addr and nc_conn_read (and netconn.xo
list_of's length slot). Deleted now: xo_net_dial_timeout and
cabi_net.xo nc_dial_timeout (no caller since the C TLS went).

## Batch N3 (the clock and cancellation checks)

rt.clock_now, clock_sleep, clock_sleep_or_cancel, select_after and
is_canceled call new cabi.xo exports of the same names: clock_now is
poll.xo wall_now (the virtual clock while the test world's is virtual,
else sysio.xo real_ns; Clock.now itself is sysio.xo's clock_now, which
reaches it only for the virtual clock, and every rt.clock_now in the Xo
runtime goes the same way), the sleeps record the test world's sleep
(twrun.xo sleep_record) and wait in poll.xo clock_wait, clock_sleep
then marks the context unwinding (tasks.xo canceled), select_after is
selects.xo after, is_canceled tasks.xo is_canceled. The arm64 assembly
of the clock_timers golden calls none of the C entries. Tests: hello on
both backends, -short, 14 goldens on LLVM and arm64 (clock, scopes,
select, tasks, budgets, real-time), cmd/xo TestNoGCTests.

C deletable after the next seed (std/llrt/xo_rt.c): xo_clock_now,
clock_wait, xo_clock_sleep, xo_clock_sleep_or_cancel, xo_select_after,
xo_is_canceled.

## Freed C of N1 to N3 after the next seed

Deleting every function listed under N1, N2 and N3 from xo_rt.c, then
what loses its last caller (the statics canceled, rl_data and rl_new;
the declarations or weak stubs of xo_x_scope_deadline, scope_timed_out,
real_ns, poll_clock_wait, tw_sleep, rt_enter, rt_exit, stop_requested,
stop_stopping and the eight xo_x_nc_* the forwarders called; the
xo_listener and xo_conn typedefs and asserts), takes xo_rt.c from 3223
to 3039 lines (184), and the result compiles (clang -fsyntax-only; no
other new unused function). In Xo after the same seed: cabi_net.xo
nc_listener_addr and nc_conn_read, and netconn.xo list_of's length
slot.
