# Raw memory intrinsics and C ABI exports (decision 0207, stage 1)

Date: 2026-10-13. Branch of the rawmem agent; base aea0c65a (the ADR
stub), merged with origin/main at f0ab329b (atomics, decision 0200;
scan intrinsics, decision 0201). Instruction counts are whole process
instructions retired, minimum of 5 runs, `/usr/bin/time -l`, on the
development Mac with other agents running (counts, not times, are
reported for that reason).

## What landed

- **C ABI exports** (decision 0204's mechanism): every top level
  function of `std/native/llrt/cabi.xo` is called by the C runtime as
  `xo_x_<name>`; `cabi_<feature>.xo` modules (`cabi_tls.xo`,
  `cabi_time.xo`) hold exports only one feature's C calls. Int and Float
  parameters (an address is an Int), an Int, Float, Str, or no result.
  The SSA builders (internal/ssa `declareExports`, selfhost/ssa
  `declare_exports`) build them as roots after the program's code: cabi
  always, a feature's when code of a module importing it was built. The
  emitters give each a global symbol: selfhost/arm64 and internal/arm64
  a branch to the function (Xo functions follow AAPCS64 for Int and
  Float words) or, for a Str result, a frame that stores the length
  through the pointer after the parameters; selfhost/llvm and
  internal/llvm an external C convention function that calls the
  internal one. The wasm backend builds none (`Builder.wasm`).
- **Raw memory intrinsics** in `std/native/rt.xo`: `mem_load`,
  `mem_store` (constant width 1, 2, 4, 8, checked by both SSA builders),
  `mem_copy`, `mem_move`, `mem_set`, `mem_cmp`, `layout` (folded by the
  SSA builders from one table, internal/ssa `Layout` and selfhost/ssa
  `layout_value`, native and wasm columns), the generic `obj_addr[T]`
  (Str, List, handle) and `obj_at[T]` (List, handle; one retain), and the
  address form of the atomics of decision 0200 (`slot_*[Int](a, i, ...)`
  is the word at a + 8 * i). Lowered inline in selfhost/arm64,
  selfhost/llvm, selfhost/wasm (rt2.xo functions and `memory.copy`,
  `memory.fill`), internal/arm64, internal/llvm; memcpy, memmove,
  memset, memcmp are platform library calls on arm64 and LLVM intrinsics
  (memcmp a call) on LLVM. `rs_set_len` is shared with decision 0201.
- Used at run time by the moved code: mem_load (widths 1, 2, 4, 8),
  mem_store (1, 2, 4, 8), mem_copy, mem_move (fmt_int), mem_cmp
  (raw.mem_eq), layout, obj_addr, rs_set_len. mem_set (raw.fill),
  obj_at, and the atomics at an address are lowered by every emitter,
  but no runtime module calls them
  yet, so no test runs them; stage 2 is their first user.
- `std/native/llrt/raw.xo` (text to and from C memory),
  `netpolicy.xo` (Net.only's address policy), `tlsname.xo` (x509 name
  checks, net.ParseIP and IP.String, PEM decoding, alert text).

## The "(0) held by C callers" group (audit: 1,806 lines)

| Audit group | Lines | Now |
|---|---|---|
| xo_rt.c fmt_int, fmt_go_g | 123 | Xo (cabi `fmt_int`, `fmt_float`; num.xo `go_g_text`) |
| xo_rt.c fault messages | 75 | texts in Xo (cabi `fault_*`); the C keeps the entry points that raise |
| xo_rt.c host:port split, address policy | 215 | Xo (netpolicy.xo); std/native/http.xo calls it directly, `rt.net_admits` deleted |
| xo_std.c nstr, nbuf, UTF-8, is_space | 164 | UTF-8 decode and encode and IsSpace in Xo; nstr and nbuf stay (C's own buffers, they go with their last C caller) |
| xo_std.c n_parse_int | 29 | Xo (cabi `parse_int`) |
| xo_std.c n_fmt_duration | 127 | the runtime's callers use Xo (cabi `fmt_duration`); the C copy only under `XO_TEXT_C` (the C path of text, decision 0194) |
| xo_std.c n_fmt_time | 47 | likewise (cabi `fmt_time`; C only under `XO_TEXT_C`) |
| xo_std.c b64_val, n_days_from_civil | 32 | Xo |
| xo_bytes.c fault_bytes_index | 16 | text in Xo |
| xo_time.c zip directory reader | 57 | Xo (cabi_time `tz_zip_entry`) |
| xo_tls.c PEM decode | 160 | Xo (tlsname `pem_next`); ends_with and types_list (16 lines of X509KeyPair's error text) stay |
| xo_tls.c hostname and IP matching | 189 | Xo (tlsname `host_ok`, `parse_ip`); names_load stays (it reads Mbed TLS) |
| xo_tls.c certificate time and hostname messages | 102 | hostname_msg in Xo; rfc3339 and x509_time stay (localtime_r, timegm: (3)) |
| xo_tls.c alert text | 58 | Xo (tlsname `alert_text`) |
| xo_testing.c test output, env, exit code, log records | 185 | not moved: C state under C locks (see below) |
| xo_testing.c fakes, records, memfs | 151 | not moved: likewise |
| xo_proc.c fake processes | 76 | not moved: likewise |

`git diff --numstat aea0c65a bcde613c -- std/llrt`: 883 lines deleted,
151 added (the export declarations and the C shells that call them), so
std/llrt is 732 lines shorter; another 176 (n_fmt_duration 127 and
n_fmt_time 49 with their comments) are compiled only with
`XO_LLRT_C=text`. The three groups not moved (412 lines) are stores of
the test world (records, fakes, memfs, fake processes) that C reads
and writes under its own mutexes: they hold state, which Xo cannot keep
outside a value until (2)'s allocator work gives it a home and (1)'s
atomics guard it, so the audit's "(0)" is (1) and (2) for them.

## Behavior

- Fault messages are the same byte for byte: TestCABIDiff (new,
  internal/llvm cabi_diff_test.go) runs 1,690 input lines (Int faults at
  the bounds and random, Float to Int faults over Go's shortest texts,
  Float32 text, UTF-8 of random valid and invalid bytes, white space,
  Net.only lists and dials of loopback literals) on the Go backend, arm64
  and LLVM: 8,760 output lines, identical on both native backends.
- TLS name checks now follow Go exactly: net.ParseIP's rules (no leading
  zeros in IPv4 fields; the C used the platform's inet_pton) and no 256
  byte limit on a name (the C copied names into 256 byte buffers).
- Rune.is_space is Xo (unicode.is_space) instead of a C call;
  `rt.rune_is_space` and `xo_rune_is_space` are deleted.

## The bar (decision 0193 item 5)

bench/perf/text and bench/perf/httpparse, base f0ab329b (origin/main
before the merge) against this branch, instructions retired, new over
base:

| Workload | arm64 | LLVM |
|---|---|---|
| text none (setup) | 1.000 | 1.000 |
| rune_count | 0.996 | 0.995 |
| trim (after unicode.is_space tests the ranges first; first version 1.040 and 1.037) | 0.978 | 0.985 |
| split | 1.000 | 0.999 |
| lines | 1.000 | 1.000 |
| utf8 | 0.999 | 1.000 |
| int_fmt | 0.998 | 0.997 |
| float_fmt | 1.000 | 1.000 |
| time_fmt | 1.000 | 1.001 |
| dur_fmt | 0.999 | 1.002 |
| parse_int | 0.999 | 1.001 |
| httpparse none | 0.997 | 1.003 |
| h1_get | 1.006 | 1.008 |
| h1_chunked | 0.997 | 1.000 |
| h2_get | 1.000 | 1.000 |

Every workload is within 5 percent; checksums equal.

## Costs

- Every native program carries cabi.xo's closure (Dragonbox Float text
  for the Float to Int fault and Float32 text, Duration and Time text,
  Net.only's policy). A hello world on arm64: 363,720 to 384,440 bytes
  (+20,720; text segment 262,144 to 278,528), base aea0c65a; 403,032
  bytes before the TLS and zone exports moved to feature modules. On
  LLVM 106,344 to 106,456 (+112: LTO drops what the C runtime's reachable
  code does not call).
- The SSA golden of fib has 84 functions (9 before); `go test -short
  ./internal/ssa` took about 90 s instead of about 9 s on the loaded Mac
  (gotchas/exports-grow-every-program.md).

## Tests run

- `go test -short` over the repository (exclude runs/): pass.
- EC2 (r8g, on demand, instance i-EXAMPLE27, terminated), commit
  cb9207d3: internal/llvm TestGolden, TestGoldenWant, TestTLSDiff,
  TestTZDiff, TestRuntime*, TestCryptoDiff, TestHTTPParseDiff,
  TestHTTPFuzz, TestLeakCheck, TestH2Diff, TestHTTP1ParseDiff,
  TestJSONTextDiff, TestIntsDiff, TestNumDiff, TestDigestDiff,
  TestProcSpecDiff: ok (171 s); internal/arm64 TestGoldenRun, TestRun,
  TestBench: ok (34 s), linux/arm64.
- TestSelfhostWasm: 132 pass, 19 unsupported, 0 fail of 151 goldens; after merging origin/main at 28cc2052, 132 pass, 20 unsupported, 0 fail of 152.
- TestCABIDiff on the Mac: pass (arm64, LLVM). TestTLSDiff and
  TestTZDiff on the Mac: pass.
- TestSelfhostSSA, TestSelfhostArm64, TestSelfhostLLVM with
  `XO_SELFHOST_DIRS=net_only,http_tls,tz_table,fault` (30 runs, go
  backend): the Go SSA builder's 3,728 functions identical, the Go arm64
  emitter's 3,731 functions identical, every LLVM run byte identical.
- Selfhost parity by hand (the self hosted emitters' output against
  `xo asmdump`/`xo llvmdump`): identical on hello, the fault program,
  the text program, http_tls, net_only.

## Next, ranked

1. Make the always compiled C callers of the heaviest exports follow use:
   the Float to Int fault and Float32 text called by generated code
   directly (the SSA builders already route Float text to num.xo), so
   Dragonbox leaves programs without Floats; then the http log time.
2. The test world stores (412 lines) after (1) and (2): a runtime state
   record in Xo reached by address (obj_at of a handle the C allocates
   once), guarded by slot_* atomics.
3. Decision 0207's design: reference counts, then the allocator, in Xo.

## Stage 2 (2026-10-13): zero filled lists and reference counts in Xo

rawmem2 agent; base 96225edd (stage 1 merged). Same method: whole
process instructions retired, `/usr/bin/time -l`, development Mac
shared with other agents, Go driver (`XO_SELF_DRIVER=0`). "less setup"
is (workload - none) of the Xo build over the same for the C build.

### 1. `raw.zero_ints` and the Argon2id rerun

`std/native/llrt/raw.xo` `zero_ints(n)`: `List[Int].with_capacity(n)`,
one `rt.mem_set` of the elements, the length stored with `rt.mem_store`
(8 bytes native, 4 on wasm, by `rt.layout(6)`), so a List of n zeros is
one allocation and a memset instead of n pushes (calloc's role in the
C). Used by std/native/archive.xo's `zeros` (deflate's hash_head and
hash_prev, 32 Ki words each, and the Huffman tables),
std/native/regexp.xo's `new_queue`, and jsontext.xo's key table.

The Argon2id in Xo (commit a4168f52's digest.xo and digest_tables.xo,
C by default since d451d306) measured in a scratch copy of this tree
with its two `zeros` calls replaced by `raw.zero_ints`
(bench/perf/digest/measure.sh, `WORKLOADS="none argon2"`):

| Build | arm64 argon2 | arm64 none | LLVM argon2 | LLVM none |
|---|---|---|---|---|
| Xo, zeros by pushes | 224,399,431 | 22,005,099 | 245,258,563 | 19,262,485 |
| Xo, raw.zero_ints | 198,229,931 | 21,954,017 | 227,344,407 | 19,161,821 |
| C (`XO_LLRT_C=digest`) | 201,837,093 | 22,006,180 | 199,525,134 | 19,230,110 |

Less setup over C: pushes 1.126 arm64, 1.253 LLVM (the 1.125 and 1.254
of arm64-codegen-2026-10-13.md); **raw.zero_ints 0.980 arm64, 1.155
LLVM**. The zero fill cost 26.2 million instructions on arm64 and 17.9
million on LLVM. Checksums equal on every build.

**Decision 0180's bar (both backends within 5 percent of C) is not met:
LLVM is 1.155.** Argon2id and BLAKE2b stay C by default; the Xo version
stays at commit a4168f52 (plus `raw.zero_ints` for its zeros). The
remaining LLVM cause, by disassembly of the LLVM build: argon2_g's
unrolled xor loops are scalar (two loads, an eor, two stores a word,
no vector instruction), while the C's cr_argon2 has 115 vector
instructions; LLVM cannot prove the three `var List[Int]` parameters
(bs, t, q) distinct, so the SLP vectorizer leaves them
(gotchas/llvm-list-params-block-vectorizing.md).

### 2. Reference counts in Xo

`std/native/llrt/rc.xo`: `inc`, `dec`, `dec_shared`, `claim`, `mark`
over the count word at the object's address, decisions 0046 and 0058
unchanged: a positive count is thread local (a relaxed `slot_load`, a
plain `mem_store` of the count plus or minus one by `int_wrap`), a
negative count with bit 62 set is shared (`slot_add` of -1 relaxed for
a retain, of 1 acq_rel for a release, the last when it returned -1),
INT64_MIN is immortal (never written); a claim takes a thread local 1,
or a shared -1 read again with acquire order (then stores 1); a mark
negates a thread local count. cabi.xo exports them (`xo_x_rc_inc`,
`xo_x_rc_dec`, `xo_x_rc_dec_shared`, `xo_x_rc_claim`, `xo_x_rc_mark`),
and xo_rt.c's rc_inc, rc_dec, rc_dec_shared, rc_claim, rc_mark (and the
public xo_rc_inc, xo_rc_dec, xo_rc_claim, xo_rc_mark rc_threads.c
calls) are calls of them; rs_append's release of its consumed string
is rc_dec now (it decremented the count by hand). Generated code keeps
its own inline retain and release (both emitters); xo_bytes.c's
`c == 1` fast paths only read the count.

The C path stays selectable: `XO_LLRT_C=rc` loads rc_c.xo, which calls
`rt.rc_*_c` (xo_rt.c under `-DXO_RC_C`, which both drivers add when the
generated code calls them), and the C then uses its own copies.

The no allocation, no counting rule (ADR 0207 design 3): every function
of rc.xo takes and returns Int words, uses no Str, List, closure, or
interpolation, no arithmetic that can fault, and calls only inline
intrinsics. selfhost/ssa contract.xo `check_no_alloc` checks each
function of native/llrt/rc as its body is built: only Int, Float,
Bool, or Unit values, no fault, closure, capture, box, or call of a
function value, no unwind block, and calls only of `rt.mem_load`,
`rt.mem_store`, `rt.int_wrap`, `rt.slot_*`, `rt.fence`,
`rt.spin_hint`, `rt.rc_*_c`; a violation is an unsupported construct
(a build error of std). A call of another function is refused too: the
builders add a landing check after every call while building
(gotchas/call-adds-landing-check.md), so `dec` carries dec_shared's one
atomic itself.

On arm64 the C reaches each count operation through a call of the
export, which branches to the Xo function (a thread local claim runs 7
instructions there, plus the call and the branch, where the C's copy
was inline); on LLVM the exports are LTO inlined into the C
(no `xo_x_rc_*` symbol is left in a linked program).

Instructions, Xo counts over `XO_LLRT_C=rc`, minimum of 5 runs (every
checksum equal):

| Workload | arm64 | LLVM |
|---|---|---|
| nested push | 1.008 | 1.000 |
| nested setnested | 1.002 | 1.002 |
| nested setdeep | 0.995 | 1.001 |
| nested mapput | 1.001 | 1.001 |
| nested strappend | 1.005 | 1.002 |
| nested method | 1.001 | 1.003 |
| nested shared | 1.010 | 1.001 |
| nested walk | 0.998 | 0.999 |
| nested index2 | 1.003 | 1.000 |
| perf (bench/perf/xo, 18 workloads) | 0.999 to 1.002 | 0.999 to 1.001 |
| text quote (less setup) | 1.021 | 1.000 |
| text replace (less setup) | 1.022 | 1.000 |
| text debug (less setup) | 1.037 | 1.000 |
| text split, lines, int_fmt, float_fmt | 1.001 to 1.002 | 1.000 |
| json parse, maps | 1.001, 1.006 | 1.000 |
| json strings (less setup) | 1.016 | 1.000 |
| json enc_strings (less setup) | 1.025 | 1.001 |
| conc spawn, fanout, rwmutex, pmap (XO_THREADS=1, min of 11) | 1.000, 1.005, 1.004, 1.000 | 0.996, 0.996, 0.994, 1.000 |
| conc mutex (XO_THREADS=1, median of 21) | 1.021 | 0.985 |

The conc programs' counts vary by up to 10 percent between runs of one
binary (mutex arm64: 130.4 to 158.0 million over 21 runs), with the
default pool more (one 5 run pass gave rwmutex arm64 0.806 and fanout
1.067); those lines are medians or minimums of 11 to 21 runs with one
worker. The largest steady cost is the string building workloads on
arm64 (text debug 3.7 percent above setup): rs_append's claim and
release are calls there.

**The bar (within 5 percent on both backends) holds.** The Xo counts
are the default; `XO_LLRT_C=rc` stays until a full tier passes on the
combined main.

Correctness:

- `std/llrt/testdata/rc_threads.c` gains `model`: 8 threads, 64 shared
  strings, 16,000 random retains, releases, claims, and marks per thread
  (ITER 2000), each checked against what the thread knows of its own
  references (a release while holding another is not the last; a claim
  fails while holding two and leaves a thread local 1 when it succeeds;
  a mark of a shared count reports shared), and every object released
  exactly once as the last reference with its text intact.
  TestRuntimeThreads runs the whole file under ThreadSanitizer for the
  Xo counts and for `-DXO_RC_C`: pass (-short, the Mac). Two mutations
  of rc.xo fail it under ThreadSanitizer: the last release with relaxed
  order instead of acq_rel (a race between the freeing thread and a
  retain), and claim without its acquire reread.
- The smoke program (a 1,000 step string append, a list copied on
  write) on arm64 and LLVM, both paths: same output, `xo leakcheck: 0
  live of 1116 allocations` each.

### What the allocator needs (not started)

- The heap pointer: `__thread` in C (tls_heap) and `solo_heap` for one
  thread; Xo needs `rt.tls_get/tls_set` (decision 0195 prerequisite 6)
  or a C accessor export of one word until then.
- Size classes and the bump chunk: Int arithmetic over the heap record's
  words (mem_load/mem_store at offsets of a record the C allocates
  once, or obj_at of a handle), free lists as intrusive next pointers
  (mem_load/mem_store at the block), the central lists under the C
  mutex (a C export of lock and unlock) until slot_cas replaces it.
- Large blocks and chunks: malloc and free stay platform calls (an rt
  intrinsic for each, or the C wrappers kept as the leaf).
- The free walkers (xo_rl_free, xo_rc_free's real-time deferral,
  xo_rc_defer's worklist) call element callbacks by address: Xo has no
  call of a function address; an intrinsic `rt.call_addr1(f, a)`
  (lowered to blr / an indirect LLVM call) is the missing piece, then
  the walkers move next to rc.xo.
- The leak counters (allocs, frees) as heap record words; xo_rt_live
  sums them under heaps_mu.
- The same no allocation, no counting rule, extended to the allocator's
  module (check_no_alloc keyed on a list of module paths).

### Tests run (stage 2)

- `go test -short` over the repository with `XO_SELF_DRIVER=0`: pass
  (internal/ssa after its goldens were regenerated). The first run,
  without `XO_SELF_DRIVER=0`, timed out in cmd/xo building xo1
  (gotchas/cmd-xo-short-builds-xo1.md) and hit TestHybridPreemption's
  load flake.
- TestSelfhostWasm: 132 pass, 20 unsupported, 0 fail of 152, after
  fixing selfhost/wasm rt2_direct (a merge had made rt.mem_* unsupported
  on wasm: 127 pass before the fix,
  gotchas/merged-is-list-dead-statement.md).
- The Mac (darwin/arm64): internal/arm64 TestGoldenRun (every golden,
  leak checked), internal/llvm TestCABIDiff and TestRuntimeThreads:
  pass; targeted TestGolden and TestGoldenRun of archive_formats,
  json_text, native_json, regexp_table (the zero_ints callers): pass.
- SSA and arm64 assembly goldens regenerated (the five rc exports and
  cabi.xo's one line shift).
- EC2 (r8g.8xlarge, on demand, instance i-EXAMPLE21,
  terminated), merge commit 2ae37a1e, linux/arm64: internal/llvm
  TestGolden (every golden, leak checked), TestGoldenThreads,
  TestGoldenShared, TestRuntimeThreads (ThreadSanitizer, both paths),
  TestRuntimeTasks, TestCABIDiff, TestLeakCheck: ok (101.6 s);
  internal/arm64 TestGoldenRun, TestGoldenRunShared: ok (18.3 s);
  cmd/xo TestNoGCTests through xo1 (built from these sources, so
  check_no_alloc ran on rc.xo): ok (690.9 s). An earlier instance
  (i-EXAMPLE36) could not connect (the shared SSH key had been
  emptied) and was terminated by the script.

## Stage 3 (2026-10-13): the heap in Xo

alloc agent; base 3f8f6ac2 (stage 2 merged). Same method: whole process
instructions retired, minimum of 5 runs (conc: 11, XO_THREADS=1),
`/usr/bin/time -l`, development Mac shared with other agents, Go driver
(`XO_SELF_DRIVER=0`), commit 69b80fa1 (e91211b7 changes only the C
version guard of `XO_INLINE_CALL`, the same C on the Mac). "less setup"
is (workload - none) of the Xo build over the same for the C build.
The C path is `XO_LLRT_C=alloc` (-DXO_ALLOC_C) of the same commit.

### What moved

`std/native/llrt/alloc.xo`: xo_rt.c's alloc_in, free_in, the magazine
steps of alloc_mt and free_mt (spare magazine, central_take,
central_put), and xo_resize, over the raw memory intrinsics and the
slot atomics at an address. cabi.xo exports them as `xo_x_heap_alloc`
(n, h), `xo_x_heap_free` (p, h), `xo_x_heap_refill`,
`xo_x_heap_free_mt`, `xo_x_heap_resize`; the C keeps its entry points
(xo_alloc, xo_free, xo_resize, rs_new) and calls them with the calling
thread's heap record. What stays C:

- Finding the heap record: `solo_heap` (one thread) and the `__thread`
  `tls_heap` (heap_slow), heap creation, the spare heaps, the list of
  all heaps under heaps_mu, and xo_rt_live's sum. Xo has no thread
  local or mutable global word; the record's address is an argument
  (ADR 0207 stage 3 designs `rt.tls_get`/`rt.tls_set` and a static
  word table for the step after).
- The platform calls, as `rt.heap_*` C leaves: malloc above the classes
  (`heap_large`, which writes the prefix), realloc (`heap_large_resize`),
  free (`heap_release`), a new 4 MiB chunk (`heap_grow`: installs the
  chunk, counts it for tasks.c, and calls `xo_x_heap_alloc` again), and
  the central lists' pthread mutex (`heap_lock`, `heap_unlock`).
- The heap record stays a C struct; its first words (counters, bump
  chunk, real-time depth and allocations, mid_max, the central lists'
  address, the last block's usable size, then the per class free list
  heads, lengths, and spare magazines) are at the offsets of alloc.xo's
  constants, checked by `_Static_assert` in xo_rt.c.

Changes to the C's design, each measured:

- `usable` (a string's capacity) was a second call per string; alloc
  and resize leave the usable size in the record (`last_size`), which
  rs_new and rs_append read after the call (`alloc_usable`,
  `resize_usable`). The C path keeps its usable().
- xo_resize's move (allocate, copy, free) is one Xo call on one thread
  (alloc's pop or carve and free's push written out in resize, since
  the module calls no functions); with threads, or a block to or from
  malloc, or an empty chunk, the C moves it as before. bench/perf/alloc
  maps went from 1.046 to 0.991 on arm64 with it.
- The magazine size test `nfree >= max(65536 / size, 8)` is `(nfree + 1)
  * size > 65536` (no division, which can fault); a magazine taken
  counts `65536 / size` blocks by a long division loop.
- Every call is last with nothing live after it, so arm64 keeps no
  callee saved register (gotchas/arm64-call-anywhere-costs-frame.md).

New internal pieces: `rt.prefetch(a)` (prfm pldl1keep on arm64,
llvm.prefetch on LLVM, nothing on wasm; the pop's prefetch of the next
link, gotchas/native-heap-cold-free-lists.md), and both arm64 emitters
lower `rt.mem_load`/`mem_store` with a constant offset to the scaled
immediate form and `rt.int_wrap` add and subtract of a constant below
4096 to the immediate form (the Go emitter mirrors it for
TestSelfhostArm64). On LLVM the C marks the calls of the single
threaded exports `[[clang::always_inline]]` (clang 17, Apple clang
16): LTO left `xo_x_heap_alloc` out of line in the hot string builders
(text int_fmt 1.077 and float_fmt 1.054 less setup before,
gotchas/lto-keeps-export-out-of-line.md).

The no allocation rule: selfhost/ssa contract.xo `check_no_alloc` runs
on native/llrt/alloc as on native/llrt/rc, allowing also `rt.prefetch`,
`rt.mem_copy`, `rt.heap_*`, and the C path's `rt.alloc_*_c`.

### The bar (decision 0180): bench/perf/alloc, nested, xo, text, json, conc

bench/perf/alloc (new, `compare.sh` builds both paths): small (2
million one element lists), mixed (a ring of 4,096 lists of random
lengths up to 3,000, replaced at random: every class and malloc), grow,
strings, maps, tasks (32 tasks a round allocating on the workers).

| Workload | arm64 | LLVM |
|---|---|---|
| alloc small (less setup) | 1.040 | 0.876 |
| alloc mixed | 0.987 | 0.954 |
| alloc grow | 0.996 | 0.990 |
| alloc strings | 0.984 | 0.932 |
| alloc maps | 0.991 | 0.935 |
| alloc tasks | 1.033 | 0.952 |
| nested (rc-borrow's 9 workloads) | 0.993 to 1.009 | 0.980 to 1.006 |
| perf (bench/perf/xo, 18 workloads) | 0.999 to 1.002 | 0.990 to 1.003 |
| text quote (less setup) | 0.977 | 0.943 |
| text split | 0.981 | 0.882 |
| text lines | 0.984 | 0.908 |
| text replace | 0.998 | 0.966 |
| text int_fmt | 1.029 | 1.013 |
| text float_fmt | 1.011 | 1.009 |
| text debug | 0.990 | 0.973 |
| json parse (less setup) | 1.001 | 1.002 |
| json maps | 0.993 | 0.961 |
| json strings | 0.996 | 0.973 |
| json enc_strings | 1.006 | 1.005 |
| conc spawn (XO_THREADS=1, min of 11) | 1.035 | 0.979 |
| conc fanout | 1.014 | 1.001 |
| conc pmap | 1.000 | 1.000 |

Every workload is within 5 percent on both backends; checksums equal.
The worst are arm64's: alloc small 1.040 (a pop and a push per
iteration through C wrapper, export branch, and the Xo function's frame
record, about 6 instructions more a pair than the C's inline code),
conc spawn 1.035, alloc tasks 1.033. On LLVM the Xo heap is below the
C on every allocation workload (LTO inlines it; the C path re-derives
the size class for usable()). Next for arm64: a tail call in the
arm64 emitters (free's only call is last) and the export symbol as an
alias of the function instead of a branch.

### Correctness

- `std/llrt/testdata/alloc_threads.c` (TestRuntimeAlloc, both paths,
  ThreadSanitizer, then -O2 with ITER 30000): solo (one thread, random
  sizes over every class, the mid classes and malloc, sizes 0 to 7,
  resizes, rs_new's capacity), threads (8 threads, blocks sent to other
  threads' mailboxes, checked, resized, and freed there), waves (one
  thread allocates, two new threads free, n rounds, at most n / 10 + 3
  chunks carved where without the central lists a round needs 0.21 of
  a chunk; the first bound, 4 whatever n, failed the -O2 run on EC2
  with 19 chunks in 300 rounds, the dead freeing heaps' magazines).
  Every block's contents are a pattern of its tag, checked
  before each resize and free; every test ends with 0 live. A mutation
  (resize copying half of the kept bytes) fails it with 44 checks.
- Mac, -short: TestRuntimeAlloc, TestRuntimeThreads, TestRuntimeTasks,
  TestCABIDiff pass; `go test -short` over the repository passes.
- TestSelfhostWasm: 132 pass, 20 unsupported, 0 fail of 152 (as stage 2).

### xo1 on the heap in Xo (EC2)

EC2 r8g.8xlarge, on demand, linux/arm64, clang 15.0.7, one instance
(i-EXAMPLE51, terminated), commit e91211b7. Two xo1 built by
the Go seed (`XO_SELF_DRIVER=0 xo build --backend=arm64 selfhost/xo`):
the heap in Xo, and `XO_LLRT_C=alloc`. Each compiled selfhost/xo
(`xo1 asm --phase-times --std=std selfhost/xo`, `XO_LEAKCHECK=1`,
`perf stat -e instructions,cycles`, GNU time), alternating, twice:

| | Xo heap run 1 | C run 1 | Xo heap run 2 | C run 2 |
|---|---|---|---|---|
| allocations (all freed) | 1,952,597,859 | 1,952,597,859 | 1,952,597,859 | 1,952,597,859 |
| instructions (user) | 2,235.93 billion | 2,227.79 billion | 2,235.93 billion | 2,227.79 billion |
| cycles (user) | 925.75 billion | 939.25 billion | 890.63 billion | 934.33 billion |
| wall clock | 5:38.34 | 5:43.41 | 5:25.65 | 5:41.58 |

Instructions 1.004 of the C's, cycles 0.986 and 0.953, wall clock
0.985 and 0.953; the assembly both write is byte identical (sha256
6af248b1...2e7a). Building each xo1: 10:43.33 (Xo heap) and 10:47.51
(C), 17.5 GB maximum resident set each.

The fixed point with xo1 on the heap in Xo: scripts/bootstrap-xo.sh
with the Xo heap xo1 as the seed, no Go on PATH: s1 433 s, s2 419 s,
s3 427 s; s2 and s3 assembly identical (441,579,991 bytes) and
binaries identical (86,383,200 bytes), s1 equals s2, s3 builds and
runs hello.

### Tests run (stage 3)

- Same EC2 run: internal/llvm TestGolden (every golden, leak checked),
  TestGoldenWant, TestGoldenThreads (four threads, ThreadSanitizer),
  TestGoldenShared, TestRuntimeThreads, TestRuntimeTasks, TestCABIDiff,
  TestLeakCheck: pass; TestRuntimeAlloc: its ThreadSanitizer runs
  passed, the -O2 run of 300 rounds failed the first waves bound (19
  chunks, above), fixed in 5e2ec59b and passing in full (both paths,
  ThreadSanitizer and -O2) on the Mac. internal/arm64 TestGoldenRun,
  TestGoldenRunShared, TestGoldenAsm: pass (19.9 s). internal/selfhost
  TestSelfhostSSA, TestSelfhostArm64, TestSelfhostLLVM (the Go and
  self hosted emitters identical, with both arm64 emitters' new
  immediate forms): pass (266.4 s). cmd/xo TestNoGCTests through xo1
  built from these sources (so check_no_alloc ran on alloc.xo): pass
  (690.2 s).
- An earlier instance (i-EXAMPLE13, commit 69b80fa1) failed
  every C runtime build: clang 15 cannot parse
  `__has_c_attribute(clang::always_inline)`
  (gotchas/lto-keeps-export-out-of-line.md); terminated by id.
- Not run: the full tier (the lead's, per batch). `XO_LLRT_C=alloc`
  stays until it passes on main.
