# xo_rt.c inventory after the generated-code entries (2026-10-15)

What keeps each section of std/llrt/xo_rt.c (main at e2d5762d, 3,209
lines) now that generated code calls Xo for every entry but the ones by
design, and what moving its root caller to Xo would free. Two halves by
line: the first (lines 1 to 1,698, Memory through the worker pool) by
Xo Lang 3, the second (1,699 to 3,209, scopes and tasks through Entry) by
Xo Lang 2 (xo-4b); the ranked merge is below them.

Columns: lines; what keeps it (called by current generated code; called
only by other C, with the root caller; Windows only; seed only, gone
after the next seed; C by design, with the reason); the lines freed if
its root caller moved to Xo; a rough effort (S under an hour, M a few
hours, L a day or more).

## Ranked (lines freed per unit of effort)

Effort units: S = 1, M = 4, L = 12. Line counts are the halves'
estimates (the second half's are measured by deletion where marked);
items that need another first are marked. Both halves' details are
below.

No work:

| Item | Lines | When |
|---|---|---|
| Seed-only forwarders and their helpers: the clock, network, stop forwarders and their helpers (195, measured), the fault forwarders, str_eq and str_cmp, the number builders, the personality forwarder (about 110) | about 300 | after the next seed |
| Dead now: deadline(), pd_wait, the net_peername and net_ntop declarations and stubs | about 25 | now |

Work, by lines per unit:

| Rank | Item | Lines | Effort | Lines per unit | Needs |
|---|---|---|---|---|---|
| 1 | Layout-only record types and their asserts (xo_scope, xo_mutex, xo_atomic, xo_cq, xo_sarm; xo_select after the seed) | about 130 | S | 130 | nothing |
| 2 | Windows' WSAPoll fallback loop and its registration list | about 150 | S | 150 | a decision that every supported Windows has \Device\Afd (UNTESTED on windows) |
| 3 | rt.fn_addr: an Xo export's address as a word, then the services records filled from Xo (svc, pd_svc, stop_svc), poll.xo's and stop.xo's callbacks as exports, the signal handlers as exports, the scheduler services that only forward back into Xo | about 300 | M | 75 | the owner's approval of an rt intrinsic; both emitters |
| 4 | The test world record tw into twflags.xo's static words | about 45 | S | 45 | DONE a30277e8: 70 lines deleted, 16 added |
| 5 | The handle code h_init, h_release, h_free (rt.sched_rt ops 19 and 25 already gone, 26326187) | about 60 | S | 60 | DONE: 46 lines in this batch (stop.xo does its own retain and release, no fn_addr needed) |
| 6 | heap_libc's dead C library mapping arm (finding) | about 20 | S | 20 | a choice: delete it, or restore it before the sanitizer macros |
| 7 | xo_worker and xo_fiber record types, after sched_hooks, fiber_map_failed, xo_sched_run_task move | about 100 | M | 25 | DONE with st, rtstats and fibers (about 180 lines; xo_sched_run_task stays as a forwarder until the next seed) |
| 8 | The C allocator entries and the main heap record (xo_alloc, xo_free, xo_resize, heap(), main_heap; xo_rt_threads_begin, obj_heap_init) | about 285 | M | 70 | DONE: entries 864ce695, the heap record and its setup in this batch (heapos.xo main_record, cabi.xo heap_start and heap_threads) |
| 9 | Entry: main as a short call into entry.xo | about 55 | M | 14 | rank 8 for the heap setup |
| 10 | heap_large, resize, release onto libcmem.xo's malloc on every build | about 28, and no C library malloc | M | 7 | nothing |
| 11 | The event loop's records ev, xo_pd, xo_timer and their asserts | about 100 | M | 25 | the Windows poller (ranks 2 and 12), the C tests rawpoll.c and evloop.c moved or retired |
| 12 | Windows' AFD start and registration in poll.xo over sys_windows | about 180 | M | 45 | rank 2 (UNTESTED on windows) |
| 13 | Contexts, parks, nodes, locks and their asserts | about 250 | L | 21 | ranks 3, 11, 14 |
| 14 | Fault unwinding's C (Windows, XO_UNWIND_LIBGCC) and the C personality with lsda_read | about 240 | L | 20 | decision 0221 stage 2 on x86-64; RtlCaptureContext through sys_c.xo |
| 15 | run_entry and run_main's boundary frame | about 15 | M | 4 | a frame address rt intrinsic (the owner's approval) |

Order this suggests: the next seed (about 300 lines) and the dead lines
now; ranks 1, 4, 5 and 6 (small, independent, about 255 lines); the
owner's decisions on rt.fn_addr (rank 3, the largest unlock, about 300
lines plus most of ranks 5, 7 and 9) and on Windows' WSAPoll fallback
(rank 2); then rank 8 and the rest. Of xo_rt.c's 3,213 lines, about
1,700 have a route out short of decision 0221 stage 2 and the Windows
pass; about 550 wait for those.

## First half (Xo Lang 3)

xo_rt.c lines 1 to 1,700 on main 837c5261 (3,213 lines in all). Code
lines exclude comments and blank lines; "seed" means the C is only for
code the 0.12.0 seed generates and goes after the next seed with no
work.

| Section (lines) | Lines (code) | What keeps it | Freed if the root moves | Effort |
|---|---|---|---|---|
| Preamble, includes and platform macros (1 to 66) | 66 (27) | every other C part | shrinks with the rest | none of its own |
| Memory: the heap record type, its asserts, main_heap and central statics (68 to 160) | about 95 (40) | C's own heap use: heap() in h_free and rt_defer, obj_heap_init handing main_heap's address to alloc.xo | about 95, once the C below stops allocating and alloc.xo makes the main thread's record itself (as ctx.xo init now makes the pool record) | M |
| Memory: C allocator entries xo_alloc, xo_free, xo_resize, alloc_in, free_in, alloc_mt, free_mt, XO_INLINE_CALL (300 to 410) | about 110 (60) | other C only: rs_new, rs_append, rl_new, h_init users in this file, and xo_proc.c (no caller since 837c5261) | about 110 with the next row's users | M |
| Memory: heap_large, heap_large_resize, heap_release (269 to 296) | 28 (17) | generated code (rt.heap_large: the runtime's records), malloc by design | 28, if rt.heap_large uses an Xo record allocator: libcmem.xo's malloc already does this for XO_NOLIBC builds, so every build could use it, which also drops the C library's malloc | M |
| Memory: xo_rt_threads_begin, obj_heap_init, hw_slow_c, heap_slow (186 to 268) | about 80 (35) | the C main (fault_init, the constructor of XO_RT_NO_MAIN) and rt.sched_rt op 20 (pool.xo's switch to threads) | about 80, if entry.xo and pool.xo do the switch | S |
| Reference counts (412 to 457) | 46 (9) | the C handle and string code below | 46 with it | S |
| The test world record tw and its weak stubs (459 to 508) | 50 (24) | C readers of tw.virt and tw.on: xo_clock_now, clock_wait, rt.sched_rt op 30 | about 45, if twflags.xo's static words hold the clock state | S |
| Output: write_all, xo_flush (510 to 529) | 20 (13) | xo_flush: generated code at exit (by design while main is C) | 20 with the entry | S |
| C ABI export declarations (531 to 548) | 18 (6) | the C callers of exports | shrinks with them | none |
| Faults: xo_fault and the 8 raiser forwarders (550 to 615) | about 65 (40) | seed | 65 after the next seed | none |
| Faults: fault_init (616 to 628) | 13 (6) | the C main | 13 with the entry | S |
| Strings: xo_str_eq, xo_str_cmp (630 to 644) | 15 (11) | seed | 15 after the next seed | none |
| Reference counted strings and lists: the number builders (684 to 716) | 33 (28) | seed | 33 after the next seed | none |
| Reference counted strings and lists: alloc_usable, rs_new, rs_append, rl_new (646 to 683, 717 to 749) | about 70 (35) | other C: str_list and nc_list (the network forwarders, second half), xo_net_msg | about 70, once the network forwarders build their lists in Xo | S |
| Tasks: xo_hdr, h_init, h_release, h_free, the immortal placeholders (751 to 836) | 86 (31) | C handles (stop_svc's release, rt.sched_rt ops 19 and 25); xo_empty_list and xo_zero_handle are data generated code names | about 60 for the handle code; the placeholders (10) could be emitted by the compiler as data in the program | S |
| Parks, nodes, contexts and the misnamed "Real-time sections" block: st and rtstats, the realtime forwarders, futex and lock wrappers, xo_park, xo_node, xo_held, xo_went, xo_cent, xo_ctx and their asserts, cur_get, ctx(), ctx_exc (838 to 1134) | 297 (144) | the C that reads contexts and takes locks: fault unwinding below, the Windows poller, clock_wait, the network forwarders; xo_realtime_enter/exit forward for the seed | about 250 once those go (the types and asserts are most of it); 10 after the next seed for the realtime forwarders | L (it follows fault unwinding and Windows) |
| Fault unwinding: fault_raise, fault_stop, fault_cleanup, fault_set, fault_raise_msg, ctx_exc users (1136 to 1290) | about 155 (90) | Windows (RtlCaptureContext before the walk) and the XO_UNWIND_LIBGCC path (glibc linux/amd64, darwin/amd64) since xo-fa's 179e7497; xo_fault_raise and xo_fault_continue forward for the seed | about 150 when decision 0221 stage 2 gives x86-64 the Xo unwinder and Windows calls RtlCaptureContext through sys_c.xo | L |
| Fault unwinding: personality routines and lsda_read (1300 to 1445) | about 150 (100) | the forwarder xo_personality: seed (5 lines); Windows' language handler (10); the full C personality with lsda_read for XO_UNWIND_LIBGCC (about 90) | 5 after the next seed; about 90 with decision 0221 stage 2; Windows' 10 at the Windows pass | L |
| Resource budgets: canceled, ctx_canceled, the budget forwarders (1447 to 1493) | 47 (22) | C callers of canceled (clock_sleep, rt.sched_rt) and forwarders for the seed | about 40 with the clock and scheduler C in the second half | S |
| Worker pool: xo_runq, xo_worker, the pool comment, park_signal and node wrappers, the services record svc and its asserts (1495 to 1700) | 206 (120) | the svc record holds C addresses of Xo exports (XO_FREE_FN), which Xo cannot take itself; xo_worker and xo_fiber are read by three small C users in the second half (sched_hooks, fiber_map_failed, xo_sched_run_task) | about 100 for the record types and asserts once those three are Xo; about 60 for svc with an rt.fn_addr intrinsic (an export's address in Xo) | S for the types, M for svc |

Totals for the first half: about 110 lines go after the next seed with
no work (fault forwarders 65, str_eq and str_cmp 15, number builders 33,
personality forwarder 5, minus overlap); about 620 lines follow small
root moves (S or M); the rest, about 650 lines (contexts and locks, fault
unwinding, the C personality), waits for decision 0221 stage 2 on
x86-64 and the Windows pass.

### Challenged "by design" items

- heap_large, resize, release: not by design. libcmem.xo already
  implements malloc in Xo for XO_NOLIBC; pointing rt.heap_large at it
  on every build removes the C (28 lines) and the C library's malloc
  from the runtime's records. alloc.xo's big blocks (heapos.large) are
  page granular, so they fit the large records (fiber caches, buffers)
  but not the small ones (timers, descriptors of 48 to 320 bytes).
- personality: already Xo (xo_x_personality) wherever the Xo unwinder
  runs; what is left is the seed forwarder, Windows' handler, and the
  libgcc path's C personality, which goes with decision 0221 stage 2.
- The services record: C because only C can take an export's address.
  An rt.fn_addr[f] intrinsic (the address of a cabi export, emitted as a
  relocation by both backends) would let ctx.xo init fill it in Xo,
  removing svc, pd_svc, stop_svc and their asserts (about 150 lines
  across both halves).
- run_entry (second half) stays C by design: the unwinding's boundary
  frame (decision 0085).

## Second half (xo-4b)

xo_rt.c lines 1,701 to 3,213 on main 275a0750. Line counts come from
reading each section and from a call graph of clang's AST of the macOS
build (`clang -Xclang -ast-dump=json`); the Windows lines come from
the `_WIN32` and `XO_WINPOLL` blocks (355 of the event loop's 629).
"Seed" as above.

| Section (lines) | Lines (code) | What keeps it | Freed if the root moves | Effort |
|---|---|---|---|---|
| Clock and network forwarders (2,790 to 2,960, with the stop forwarders xo_stop_requested and xo_task_stopping at 3,113 to 3,118) | about 140 (90) | seed: generated code calls Xo since entries N1 to N3 (77d6b6dd, 70269e7e, e2d5762d) | 195 in all after the next seed, measured by deleting them from a copy that still compiles: the 25 forwarders, the helpers that lose their last caller (str_list, nc_list, nc_handle, clock_wait, canceled, rl_new, rl_data, hw, hw_slow; the last five are in the first half), 17 declarations or weak stubs, the xo_listener and xo_conn types | none |
| Layout-only record types and their asserts: xo_scope, xo_mutex, xo_atomic, xo_cq, xo_sarm and xo_select, with the asserts at 1,986 to 2,039, 2,070 to 2,076, 2,108 to 2,112, 2,132 to 2,136, 2,155 to 2,157 | about 130 (90) | nothing but their own _Static_asserts: no C reads a field of xo_scope, xo_mutex, xo_atomic, xo_cq or xo_sarm; xo_select only in xo_select_after (seed) | about 130 (select's 35 after the next seed); the Xo constants (tasks.xo S_*, mutex.xo M_*, selects.xo SE_*) stay the layouts' only definition | S |
| Scheduler services: xo_sched_rt (51 lines), xo_sched_run_task, run_claimed, run_entry, sched_hooks, fiber_counted, fiber_map_failed, svc_addr, the fibers record (1,773 to 1,790, 1,862 to 1,985) | about 150 (75) | rt.sched_rt calls from pool.xo, tasks.xo, sched.xo, twrun.xo, heapos.xo (16 call sites, 14 ops). Ops that only forward back into Xo: 2 (twrun.xo kick), 16 (tasks.xo run_claimed), 31 (deadlock). Ops on first half C: 0, 4, 5 (the st counters), 9 (oom), 19, 25 (h_release, h_free), 20 (threads_begin), 30 (tw's address). Op 18 is xo_empty_list's address. Op 29 (run_entry) sets the boundary frame and calls t->run | about 120. The forwards back into Xo go once their callers can call an Xo function across the import layering: an Xo export's address in a service word (the rt.fn_addr intrinsic the first half also names) or a direct call where no import cycle stands in the way; the rest goes with the first half's st, h_* and tw moves. run_entry: see below | M |
| Event loop, Windows: the AFD types and constants, nx_ret, nx_close, nx_shutdown, x_errno, ev_poke, afd_open, ev_init_win, pd_new's registration, pdsvc_open, close, shut and poke, the WSAPoll fallback loop poller_main, ev's Windows fields, the Windows D_* asserts | 355 (about 230) | Windows only: poll.xo's S_OPEN, S_CLOSE, S_SHUT, S_POKE and S_START_WIN service words, and the fallback when the AFD device does not open or XO_WINPOLL=wsapoll | about 150 for the WSAPoll fallback loop and the registration list it needs (S plus a decision: every supported Windows has \\Device\\Afd, and poll.xo's poll_main_win already runs the AFD path); about 180 more for afd_open, ev_init_win and the pd registration in poll.xo over sys_windows (NtCreateFile, CreateIoCompletionPort, SetFileCompletionNotificationModes, WSAIoctl SIO_BASE_HANDLE, CreateThread) | S plus a decision, then M; UNTESTED on windows (cross links only) |
| poll.xo's and stop.xo's callbacks into C: pdsvc_canceled, pdsvc_wait, pdsvc_tadd, pdsvc_tdel, poll_signal_cb, poll_stop_cb, stop_signal, pd_svc and pd_svc_init; stopsvc_cancel, chan, close, retain, release, exit and now, stop_grace_fire, entrysvc_mid_max, stop_svc and stop_svc_init (2,610 to 2,675, 2,966 to 3,020, 3,068 to 3,112) | about 130 (85) | poll.xo (svc S_CANCELED, S_WAIT, S_SIGNAL, S_STOP) and stop.xo (its S_ words) call back through the records, because they sit below tasks.xo, chan.xo and twrun.xo in the imports. Most targets are Xo already: park_wait_n, stop_signal, chan_new, chan_close and stop_cancel are exports; ctx_canceled is budget.xo canceled; tnow is poll.xo now. node_cancel, h_retain, h_release, xo_flush and _exit are first half C | about 110: the Xo targets through export addresses in the records (the same rt.fn_addr) or direct calls where the imports allow; the data words (ev.mu, ev.dead, tw.in_main, st, rtstats) move with their owners | M |
| Signal handlers: on_signal; on_console and on_overflow (Windows) (3,021 to 3,066) | about 45 (30) | stop.xo installs them from stop_svc words 13 and 14 | about 45: a handler can be a cabi.xo export (C ABI; on_signal only bumps ev.sigs and writes ev.pfd[1], both poll.xo's words, and saves errno, which Xo's raw system calls do not touch) | S with the export address |
| The event loop's records: ev, xo_pd, struct xo_timer, their asserts, the timer heap and descriptor forwarders (2,236 to 2,310, 2,465 to 2,570 without the Windows lines) | about 110 (60) | the Windows poller and on_signal (ev), pd_new on Windows (xo_pd); the C tests in std/llrt/testdata (rawpoll.c, evloop.c) call set_nonblock, pd_ready, pd_new, pd_free, ev_poke and ev_start | about 100 once the Windows poller and the handlers are Xo and those two C tests move to Xo (or go: poll.xo has Xo tests) | M |
| Dead now: deadline() (no caller), pd_wait (no caller), xo_x_net_peername and xo_x_net_ntop's declarations and weak stubs (no C caller since the C TLS went), the XO_NX_REF block they need | about 25 (15) | nothing | 25 now, no seed needed | S |
| Fibers: the ThreadSanitizer and AddressSanitizer macros, XO_NO_SANITIZE, XO_FIBER_CACHE and XO_WIN_FIBER_COMMIT (1,831 to 1,871) | about 40 (25) | first half C (TSAN_ACQUIRE and friends) and pool.xo's asserted constants | with the first half's lock and context code | none of its own |
| Entry: main, run_main, XO_RT_TEST_THREADS (3,126 to 3,213) | 88 (60) | the C library's start calls main; XO_NOLIBC builds start in xo_nolibc.c. run_main sets main's boundary frame | about 55: main can be five lines calling an entry.xo export with argc, argv and environ (heap and fault setup, the main thread's creation and join, the flush and the end reports are calls into Xo or the first half's C already); run_main's boundary as run_entry's | M |

Totals for the second half: 195 lines after the next seed with no work
(about 155 of them here, the rest in the first half); about 25 dead
now; about 130 of layout-only types (S); about 440 more after S and M
root moves, most behind one mechanism, an Xo export's address as a
word (rt.fn_addr, which needs the owner's approval as an rt intrinsic);
about 330 Windows only, half of it the WSAPoll fallback.

### Findings while reading

- heap_libc is always 0. Its `#if defined(XO_TSAN) || defined(XO_ASAN)`
  (line 220) comes before the lines that define those macros (1,834 to
  1,850), and no build passes them with -D. So heapos.xo's C library
  mapping arm (W_LIBC, rt.sched_rt ops 11 and 12) never runs, and
  xo_sched_rt has had no cases 11 and 12 since 694cc542: sanitizer
  builds map the heap with raw system calls, out of sight of the
  sanitizers' mmap interceptors. Either move the sanitizer macros above
  line 220 and restore the two ops (or make heapos.xo call mmap through
  sys_c.xo), or delete the libc arm, heap_libc and obj_heap_init's libc
  parameter (about 20 lines). TestGoldenASan net_only passes either way
  today.

### Challenged "by design" items

- run_entry and run_main (decision 0085's boundary frame): only the
  frame address is native. The arm64 and LLVM emitters already emit
  frame and stack code for fibers (rt.stack_switch, rt.fiber_start_addr,
  decision 0212), so a frame address intrinsic would move both (an rt
  intrinsic: the owner's approval). Until then about 15 lines stay C by
  design.
- main: by design only as the symbol the C library's start calls; its
  body is calls into Xo and the first half (above).
- The signal handlers: not by design; any C ABI function can be a
  handler, and cabi.xo's exports are C ABI.

## Function by function after rank 13 (xo-fa, 2026-10-15, main f5a58ac8)

The runtime C on main f5a58ac8 is 1,258 lines: xo_rt.c 1,098, xo_nolibc.c
128, xo_http.c 19, xo_std.c 13. Each block is in one class. **Seed-only** means
generated code of seed 0.13.0 (d770aa15), or std it compiles, still names it;
it can go once 0.14.0 is the seed. The check was `git grep` of the name in
selfhost/ and of the `rt.` call in std/native at f5a58ac8 and at v0.13.0,
plus selfhost/ssa/exports.xo's mapping at each. **Still used** names the root
caller and what would move it. **By design** gives the reason. Line counts
include each block's comments. Xo Lang 3's rank 8 (the heap record) was in
flight at the time and is counted under "still used" here.

### xo_rt.c (1,098 lines)

| Lines | Block | Class | Root caller, and what moves it |
|---|---|---|---|
| 324-333 (10) | xo_heap_large, xo_heap_large_resize, xo_heap_release | seed-only | 0.13.0 lowers rt.heap_* to these (0e0ce261 added the export mapping after the cut) |
| 790-794 (5) | xo_sched_run_task | seed-only | 0.13.0 has no rt.sched_run_task mapping |
| 620-638 (19) | xo_personality (Windows language handler) | dead now | no generated code names it since 69ef16a8 (before 0.13.0); fault_raise_msg's pers_old can be xo_x_personality on Windows too |
| 748-777 (30) | XO_TSAN, XO_ASAN, XO_NO_SANITIZE | dead now | defined, never used |
| 856-858 (3) | XO_ATOMIC_V | dead now | defined, never used |
| 251-254 (4) | xo_oom | test-only | only the C tests call it: move to testdata/xrec.h |
| 338-343 (6) | xo_chunks_now | test-only | tasks.c, alloc_threads.c |
| 381-392 (12) | XO_RC_IMMORTAL, rc_load, rc_inc, rc_dec | test-only | the C tests |
| 896-899 (4) | ev_poke | test-only | evloop.c |
| 935-961 (27) | xo_listener, xo_conn and asserts | test-only | evloop.c's view of netconn.xo's handles |
| 135-249 (115) | the heap record xo_heap, its asserts, central lists, main_heap, tls_heap | still used | alloc.xo and obj.xo through obj_heap_init; Xo Lang 3's rank 8 moves it (in flight) |
| 256-322 (67) | heap_slow, heap, hw_slow_c, obj_heap_init, obj_heap_words, xo_rt_threads_begin | still used | main, the test constructor, xo_sched_rt op 20; rank 8 |
| 1020-1025 (6) | entrysvc_mid_max | still used | entry.xo (stop_svc word 15) writes main_heap.mid_max; rank 8 |
| 409-417 (9) | xo_flush | still used | generated code: std fsops.xo rt.flush, which lowers to xo_flush (no exports.xo mapping). Move: map rt.flush to xo_x_flush |
| 1049-1052 (4) | xo_set_grace | still used | generated code: the SSA builder's rt.set_grace for Proc.set_grace. Move: map rt.set_grace to xo_x_stop_set_grace |
| 451-483 (33) | xo_rl_copy, xo_rl_grow, the xo_rstr and xo_rlist types | still used | the LLVM emitter (call.xo, func.xo, gen.xo's head) calls them. Move: call obj.xo's rl_copy_n and rl_grow_n exports as arm64 does |
| 526-547 (22) | xo_hdr, xo_zero_handle, xo_empty_list | still used | data symbols whose addresses both emitters take (E3). Move: the compiler emits them (as xo_tlocal), or static words |
| 423-449 (27) | fault_init and the init declarations | still used | main and the test constructor. Move: entry.xo main does the inits (ctx, poll, stop) itself |
| 577-618 (42) | fault_raise_msg | still used | fault.xo's RAISE word. Move: fault.xo fire calls cabi.xo fault_raise_cur with rt.fn_addr of xo_x_personality |
| 704-717, 796-816 (35) | run_claimed, xo_sched_rt | still used | pool.xo's rt.sched_rt ops 2 (tw kick), 16 (run claimed), 18 (empty list), 20 (threads begin). Move: the ops to exports or Xo calls; op 16's TSan acquire through sanhook.xo's weak __tsan_acquire (decision 0232) |
| 780-788 (9) | ctx_init | still used | tells ctx.xo whether the C was built with TSan. Move: sanhook.sanitized() at run time (decision 0232) |
| 394-407, 875-894 (34) | weak stand-ins: xo_x_tw_*, nc listener and conn free | still used | the C's services records name them. Move: the Xo callers test rt.sym_present (decision 0232) |
| 901-903, 913-922, 963-991, 1013-1019, 1027-1047 (69) | ev_start, pd_svc_init, environ, the handlers' declarations, the entry services, stop_svc_init | still used | stop.xo and entry.xo read stop_svc. Move: rt.fn_addr of the exports and Xo inits (decision 0233's pattern) |
| 549-575 (27) | TSAN_ACQUIRE annotation | by design | ThreadSanitizer annotation (the C is instrumented; it goes with op 16 if sanhook's weak call is used) |
| 992-1011 (20) | on_overflow | by design | the Windows stack overflow handler: runs on the thread's guarantee, after the guard page |
| 1054-1089 (36) | main | by design | the C library's start calls `main` (Windows: _setmode too); its body is Xo calls. Could be a bare-named export, as cabi_libc.xo's |
| the rest (423) | the header, includes, Windows Winsock and errno asserts (OS header layouts sock.xo relies on), section comments of what is Xo now, the xo_std.c and xo_nolibc.c includes | overhead | shrinks with the code; the Winsock asserts could move to a test that compiles them |

### xo_nolibc.c (128 lines): by design

| Lines | Block | Reason |
|---|---|---|
| 1-48 | header, declarations, the data symbols environ, stdin, stdout, stderr | data symbols the C library's users name |
| 50-78 | nl_run, xo_nolibc_start | the thread pointer (msr tpidr_el0) and the switch to main's 1 GiB stack (asm) |
| 80-92 | _start | asm: the initial stack pointer |
| 94-128 | snprintf, sprintf, fprintf, printf | variadic (va_start); they hand the va_list to libcio.xo |

### xo_http.c (19) and xo_std.c (13)

| Block | Class | Root caller, and what moves it |
|---|---|---|
| xo_x_streams_free (weak stand-in) | still used | entry.xo main's end. Move: rt.sym_present in entry.xo |
| xo_http.c's last line, "A std function's fault at its caller: generated code calls this name." | dead | a stale comment with no function after it |
| xo_std.c: the include of xo_http.c and comments | overhead | goes with xo_http.c |

### Totals

| Class | Lines |
|---|---|
| seed-only (goes after 0.14.0) | 15 |
| dead now (delete) | 52, plus 1 stale comment line |
| test-only (move to testdata/xrec.h) | 53 |
| still used: the heap record (rank 8, in flight) | 188 |
| still used: generated code names it (needs an emitter or mapping change) | 68 |
| still used: C glue to Xo (movable now) | 216 + 19 (xo_http.c's stand-in) |
| by design | 83 in xo_rt.c + 128 xo_nolibc.c |
| overhead (includes, asserts, comments) | 423 + 13 (xo_std.c) |
| **total** | **1,258** |

### After 0.14.0

The four "generated code names it" items have to land before 0.14.0 is cut for
their C to go after it: the rt.flush and rt.set_grace mappings, LLVM's
rl_copy and rl_grow through obj.xo's exports, and the placeholders as emitted
data. If they do, and rank 8 and the movable glue land too, what is left after
0.14.0 is the by-design C:
- xo_rt.c: main, on_overflow and the TSan annotation (83 lines, about 40
  of them code). The annotation goes too if op 16 uses sanhook.xo's weak call.
- xo_nolibc.c: 128 lines.
- The includes and Windows asserts those need: about 150 lines.

That is about 350 lines in all, down from 1,258. Without the four emitter
changes before the cut, their 68 lines and the forwarders stay one release
longer (until 0.15.0).
