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

(merged once both halves are in)

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

(in progress)
