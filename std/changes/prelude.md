# prelude changes

## 2026-11-06: perf: a large block's growth reuses the mapping its last growth made

- Kind: perf
- Commit: this push
- Found by: the json10kx20 row of bench/results/release-vs-go-v0.40.0.md:612 (max RSS 3.25 times Go's on linux/amd64)
- Go reference: none (Go's heap has no such cache; runtime/mheap.go reuses spans by page count)
- Test: std/native/llrt/heapos_test.xo "a block grown past 1 MiB reuses the mapping its last growth made"
- Timing: ab-host micro13 json10kx20, 3 rounds: linux/amd64 max RSS 45,352 to 12,092 KB (0.267; Go 13.9 MB), wall 0.94; linux/arm64 within noise (wall 0.897, 0.882 to 1.015; max RSS 1.042, 1.0 to 1.042; 15,184 to 15,820 KB: the quarter bound already dropped the dead mappings there)
- Shipped in: not yet released

The large block cache served a request only from a mapping of at most four times its size, unrounded, while a growing block's mapping is four times its size rounded up to 64 KiB: a block that grew past 1 MiB mapped 4 MiB, and the same growth next time could not take that mapping back. A loop that grows such a block each round (json10kx20's encode and decode) mapped a new 4 MiB every round; on amd64 the old ones stayed resident in the cache (decision 0492) until it held 16 of them. The bound is now rounded the same way (gotchas/large-cache-fit-refuses-own-growth.md).








## 2026-11-06: perf: equal_fold, eq_lower and to_lower/to_upper's fast path step 8 bytes at a time

- Kind: perf
- Commit: this push
- Found by: the SIMD survey of std's byte paths (bench/results/simd-survey-2026-11-06.md): ASCII case insensitive compares were the hottest unvectorized search loops
- Go reference: strings.EqualFold, bytes.EqualFold (go1.26.1)
- Test: std/native/llrt/scan_test.xo "fold_eq_words and case_run match a byte loop at each byte"
- Timing: bench/perf/simdscan on linux/arm64, new std against main's (the same xo): fold (4 KiB) 0.185, fold_32 0.212 of main's wall; against Go 1.26.1 fold 0.165, fold_32 0.245. bytestransform upper 1.04 of main's (short lowercase texts: one word read before the byte that changes), lower 0.93
- Shipped in: not yet released

`Str.equal_fold`, `Bytes.equal_fold` and `eq_lower` compare 8 bytes a step while both words are ASCII, equal, or equal with their capitals lowered (std/native/llrt/scan.xo fold_eq_words), then go on byte by byte from the first word that is not; `to_lower` and `to_upper` find the first byte to change (or at or above 0x80) a word at a time (scan.xo case_run).

## 2026-11-06: perf: Str and Bytes equality, order and one byte search faster on linux; ASCII validation faster everywhere

- Kind: perf
- Commit: this push
- Found by: the SIMD survey of std's byte paths (bench/results/simd-survey-2026-11-06.md)
- Go reference: bytes.Equal, bytes.Compare, bytes.IndexByte, utf8.Valid
- Test: std/native/llrt/libcstr_test.xo "memcmp of 0 to 80 bytes differing at each byte", std/native/llrt/scan_test.xo "find_byte and span find a byte at each place of 0 to 100"
- Timing: bench/perf/simdscan on linux/arm64, new std against main's (the same xo): equal 0.73, compare 0.74, index_byte 0.74, utf8_ascii 0.60, index_byte_64 0.87 of main's wall; against Go 1.26.1 equal 1.65 (was 2.30), compare 1.34 (1.86), index_byte 1.91 (2.73), utf8_ascii 1.26 (2.63)
- Shipped in: not yet released

On linux the native build carries its own `memcmp` (std/native/llrt/libcstr.xo), which every Str and Bytes `==` over 16 bytes and every `<` reach: it compared 8 bytes a step and then byte by byte; it now compares 32 bytes a step and finds the first differing byte of a word by its lowest set bit, with the last partial word read as the word ending at the end. `find` of one byte and the ASCII run of UTF-8 validation (`Bytes.to_str`) test 32 bytes a step, one branch for two 16 byte masks, with wrapping index arithmetic in the loops.

## 2026-11-05: fix: a read on a FakeNet connection is woken by its deadline

- Kind: fix
- Commit: this push
- Found by: aws-19's azservicebus tests over std/amqptest (Receiver.receive(20) on an empty queue faulted XO-F001)
- Go reference: net.Conn.SetReadDeadline (the in memory pipe of net.Pipe honors it)
- Test: std/native/llrt/netconn_test.xo "a waiting read on a fake connection times out at its deadline", std/native/amqptest_test.xo "receive on an empty queue times out in the test world"
- Timing: none (the timer exists only while a read waits with a deadline)
- Shipped in: not yet released

A `read` that waits on a connection of FakeNet's in-memory network now fails with `Timeout` when its read deadline (decision 0430) passes: a timer wakes the wait, so the test world's virtual clock moves to the deadline instead of reporting `XO-F001: deadlock: every task is blocked and no timer is pending`. A waiting write is still not woken by a deadline.

## 2026-11-05: fix: a CPU profile that fills its sample ring says so, and the ring is 8 times larger

- Kind: fix
- Commit: this push
- Found by: Xo Lang 1's cold self build profile at 1000 Hz (32,768 of 280,244 samples kept; a pass read 35% instead of 27%)
- Go reference: runtime/pprof's profBuf, which grows its record and reports overflow as lost samples
- Test: compiler/xo prof_test.xo "top_text warns loudly when CPU samples were lost and suggests a rate"
- Timing: none (the larger ring is mapped only when profiling, its pages touched as samples arrive)
- Shipped in: not yet released

The runtime's CPU sample ring (std/native/llrt/prof.xo CAP) holds 262,144 samples, up from 32,768. A profile that still fills it prints `xo profile: PROFILE INCOMPLETE: N samples dropped` at exit even under XO_PROF_QUIET, and `xo prof` repeats it with the share lost and the XO_PROF_HZ that would keep every sample. `XO_PROF_RING=N` sets the ring's slots (1,024 to 4,194,304), for a test that must fill it; the exit line without XO_PROF_QUIET says PROFILE INCOMPLETE too.

## 2026-11-05: feature: XO_CRASH_FILE keeps a native program's fault and crash reports in a file

- Kind: feature
- Commit: this push
- Found by: decision 0520 section 4 (a soak or chaos failure on a native build kept only standard error)
- Go reference: debug.SetCrashOutput (Go 1.23), which copies the fatal error report to a file
- Test: internal/arm64 TestCrashReport (the SIGSEGV report in the file, equal to standard error, and a second report appended), TestCrashFileFault (an uncaught fault's line and traceback)
- Timing: none (one environment lookup at start; nothing runs until a report)
- Shipped in: not yet released

`XO_CRASH_FILE=PATH` on Linux and macOS appends the reports a native program writes on standard error when it ends badly, a hardware fault's (crash.xo, both phases) and an uncaught fault's (`fault:` and its traceback), to PATH as well, creating it with mode 0644. The file is opened at start (std/native/llrt/crashfile.xo, close on exec), since the hardware fault report runs in a signal handler that may not allocate; a path that does not open is skipped. Windows and wasm ignore the variable.

## 2026-11-05: feature: Display for List, Map and Set

- Kind: feature
- Commit: this push
- Found by: the comparison with Go's container/set proposal (golang/go#69230); `${xs}` was XO0404 and agents hand wrote loops
- Test: internal/llvm/testdata/golden/collection_display (native, LLVM, Go) and test/errorcheck/interp_collection
- Shipped in: not yet released

`"${xs}"` and `xs.display()` work on a `List`, `Map` or `Set` whose elements have Display; the text is the Debug text (`[1, 2]`, `{"a": 1}`, `#{"x"}`). Decision 0525.

Also `Set.is_subset(o)` and `Set.intersects(o)` (the latter stops at the first shared element and allocates nothing); `==` on Sets already compared contents, ignoring order. A uniquely held Set is updated in place by `add` and by `s = s.union(x)` (1,000,000 adds 34 ms, 1,000,000 one element unions 70 ms on Mac arm64, against 49 ms for Go's map[int]struct{}).

## 2026-11-05: feature: Int8 to UInt32 wrapping_from and Float32 to_bits and from_bits

- Kind: feature
- Commit: this push
- Found by: the std/amqp port (2026-11-05): `Int.wrapping_from`, `UInt64.wrapping_from` and `Float32.to_bits` were the natural names and did not exist (the owner's survey follow-up)
- Go reference: the conversions uint8(x), int8(x), ... of an int64, and math.Float32bits and Float32frombits
- Test: golden num_wrapping (every backend: 22 boundary values through all seven types, 18 binary32 patterns round tripped, 30 narrowed products of 0.1)
- Shipped in: not yet released

`T.wrapping_from(x: Int)` for Int8, Int16, Int32, UInt8, UInt16, UInt32 and UInt64 is Go's conversion: the low bits kept, sign extended for a signed T, no fault (`try` is the checked form, `UInt64.from_bits` the old name of the 64 bit one). The compiler lowers it to a mask, or a shift pair for a signed type, and a bare integer conversion. `Float32.to_bits() -> UInt32` and `Float32.from_bits(b: UInt32)` give and take the binary32 bits (std/native/prelude.xo, from the widened double, which is exact); a signaling NaN is quieted on the native backends, where the Go backend keeps it.

## 2026-11-05: perf: the native heap has four size classes a doubling from 1 KiB to 8 KiB

- Kind: perf
- Commit: this push
- Found by: decision 0506 (bench/results/size-class-waste-2026-11-04.md: allocations above 1 KiB were handed 1.195 times the bytes they asked for, a 16 KiB buffer a 32 KiB block)
- Go reference: runtime/sizeclasses.go (about four classes a doubling, at most 12.5% tail waste)
- Test: std/native/llrt/obj_test.xo "every size above 1 KiB gets the smallest class that holds it"; the arm64 and llvm goldens
- Timing: xo bench --ab self on the linux/arm64 host, 5 rounds, 4 of them loaded by other jobs: max RSS 0.986 of main (4,341,468 to 4,280,256 KB), wall 0.994, cpu 0.993; bench/perf/sizewaste: 2 and 4 KiB buffers 1.27 times their payload (2.01), sizes between classes 1.03 to 1.06 (1.38 to 1.41) (bench/results/size-classes-stage-b-2026-11-05.md)
- Shipped in: not yet released

Above 1 KiB the classes were powers of two (2 KiB to 256 KiB), so a block could be almost twice its request. Classes 65 to 76 are now blocks of 1280, 1536, 1792 and 2048 bytes, then four a doubling to 8 KiB (a block at most 25% over its request, against up to 100% before: a 2 or 4 KiB buffer takes 1.27 times its payload, not 2.01). 16 KiB to 256 KiB stay powers of two (page blocks, decision 0425), so requests from 8 KiB to 16 KiB still take a 16 KiB block. A heap record's per class words moved to its end (82 classes). XO_HEAPSTATS reports a class's payload size.

## 2026-11-05: fix: a --race build no longer reports two tasks' faults on one worker as a race

- Kind: fix
- Commit: this push
- Found by: the 2026-11-05 race stress (lazy_const_cycle_tasks under --race on linux, 23 runs in 24)
- Go reference: Go's race detector leaves the runtime out, as decision 0273 mode 2 does
- Test: internal/llvm TestRaceRuntimeInlinedUnchecked (lazy_const_cycle_tasks); 48 linux --race runs clean
- Shipped in: not yet released

The fault unwinder zeroed and copied its per-thread scratch with the C library's memset and memcpy, which ThreadSanitizer intercepts. Two tasks that faulted one after the other on the same worker looked like a race. Those writes are now ignored, as the runtime's malloc already was.

## 2026-11-05: fix: rt.heap_release of an address that is no block's prefix ends the program

- Kind: fix
- Commit: this push
- Found by: the 2026-11-05 race stress (hserve.xo released three rt.heap_large results without `- 8`)
- Go reference: none (the native runtime's own records)
- Test: TestGoldenRun http_trailers, http_h2_config, http_server_limits fail with "not a block's prefix" before hserve's fix; the full linux/arm64 golden set passes with it
- Shipped in: not yet released

`rt.heap_release` (libcmem.xo heap_release) now ends the program with status 70 and "xo: internal: rt.heap_release(ADDR): not a block's prefix" when given an address that is not 16 byte aligned. Before, such a block went onto a free list 8 bytes off and corrupted the heap silently. Windows `--race` blocks, whose prefix is 8 past a 16 byte boundary, are not checked.

## 2026-11-05: feature: interface Yaml, the marker of derive Yaml

- Kind: feature
- Commit: this commit
- Found by: decision 0512 (std/yaml)
- Test: golden yaml_derive, `TestDerivableTable`

The prelude has `interface Yaml {}`, the marker `derive Yaml` implements (as `Xml` is for `derive Xml`); `yaml.decode` and `yaml.encode_value` take a `T: Yaml`. Programs name it only through `derive Yaml`. See std/changes/yaml.md.

## 2026-11-05: perf: on macOS scopes of tiny tasks no longer run slower than in 0.39.0

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 2's 0.40.0 pre-cut, bench/perf/conc spawn on darwin/arm64 (wall 1.191, CPU 2.144 against 0.39.0); decision 0518
- Go reference: Go's wakep chain, which decision 0514 left out on macOS for tiny tasks
- Test: TestGoldenRun conc_, spawn, sched, task_, scope_, fanout, chan_ on darwin/arm64; TestSelfhostWasm conc_*
- Timing: darwin/arm64 conc spawn 0.881 of main (11 rounds), CPU 0.56; spawn100k 0.953 (noise, 0514's gain kept); darwin/amd64 spawn100k 1.04 and conc spawn 1.013 (noise)
- Shipped in: not yet released

The worker that finds a tiny task while it is the only worker awake wakes
one more again, as before decision 0514: with no spinner left, each scope's
spawns and joins woke sleeping workers instead, a costlier macOS wake each.

## 2026-11-05: perf: on macOS a burst of tiny tasks wakes no extra workers

- Kind: perf
- Commit: this commit
- Found by: micro13 spawn100k on darwin/amd64 against Go (xo-lang-4; decision 0514)
- Go reference: Go's wakep chain (the last spinner to find work wakes another), which this keeps on Linux and Windows
- Test: the 27 concurrency goldens (conc_, spawn, sched, task_, scope_, fanout, chan_) on darwin/arm64; TestSelfhostWasm
- Timing: spawn100k darwin/amd64 work 29 to 26 ms (0.897, Go 25 to 27), darwin/arm64 22 to 18 (0.818); conc suite within noise, mutex 0.959
- Shipped in: not yet released

On macOS a spinning worker that finds a task of a spawn site whose tasks
finish in microseconds (decision 0359) no longer wakes another spinner, so
a burst of such tasks stays with the workers already awake instead of
waking sleeping ones whose wake and sleep cost more than the tasks.

## 2026-11-05: perf: map, indexed, group_by and sort_by size their result once

- Kind: perf
- Commit: this commit
- Found by: the capacity hint audit after 1b4b2bfa1 (xo-lang-4)
- Go reference: make([]U, 0, len(xs))
- Test: TestGoldenRun bytes_indexed, list_filter_map, list_group_by, list_sort_guard, map_inplace_copies; stdtest corpus prelude (arm64, LLVM, Go); the wasm goldens of the same names
- Timing: 1M Ints mapped to pairs linux/arm64 0.85, darwin/arm64 0.80; indexed 0.862, 0.769 (bench/results/capacity-hints-2026-11-05.md)
- Shipped in: not yet released

List.map (when the result type differs), List.indexed, Bytes.indexed,
List.group_by and List.sort_by built their result by pushing onto an empty
list, so a large one grew through about 40 copies; from 16 elements it is
now allocated once at its size.

## 2026-11-05: perf: Buf.with_capacity reserves its size on native builds

- Kind: perf
- Commit: this push
- Found by: profiling micro13 sort1M on linux/amd64 against Go (xo-lang-4)
- Go reference: make([]T, 0, n)
- Test: test/codegen/lists reserved (xo_x_obj_rl_new_n on arm64, amd64 and LLVM); TestGoldenRun native_gaps
- Timing: micro13 sort1M (fills a Buf.with_capacity(1_000_000), then sorts it) work_ms linux/amd64 65 to 61 (0.938), darwin/amd64 75 to 73 (0.973); wall linux/arm64 0.986 (bench/results/buf-with-capacity-2026-11-05.md)
- Shipped in: not yet released

`Buf[T].with_capacity(n)` evaluated n and dropped it, so the buffer started
empty and grew by copying (about 40 copies for a million elements, 2.9 ms
of sort1M's 65 on linux/amd64). It now allocates room for n elements, as
`List.with_capacity` has since decision 0170.

## 2026-11-05: fix: a CPU profile sample in an outline atomic helper has its caller

- Kind: fix
- Commit: this push
- Found by: profiling micro13 spawn100k on linux/arm64 (xo-lang-4)
- Go reference: none (Go inlines its atomics)
- Test: internal/llvm TestProfHelperCaller (raise 15.8% against the helper's 84.2% without the fix); TestProf* and the fault goldens unchanged
- Timing: none
- Shipped in: not yet released

On linux/arm64 a read modify write atomic calls one of the outline helpers
(`__aarch64_cas8_acq_rel` and friends) that xo's linker writes; they set up
no frame and have no unwind entry, so a profile sample in one ended the
walk there and the profile showed the helper with no caller. A leaf with no
unwind entry now takes its caller from the link register, as a leaf outside
the program did.

## 2026-11-05: fix: XO_PROF=cpu no longer crashes a program whose sample walks off a fiber's stack

- Kind: fix
- Commit: this push
- Found by: profiling micro13 spawn100k on linux/arm64 (xo-lang-4)
- Go reference: runtime traceback bounded by the goroutine's stack (gentraceback's stack.hi)
- Test: internal/llvm TestProfSpawnStress (fails without the fix: SIGSEGV in run 3 and run 4 of 16); TestProf* and the fault goldens unchanged
- Timing: none (the walk reads less)
- Shipped in: not yet released (0.39.0 has the bug: 1 of 30 runs)

A CPU profile sample unwinds the interrupted thread's frames in the signal
handler. The walk bounded each frame only by 64 MiB above the stack pointer,
so on a fiber near its stack's top it could read past the end into the next
fiber's guard page, and the program died with SIGSEGV in `cabi.on_prof`
(8 of 30 runs of a spawn heavy program at 1,000 Hz on linux/arm64). The walk
now stops at the end of the running fiber's stack.

## 2026-11-05: perf: Atomic[Int].add is one fetch and add, not a compare and swap loop

- Kind: perf
- Commit: this push
- Found by: micro13 spawn100k against Go (Lead, after the 0.39 precut report)
- Go reference: sync/atomic Int64.Add (one LDADDAL on arm64)
- Test: internal/llvm/testdata/golden/atomic_ops (operand bounds, values on both sides of 2^62, an overflow caught by task.isolate stores nothing), arm64, LLVM and Go parity
- Timing: bench/results/atomic-add-ldadd-2026-11-05.md: spawn100k linux/arm64 ab-host --rounds=15 wall 0.911, cpu 0.754; darwin/arm64 0.783, cpu 0.477; darwin/amd64 unchanged
- Shipped in: not yet released

`add` loaded the value and retried a compare and swap until it held, so tasks
adding to one counter failed and retried their swaps against each other (84%
of spawn100k's CPU on linux/arm64). It now does one fetch and add when the
operand is under 2^20 and the value within 2^62 of zero, where no overflow is
possible; larger values and operands keep the checked loop, so a sum that
does not fit still faults and stores nothing.

## 2026-11-05: perf: List.sort_by of a list nothing else holds sorts it in place

- Kind: perf
- Commit: this push
- Found by: decision 0508 step 2
- Go reference: slices.SortStableFunc (in place, stable)
- Test: prelude_test.xo "list_sort_by in place: a held list keeps its order, equal keys keep theirs", arm64 and LLVM
- Timing: linux/arm64 offload host, ab-host.sh, bench/perf/listmap: sortby (100k records) --rounds=15 wall 1.037 (0.971 to 1.271, within noise), max RSS 0.863; sortby_small (8 records) --rounds=7 wall 0.871
- Shipped in: not yet released

`sort_by` moves its list into a `var` and runs the stable in-place sort on
it (one key per element, as before), instead of sorting an index list and
building a new list from it. A list another binding holds is copied by
the sort's first write, so the holder keeps its order.

## 2026-11-05: perf: List.reverse of a list nothing else holds reverses it in place

- Kind: perf
- Commit: this push
- Found by: decision 0508 step 4
- Go reference: slices.Reverse (in place, swapping from both ends)
- Test: prelude_test.xo "list_reverse in place: a held list keeps its order" (Ints, Strs, structs with lists, 0 and 1 elements), arm64 and LLVM
- Timing: linux/arm64 offload host, ab-host.sh --rounds=7, bench/perf/listmap reverse (1M Ints 20 times, 8 Strs 1M times): wall 0.457 (0.404 to 0.492), max RSS 0.609
- Shipped in: not yet released

`reverse` moves its list into a `var`, makes it unique with one counted
write (a copy when another binding holds the list, so the holder keeps its
order) and swaps from both ends, instead of pushing every element onto a
new list.

## 2026-11-05: perf: List.map to its own element type writes a list nothing else holds in place

- Kind: perf
- Commit: this push
- Found by: decision 0508 (the sort1M investigation: a builder can reuse a receiver its caller handed over at a last use)
- Go reference: none (Go has no map builder; a loop writing xs[i] = f(xs[i]))
- Test: prelude_test.xo "list_map in place: ..." (a held list keeps its elements; temporaries, 255, 256 and 300 elements, a new element type; counted elements and a closure that captures the list; an error part way), arm64 and LLVM
- Timing: linux/arm64 offload host, ab-host.sh --rounds=7, bench/perf/listmap: 1M Ints (`big`) wall 0.663 (0.613 to 0.831), max RSS 0.609; lists of 32 and 31 (`mid`) 0.992 (0.958 to 1.045), within noise; four Ints and two Strs (`small`) 0.843 (0.796 to 0.868)
- Shipped in: not yet released

`map` of a list of 256 or more elements whose result element type is the
list's own writes each result over its element when nothing else holds
the list (a temporary, or a binding not read again), instead of building
a new list. f is seen once as a `fn(T) -> T`, so no element is converted.
A list another binding holds is copied by the first write, so its holder
never sees a change. Shorter lists keep the new list: the type test costs
two small allocations (32 elements in place were 1.07 of main's time).

## 2026-11-04: perf: List.sort of a list nothing else holds sorts that list's own array

- Kind: perf
- Commit: this push
- Found by: micro13 sort1M_copy (`xs.freeze().sort()`): a function taking a list and sorting its own `var` copy made no second list, but `sort` did; list_sort handed its parameter to the Int path through rt.any_of while the merge path still used it, so the count was 2 and pdqsort's first write copied the 8 MB list
- Go reference: none (Go's slices.Sort has no copying form)
- Test: prelude_test.xo "list_sort: a list another binding holds keeps its order" and the other list_sort tests, arm64 and LLVM
- Timing: bench/results/sort1M-in-place-2026-11-04.md (darwin/arm64 --ab of sort1M_copy: max RSS 0.59, wall 0.995 (0.991 to 0.999); linux/amd64 wall within noise, max RSS 0.966; i7 laptop median 78 to 75 ms, Go 75)
- Shipped in: not yet released

`sort` takes its parameter's reference first and runs the Int sort through
`sort_in_place` on it, so when the call is the caller's last use of the
list (a temporary such as `xs.freeze().sort()`, or a binding not read
again) the sort writes that list's own array. A list another binding still
holds is copied first, as before, so its holder keeps its order. The
stable merge for other element types is unchanged (a List[Float] of 1M
touches the same pages before and after: its one extra list is the
merge's buffer).

## 2026-11-04: perf: List[Int].sort_in_place runs pdqsort in the receiver

- Kind: perf
- Commit: this push
- Found by: the sort1M x86 gap (Lead, 2026-11-04): split timing on the i7 laptop put the sort itself at Go's speed; the gap was micro13's `xs.freeze().sort()` building a second 8 MB list (1,529 page faults against Go's 1,003), and `sort_in_place`, the in-place form, ran the stable merge with a compare closure for Ints
- Go reference: slices.Sort (pdqsortOrdered), already ported as list_sort_int
- Test: prelude_test.xo "list_sort_in_place: Ints take pdqsort in the receiver, as sort does" and the other list_sort tests, arm64 and LLVM
- Timing: bench/results/sort1M-in-place-2026-11-04.md (linux/amd64 --ab, sort_in_place of 1M Ints: 0.681 (0.674 to 0.702) of the merge's wall time)
- Shipped in: not yet released

`sort_in_place` on a `List[Int]` sorts with list_sort_int's pdqsort in the
receiver's own array, with no scratch list and no closure call per compare.
Equal Ints cannot be told apart, so the result is the stable sort's, as
`sort` of a `List[Int]` already was.

## 2026-11-04: perf: a TCP connection's socket is made nonblocking by accept4 and socket on Linux

- Kind: perf
- Commit: this push
- Found by: the per connection system calls of bench/perf/tcpping against Go's twin (Lang 3's handshake row, decision 0502's investigation)
- Go reference: internal/poll's accept (accept4 with SOCK_NONBLOCK|SOCK_CLOEXEC) and net's sysSocket
- Test: the net, http and tls goldens on linux/arm64; xo bench --ab on tcpping, smtp and httpsession
- Timing: linux/arm64 --ab against main: std/smtp send 0.949 (0.942 to 0.958) of the wall time; tcpping and httpsession within noise
- Shipped in: not yet released

On Linux, an accepted or dialed TCP connection's socket is created
nonblocking and close on exec by accept4 or socket itself, as Go's are,
instead of three fcntl calls after it (get flags, set O_NONBLOCK, set
FD_CLOEXEC). macOS and Windows keep the calls, since their accept and
socket take no such flags.

## 2026-11-04: changed: Duration.seconds is Go's Duration.Seconds to the bit

- Kind: behavior
- Commit: this push
- Found by: the std/rate random differential against golang.org/x/time/rate (tokens off in the last bit); decision 0503, accepted by the owner 2026-11-04
- Go reference: time.Duration.Seconds (`float64(d/Second) + float64(d%Second)/1e9`)
- Test: golden duration_seconds_go (Go's bits for durations where one division differs, negatives, 0, 1 ns, 2^62 ns), the rate differential
- Shipped in: not yet released

`seconds()` was the nanoseconds over 1e9 as one division (the SSA lowering and the Go backend's `float64(d) / 1e9`); for some durations past one second that rounds differently from Go's whole seconds plus the rest over 1e9 in the last bit. Every backend now computes Go's formula; a port that calls `seconds()` where Go calls `Seconds()` agrees with Go to the bit.

## 2026-11-04: perf: Str.repeat and Bytes.repeat fill as Go's Repeat

- Kind: perf
- Commit: this push
- Found by: the mem_trim probe of the untested-features run (decision 0505): re-allocating 160 MB of blocks took 340 ms against Go's 8, all of it in `b"x".repeat(n)`
- Go reference: strings.Repeat and bytes.Repeat (one copy, then the filled prefix copied after itself, in chunks of at most 8 KiB rounded to whole copies)
- Test: golden repeat_fill (every backend); twin bench/perf/repeat
- Timing: linux/arm64 host 2, best of 5: one byte pattern to 160 MiB 354 ms to 4.0 (Go 3.0), three bytes to 150 MB 113 to 4.1 (Go 2.5), 100 bytes to 160 MB 3.8 to 4.2 (Go 3.3); `xo bench --ab` byte1 B/A 0.012 (0.012 to 0.014), byte100 within noise
- Shipped in: not yet released

The native `str_repeat` and `bytes_repeat` (llrt/raw.xo) made one `mem_copy` per repetition, so a short pattern cost a call per byte or three. They now share `fill_repeat`, Go's algorithm. The Go backend already called Go's.

## 2026-11-04: perf: a worker back from its poller wait runs the fibers it readied without waking another

- Kind: perf
- Commit: this push
- Found by: XO_RTSTATS of bench/perf/tcpping handshake (1.54x Go, Lang 3): about 11,000 chain wakes in 2,000 connections, 26,000 context switches against Go's few hundred
- Go reference: runtime.findrunnable (a thread back from a blocking netpoll runs the goroutine on its own P, not spinning; injectglist wakes others only for more)
- Test: the conc and net goldens (every native backend); xo bench --ab on tcpping, smtp, httpsession, micro13 spawn100k and the HTTP rows
- Timing: bench/results/worker-own-poll-2026-11-04.md (linux/arm64: tcpping handshake 0.93 of Go's wall, from 1.54; smtp send 0.53 of main, 0.71 of Go; connect 0.78; the HTTP rows unchanged)
- Shipped in: not yet released

On the native backends, a worker whose wait in the network poller
(decision 0324) readied fibers on its own ring used to count itself a
spinner and, on taking its own fiber, wake another worker that found
nothing: one futex wake and sleep per request and reply between two
fibers. When at most one other fiber waits on a descriptor it now runs them
without becoming a spinner (decision 0502); with more (a server's other
connections) one worker keeps waiting in the poller as before.

## 2026-11-04: perf: eprintln writes the line and its newline in one write

- Kind: perf
- Commit: this push
- Found by: XO_PROF of the log twin (87% of a text record's time in `write`): eprintln made two write system calls, the text and then "\n", where Go's log and slog make one
- Go reference: os.Stderr.Write of a whole line (log/slog's handler writes the record with its newline in one Write)
- Test: twincheck bench/perf/log; the stdio goldens (every backend)
- Timing: bench/results/std-twin-log-2026-11-04.md (host 2: text 0.640 of Go, json 1.094; Mac 0.62 of its old time)
- Shipped in: not yet released

Stdio.eprintln (and so every log record) copies a line shorter than 64 KiB
into one buffer with its newline and writes it once; the other lines
keep the two writes. Two writes also let another process's line land
between a record and its newline.

## 2026-11-04: fix: a --race CPU profile no longer stalls the program

- Kind: fix
- Commit: this commit
- Found by: TestGoldenTasks/debug_routes under ThreadSanitizer hanging in its /profile request (4 in 1600 runs on linux/arm64)
- Go reference: none (Go's race runtime is the Go flavor of ThreadSanitizer, whose signals go through the Go runtime)
- Test: debug_routes --race 3200 runs on linux/arm64 with no hang (census harness, gotchas/tsan-cpu-profile-stall.md)
- Speed: none

Under ThreadSanitizer the profiler's SIGPROF handler is installed with
the C library's sigaction, which the sanitizer intercepts, instead of
the rt_sigaction system call: a sample that lands inside the
sanitizer's runtime is deferred to a safe point instead of running on
the locks the thread holds. Other builds are unchanged.

## 2026-11-04: perf: one poller wake byte at a time

- Kind: perf
- Commit: this commit
- Found by: the owner's v0.38.1 evaluation (spawn100k 1.33x Go on darwin/amd64); XO_PROF: write 45% (poll.poke from pool.wakep's np_break)
- Go reference: runtime/netpoll_kqueue.go and netpoll_epoll.go netpollBreak (netpollWakeSig CAS; netpoll clears it after reading the pipe)
- Test: TestGoldenRun conc, spawn, scope, timer, signal, net and http goldens; the micro13 spawn100k on darwin/amd64, linux/amd64 and linux/arm64
- Speed: darwin/amd64 spawn100k 39 to 30 ms (Go 26)

np_break writes the self pipe's byte only when it sets the event loop's
break flag (EV_BRK) from 0 to 1; the poll that drains the pipe clears it.
A push while a byte waits writes nothing: that byte ends the poller's
next wait anyway. Windows posts a completion per break as before.

## 2026-11-04: perf: the darwin size class limit is 64 KiB

- Kind: perf
- Commit: this commit
- Found by: bisecting the owner's v0.36.0 Intel Mac evaluation (darwin/amd64 map1M 141 to 154 at d3b9a5458)
- Go reference: none (Go's size classes go to 32 KiB, its large spans stay in the page heap)
- Test: TestGoldenRun heap goldens; cmd/xo TestHeapMidMaxRange; the 13-perf micro on darwin/amd64; the xo self build on darwin/arm64 (decision 0448 addendum tables)
- Speed: darwin/amd64 map1M 145 to 138 ms, sort1M 90 to 83

heapos.xo MID_DARWIN 16384 to 65536: a freed block of 16 to 64 KiB stays
on its class free list instead of being unmapped, so a map's or list's
regrowth reuses it. The micro's max RSS 55.5 to 58.7 MB; the self
build's footprint the same, its max RSS 6 to 15% higher.

## 2026-11-04: fix: an out of range XO_HEAP_MID_MAX is reported at start

- Kind: fix
- Commit: this commit
- Found by: the darwin 0.35 to 0.36 bisect (XO_HEAP_MID_MAX=262144, one word past the cap, kept the 16 KiB default and looked like no effect)
- Go reference: none (GODEBUG ignores unknown settings, but Xo's heap knobs take numbers)
- Test: cmd/xo TestHeapMidMaxRange (1024, 65536, 262136 quiet; 262144, 512, 64K, off warn; every native backend)

entry.xo's start prints `xo heap: XO_HEAP_MID_MAX ignored: it takes a
byte count from 1024 to 262136` on stderr when the value is outside that
range or not a number, and keeps the default as before.

## 2026-11-04: perf: on darwin the large block cache keeps the pages of the sizes a program asks for again

- Kind: perf
- Commit: this commit
- Found by: bisecting the owner's v0.36.0 Intel Mac evaluation (darwin/amd64 json10kx20 0.35.0 to 0.36.0); cause 14a71b5e6
- Go reference: none (Go's scavenger returns pages in the background, not at free)
- Test: TestGoldenRun (heap and json goldens); the 13-perf micro on darwin/amd64 and darwin/arm64; the xo self build's footprint
- Speed: darwin/amd64 json10kx20 122 to 105 ms, map1M 152 to 145; darwin/arm64 map1M 55 to 50

keep gives a freed mapping's pages past its first 16 KiB back to Darwin
(MADV_FREE_REUSABLE, decision 0448 stage 1) only when none of the last
16 requested sizes (decision 0492's ring) could use it; a mapping that
one could is taken again soon, and the advice made every reuse fault
its pages back in. The xo self build's peak footprint 3550 to 3584 MB
down to 3431 to 3445 (its max RSS about 5% higher); the micro and push
max RSS unchanged.

## 2026-11-04: perf: on amd64 the large block cache keeps the sizes a program asks for again

- Kind: perf
- Commit: this commit
- Found by: the owner's v0.36.0 and v0.38.1 evaluations (Intel Mac json10kx20 111 to 176 ms); bisected to f3f9552ac
- Go reference: none (Go's heap keeps freed spans for reuse)
- Test: TestGoldenRun (heap goldens); the 13-perf micro and bench/perf/xo putget and push on linux/amd64, darwin/amd64 and darwin/arm64 (decision 0492's table)
- Speed: json10kx20 linux/amd64 84 to 47 ms, darwin/amd64 154 to 124 ms; arm64 unchanged

The cache table keeps the last 16 sizes asked of it (take, hit or miss),
and on amd64 a freed mapping one of them could use (take's fit: need <=
size <= 4 need) may pass decision 0448's quarter bound (never CACHE_N or
CACHE_MAX). A program that frees and asks again for one size keeps those
mappings; a growing list still gets the bound. Max RSS on amd64: the
micro +6 MB (linux) and +9 MB (darwin), putget +0.8 and +1.9 MB, push
40.0 to 29.6 MB (linux) and 29.5 to 30.0 (darwin). arm64 keeps 0448's
rule (no speed to gain there; its RSS would have risen the same).

## 2026-11-04: perf: a Map's rebuild makes its slot table at its size

- Kind: perf
- Commit: this commit
- Found by: the owner's v0.36.0 evaluation (map1M 1.13x of 0.35.0), bisected over std with the 0.36.0 xo to 188398ede (decision 0452's list growth curve)
- Go reference: runtime/map.go makes a grown map's buckets at their size (makeBucketArray)
- Test: arm64 TestGoldenRun map goldens, test/codegen; bench/perf/micro13 map1M
- Timing: linux/amd64, micro13 map1M, xo bench --ab 20 rounds, main against the change: wall 0.905 (0.88 to 0.953), max RSS 0.902; the program's own millis (21 runs) 0.35.0 73/71, main 85/82, the change 76/72 (median/min); darwin unchanged (about 50 ms, 48 MB)

xo.map.rebuildk (and the lists path's xo.map.rebuild) filled the new
hash table by pushing its slots into a list made with no room. With
decision 0452's 1.25x growth past 256 elements, a table of 2M slots
grew about 40 times while it was filled, once per rebuild, where
doubling grew it about 21 times. The table is now made at its size,
and the lists path's entry lists get room for every entry until the
next rebuild (3/4 of the table, not 2 * want + 6).

## 2026-11-04: perf: wasm modules leave out Linux's vDSO clock lookup

- Kind: perf (size)
- Commit: this commit
- Found by: the wasm component size breakdown (decision 0447 queue, component size)
- Go reference: none
- Test: TestWasmComponentServe, TestWasmCommandGoldens (wasip2, wasip3)
- Size: hello component 324,153 to 319,394 bytes (-4,759); the 0447 step 2 program 531,365 to 526,612

sysio's clock_now and real_ns answer WASI's clock first under a constant
`target.arch == TargetArch.Wasm32` test, so clock_read, clock_choose and
linux.vdso_clock (elf_dynsym, auxval) are no longer compiled into a wasm
module; the clock read is unchanged on every target.

## 2026-11-04: fix: XO_PROF's stack walk stops at a negative pc

- Kind: fix
- Commit: this commit
- Found by: Xo Lang 1 (a CPU profile of xo with the drop thunk, whose first draft had no frame record, faulted at exit: `-9223372036854775808 - 1 does not fit Int` in prof.table_of)
- Go reference: none (runtime/pprof's unwinder stops at an invalid pc too)
- Test: XO_PROF=cpu on xo's self build (32768 samples, written)

The profiler's walk recorded any word a misdescribed frame's step read
as a pc; a negative one (the immortal reference count) then overflowed
`pc - 1` while the profile was written, and the program exited 70 after
its own work. A pc of 0 or below now ends the stack, as 0 did.

## 2026-11-04: perf: List.sort, insert and remove_at move elements by swaps

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 3 (netip compare 2.02 of Go, 13% flat in `drop Str`; list_sort's `a[j] = a[j - 1]` a dup and a drop of a Str field per move)
- Go reference: slices.SortStableFunc and slices.Insert/Delete (go1.26.1) move elements as words
- Test: TestStdCorpora (the prelude corpus's sort, insert and remove_at rows), every arm64 golden under XO_FREE_CHECK=1, native_list_swap and native_list_swap2
- Timing: bench/perf/netip compare 0.509 (0.504 to 0.515) on linux/arm64 together with the next commit's derived order (the two measured as one stack); xo's self build 1.001 (0.996 to 1.006, within noise)

`List.sort`'s runs of 16 and its merges between two buffers, and the
insertion runs of `sort_in_place` and `sort_in_place_by`, move elements with
`rt.list_swap` and `rt.list_swap2` (decision 0374): a merge's swap leaves the
destination's old element in the source slot the pass has consumed. The
two buffers are made unique with one counted write each, as before (the
first move of each copied). `List.insert` pushes then swaps the new element
down, and `List.remove_at` swaps the element to the end and pops it, so a
shift counts nothing where a counted move retained each element and
released the one it replaced.

## 2026-11-04: perf: ByteBuffer.read_until keeps its segment borrowed

- Kind: perf
- Commit: this commit
- Found by: the Lead's Option-of-a-view item (bench/perf/bytebuf buf against views); decision 0487
- Go reference: none (bufio.Reader.ReadSlice returns a view without counting)
- Test: goldens borrow_setfield_chain and borrow_setfield_join; every arm64 golden under XO_FREE_CHECK=1
- Timing: bench/perf/bytebuf small_buf 0.940 (0.934 to 0.976), linux/arm64 host 1, n=3

read_until's fast path writes scan_to on both paths instead of in an
`if`, whose join passed the ByteBuffer through a block parameter and
kept the segment from b.segs counted; with decision 0487 the segment is
borrowed: one retain a record (the returned view), from two retains and
two releases.


## 2026-11-04: perf: a write to a container held in a Mutex stays in place, with no walk of the container at unlock

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 2's cookiejar and httpsession measurements (decision 0488)
- Go reference: none (Go has no counts; its sync.Mutex guards a value written in place)
- Test: internal/llvm TestTSanSharedMaps (tsan_maps writes lists in place under the lock and reads their elements on other threads; with the new pass off it fails), TestGoldenTasks under ThreadSanitizer, XO_FREE_CHECK goldens
- Timing: bench/results/shared-container-in-place-2026-11-04.md (host 1, n=5): cookiejar 0.622 to 0.643 of its wall, httpsession store 0.919, store100k 0.876 (max RSS 0.541), plain within noise; a Mutex[List[Str]] of 100,000 elements, 20,000 writes, 2.55 to 3.28 s to 0.01 s (Mac); the self build within noise

A List, Map or struct shared with one holder (count -1, as `Mutex.with` hands it back) is written in place without being claimed back to thread local, and the element written is shared as it is stored (decision 0488). Before, the unlock that followed shared the whole container again, an O(size) walk per `with`. A Map in a Mutex now takes the same in-place write instead of going to its log form on the second shared write.

## 2026-11-04: feature: File.read_at on wasm

- Kind: feature
- Commit: this commit
- Go reference: os.File.ReadAt (go1.26.1), as natively
- Test: the zip_open_file golden (a zip written to a real file, then read_at and archive.zip_open on another File) on wasip1 (Node and wasmtime), wasip2 commands, arm64 and LLVM
- Timing: none (one host call)

`f.read_at(offset, max)` in a wasm build is preview 1's fd_pread on wasip1/wasm and wasi:filesystem's `descriptor.read` at the offset in a wasip2/wasm command, so `archive.zip_open` reads a zip in place there too. The descriptor's position does not move. A wasi:http component has no files (XO1515).

## 2026-11-03: behavior: a fault inside Mutex.with poisons the mutex; Mutex.reset

- Kind: behavior
- Commit: this commit
- Found by: decision 0482 (owner, 2026-11-03): std/cookiejar's jar under Mutex paid a copy of its maps per write to keep the value for a fault
- Go reference: none in Go (a panic under a deferred Unlock leaves the writes); Rust's Mutex poisoning
- Test: golden mutex_poison on arm64, LLVM, Go and wasm; fault_tasks, conc_mutex_move, byref_fault_merge updated
- Timing: bench/results/mutex-poison-2026-11-03.md

A fault that unwinds out of a `with` closure no longer restores the
value: the mutex is poisoned, and later `with` and `with_read` fault
with XO-F010 until `m.reset(v)`. An Err and a cancellation still leave
the value unchanged. A closure that cannot be canceled no longer keeps
the value it was given, so its writes do not copy it.
## 2026-11-03: fix: the native heap record list's head is read only with acquire

- Kind: fix
- Commit: this commit
- Found by: the third ThreadSanitizer race on the head in two days (5a5a5792b; gotchas/heap-list-head-is-newest-record.md)
- Go reference: none (Go's runtime mheap is not a list of per thread records)
- Test: internal/repotest TestHeapListHeadAccessors; goldens with threads and the --race runtime tests
- Timing: none (setup, diagnostics and the huge page mode read it, no allocation path)

Every read of std/native/llrt/heapos.xo's heap record list head
(W_HEAPS) goes through one accessor, `heaps_head`, an acquire load, and
every write through `heaps_set`, a release store. A pool thread pushes its
record there while other threads read it; three plain loads added since
2026-11-02 were each a race. A repository test now fails on any other use
of the word.

## 2026-11-03: feature: File.read_at

- Kind: feature
- Commit: this commit
- Go reference: os.File.ReadAt (go1.26.1), one call (a short read is allowed)
- Test: the prelude corpus `File.read_at reads at an offset without moving the position, as Go's ReadAt` (MemFs; arm64, LLVM, Go backend); a real file on macOS in a scratch program
- Timing: none (one syscall)

`f.read_at(offset, max)` is pread(2) (pread64 on Linux, ReadFile with an OVERLAPPED offset on Windows): the position and the read buffer are untouched, `None` at or past the end. A file open for writing, a closed file and a negative offset are errors. A wasm build answers "function not implemented".

## 2026-11-03: feature: XO_FREE_CHECK=1 poisons freed blocks and closed connections, so a use after a free faults on native

- Kind: feature
- Commit: this commit
- Found by: the Lead (2026-11-03): http_reverse_proxy_ws's use after close trapped only on wasm (gotchas/native-silent-on-freed-llrt-records.md)
- Go reference: none (Go's GC has no frees); GODEBUG's clobberfree is the nearest
- Test: at ab852cf42 plus this change http_reverse_proxy_ws crashes at 0xdeadbeefdeadbeef in hclient.send_chunk < http.tunnel under the mode and passes without it; the 83 network goldens and internal/stdtest pass under it (decision 0484)
- Timing: none (off by default; the paths it hooks are free_mt, close_at and the start)

Decision 0484. With XO_FREE_CHECK=1 the program runs on per thread heaps
from its start; every free goes through alloc.free_mt, which fills a
size class block with POISON and holds it in its thread's ring of
65,536 before it is reused, checking it was not written meanwhile;
hclient.xo close_at retires a connection's fields as POISON instead of
zero (heapos.retire).

## 2026-11-03: feature: DirEntry, read_dir_entries, walk_entries, walk_dir_entries

- Kind: feature
- Commit: this commit
- Found by: bench/perf/walkdir walk 1.54 of Go's wall on host 1 (bench/results/std-twin-rows4-2026-11-03.md): Go's WalkDir gives name and type from the directory entry and stats only on `Info()`
- Go reference: io/fs DirEntry, os.ReadDir, path/filepath.WalkDir (go1.26.1)
- Test: the prelude corpus ("read_dir_entries, walk_entries and walk_dir_entries ..."), twincheck bench/perf/walkdir
- Decision: 0483 (Accepted, owner, 2026-11-03); free functions, not Fs methods, so a user Fs still satisfies Fs
- Timing: Mac iteration (not evidence): walk 2304 ms (FileInfo), walk_entries 1597 (Go 1514); host 1 (bench/results/std-twin-walkdir-2026-11-03.md): walk_entries 0.91 of Go (walk 1.49, walk_info 0.94, walk_entries_info 1.11)

`DirEntry{name, path, is_dir, is_symlink}` and `DirEntry.info(fs)` (the lstat on demand), `read_dir_entries`, `walk_entries` and `walk_dir_entries` list a directory with the type from getdents64 (d_type), getdirentries64 or FindNextFile and no stat; an entry whose type the file system does not give (DT_UNKNOWN, a Windows reparse point) is stat'ed, as in Go. A user Fs and the test world answer through `read_dir`. Nothing existing changes.

## 2026-11-03: perf: trim_chars with a non-ASCII cutset decodes with raw.decode and a set walk

- Kind: perf
- Commit: this commit
- Found by: the host row of bench/perf/trimchars after decision 0465 step 2 (bench/results/std-twin-trimchars-2026-11-03.md): trim 1.54 of Go
- Go reference: strings.Trim's trimLeftUnicode and trimRightUnicode
- Test: the strings, bytes, strsearch, strwhere and trimchars corpora on arm64, LLVM and the Go backend
- Timing: Mac iteration (not evidence): trimchars trim 46 ms (Go 33); host row follows in std-twin-trimchars-2026-11-03.md

`trim_start_unicode` and `trim_end_unicode` (llrt scan) looked each rune up with `index_rune(chars, r)`, which took `chars.bytes()` and checked bounds per rune; they now walk the cutset's bytes with `rune_in_set` and decode the string with `raw.decode`, and return `rt.str_view`.

## 2026-11-03: perf: Str.trim and the trim_chars family return a view of their receiver on every path

- Kind: perf
- Commit: this commit
- Found by: decision 0465 step 2's ceiling (bench/results/retview-ceiling-2026-11-03.md): the trims did not qualify as functions that return a view of their parameter
- Go reference: strings.TrimSpace and strings.Trim return a slice of s on every path
- Test: arm64 string goldens under XO_RC_CHECK; internal/llvm TestGolden trim goldens; std test of native/path
- Timing: xo bench --ab on linux/arm64 (with decision 0465 step 2): bench/perf/trimchars trim 0.741 (0.663 to 0.76), trim_end 0.728 (0.697 to 0.744)
- Shipped in: not yet released

llrt strings.trim (Str.trim, trim_start, trim_end) cuts with rt.str_view
instead of raw.str_cut and no longer returns s itself when nothing is
trimmed: a Str is valid UTF-8 and both scans step over whole runes, so
str_cut's copy path never ran. scan.trim_chars, trim_start_chars and
trim_end_chars return rt.str_view(s, 0, s.len()) for an empty s or
cutset. Every return is then a view of the receiver, so decision 0465
step 2 marks them and their callers count the result only where they must
own it.
## 2026-11-03: feature: ByteBuffer, Go's bytes.Buffer for parsers

- Kind: feature
- Commit: this commit
- Found by: the backend survey of 2026-10-30 (std-coverage.md: no cursor to consume Bytes from the front), decided 2026-11-03 as decision 0480 (B1)
- Go reference: bytes.Buffer (Write, Len, Next, ReadBytes, Bytes); buffer_test.go ported
- Test: internal/stdtest corpus bytebuffer (Go's TestNewBuffer, TestBasicOperations, TestLargeStringWrites, TestLargeByteWrites, TestMixedReadsAndWrites, TestNext, TestReadBytes, TestReadEmptyAtEOF, TestBufferGrowth, and views after writes) on every backend
- Timing: linux/arm64, 1M 50 byte records 1.007 of Go's bytes.Buffer wall, 1.049 of B3's hand written views (the same work); 64 MiB record 0.208; peak RSS 0.18 to 0.25 of Go's (bench/results/std-twin-bytebuf-2026-11-03.md)
- Shipped in: not yet released

`ByteBuffer.new()`, `write`, `len`, `peek`, `next`, `read_until`,
`discard` and `find`: a record cut from bytes that arrive in pieces costs
time linear in its size, where `rest + piece` copies the unread rest each
time. Returned records are views that stay valid after later writes.

## 2026-11-03: feature: NetConn, a connection interface with deadlines

- Kind: feature
- Commit: this commit
- Found by: decision 0430's owner option (Stream deadline setters), decided 2026-11-03 as decision 0481
- Go reference: net.Conn (SetDeadline, SetReadDeadline, SetWriteDeadline)
- Test: golden netconn_deadline (plain and TLS through one NetConn: read timeout, request, write past its deadline) on arm64, LLVM and Go
- Timing: none (an interface; calls dispatch as Stream's do)
- Shipped in: not yet released

`NetConn` has Stream's methods and the three deadline setters; `Conn`,
`ServerConn` and `tls.TlsConn` satisfy it. Stream is unchanged.

## 2026-11-03: fix: Stdio.width and read_secret in a wasm module

- Kind: fix
- Commit: this commit
- Found by: decision 0447 stage 4's look at wasm_cmd_unsupported.txt (term_stdio)
- Go reference: none (golang.org/x/term's GetSize and ReadPassword fail on a descriptor that is not a terminal)
- Test: golden term_stdio as a wasip2/wasm command and under wasip1 (internal/selfhost TestWasmCommandGoldens, TestSelfhostWasm), off both unsupported lists
- Timing: none (no hot path)
- Shipped in: not yet released

In a wasm module `Stdio.width()` faulted (`rt.call_c2` in obj.heap_word)
and `read_secret` with it: the terminal width and echo code took the
native heap's scratch record before checking for a WASI module. A WASI
module has no terminal, so width is None and read_secret reads the line
without touching echo, as `stdin_is_tty` already answered false.

## 2026-11-03: feature: Duration.round and truncate, Bytes.to_str_lossy and to_valid_utf8

- Kind: feature
- Commit: this commit
- Found by: the backend survey of 2026-10-30, rank 13 (std-coverage.md)
- Go reference: time.Duration.Round and Truncate; bytes.ToValidUTF8 and string([]rune(s))
- Test: internal/stdtest corpora time (time_test.go's durationRoundTests and durationTruncateTests plus 300 seeded pairs) and bytes (bytes_test.go's toValidUTF8Tests plus the seeded invalid UTF-8 cases), native and Go backends
- Timing: bench/perf/bytestransform on linux/arm64: to_valid_utf8 0.68 of Go's ToValidUTF8 wall, to_str_lossy 0.97 of utf8.Valid plus string([]rune) (bench/results/std-twin-rows15-2026-11-03.md)
- Shipped in: not yet released

`d.round(m)` and `d.truncate(m)` are Go's Duration.Round and Truncate:
the nearest multiple of m (halves away from zero, the largest or smallest
Duration on overflow) and the multiple toward zero; d itself for m <= 0.
`b.to_str_lossy()` turns Bytes into text the way std reads outside text
(each byte of an invalid UTF-8 sequence U+FFFD, decision 0259), so
gunzipped or socket bytes get what `Fs.read_text` gives; `to_valid_utf8`
is Go's ToValidUTF8, one replacement for each run of invalid bytes.

## 2026-11-03: perf: XO_THP=dense keeps the main thread's stack and TLS off huge pages

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 2's THP threshold sweep (the dense-vs-off max RSS gap on linux/arm64)
- Go reference: Go's main goroutine stack is a heap span like any other, not a 1 GiB mapping
- Test: ab-host bench/perf/xo push, self build and a deep recursion program on linux/arm64; arm64 heap goldens
- Timing: max RSS about 2 MB lower in every program under THP=always (putget 8,696 to 6,612 KB, csv reader 5,908 to 4,068); push wall 1.002, self 0.992, deep recursion 0.995 (decision 0477)
- Shipped in: not yet released

On Linux the main thread's 1 GiB stack and its TLS block are now mapped
MADV_NOHUGEPAGE under the default XO_THP=dense, as thread and fiber stacks
already were; under THP=always the first touched 2 MiB of the stack had
been a huge page. XO_THP=kernel keeps the kernel's policy for them.

## 2026-11-03: perf: XO_THP=dense opens the main thread's first heap chunk by halves too

- Kind: perf
- Commit: this commit
- Found by: Lang 4's 0.36.0 release comparison (coll putget max RSS on linux/arm64), Xo Lang 2's THP breakdown
- Go reference: Go's heap advises huge pages by density for every heap region alike
- Test: arm64 heap, alloc and mem goldens; bench/perf/xo putget per XO_THP mode on linux/arm64; ab-host coll edit and self build
- Timing: putget max RSS 10,704 to 8,696 KB (dense, linux/arm64 THP always; 0.84 of v0.35.0); coll edit wall 0.989, self 1.002, both within noise (decision 0469)
- Shipped in: not yet released

On Linux the main thread's heap no longer advises its whole first 4 MiB
chunk for huge pages from the start; like every other thread's, it opens
the first 2 MiB on small pages and advises the rest once that is used up.
A small program no longer faults a huge page it barely touches.

## 2026-11-03: perf: a netpoller wait in a program with no concurrency wakes at once

- Kind: perf
- Commit: this commit
- Found by: Lang 1, moving std/dns lookups onto the netpoller (31ba6938b, reverted in db6eefbfc): about 0.8 ms more per lookup against a server in another process
- Go reference: none (Go's runtime always has its scheduler)
- Test: internal/arm64 TestGoldenRun net, dns, http, sleep, signal and concurrency goldens on linux/arm64
- Timing: 5000 lookups of a no-concurrency program against a responder in another process, on linux/arm64 with the dns change applied: 4.1 s to 0.19 s
- Shipped in: unreleased (the ticks since decision 0324)

A program without concurrency in its code as the compiler counts it
(concflag.xo) runs main on its own thread, with no workers, but its
poller was set up for decision 0324, where idle workers poll the network
and the poller thread only ticks (every 0.5 ms and up). Nothing else
polled, so each wake of a netpoller wait waited for the next tick. Such a
program's poller thread now waits in epoll or kqueue itself
(gotchas/netpoll-wait-idle-wake-latency.md).

## 2026-11-03: fix: long standard input on wasm

- Kind: fix
- Commit: this commit
- Found by: decision 0447 R2a sizing, reading compiler/wasm's heap_large
- Go reference: none
- Test: golden stdin_long_line on wasip1/wasm, wasip2/wasm and natively
- Timing: none
- Shipped in: none (standard input on wasm came in b16428830, after v0.36.0; before it a read faulted)

Reading standard input on wasm corrupted what followed once the input
buffer grew past 4 KiB: a long line, or many lines read ahead. The
buffer's growth (rt.heap_large_resize) copied from 8 bytes before the
old block (gotchas/wasm-heap-large-resize-prefix.md). Each read's 64 KiB
block is also given back now instead of leaked.
## 2026-11-03: perf: the large block cache holds at most a quarter of the bytes in use

- Kind: perf
- Commit: this commit
- Found by: Lang 4's 0.36.0 release comparison (coll putget max RSS 1.18x of v0.35.0 on darwin, 1.32 to 1.39x on linux/arm64)
- Go reference: none (Go's heap scavenger returns free spans in the background; Xo's cache keeps freed mappings for reuse)
- Test: golden mem_trim_large and the heap and alloc goldens; bench/perf/xo putget and push under xo bench --ab (darwin) and ab-host (linux/arm64)
- Timing: max RSS putget 0.761 (darwin) and 0.847 (linux/arm64), push 0.644 and 0.762, push wall 0.664 on linux/arm64; xo's builds within noise (decision 0448)
- Shipped in: not yet released

A freed large block's mapping is kept for reuse only while the kept
mappings stay under a quarter of the bytes the program has in use, as
well as the cache's 16 mappings and 64 MB. A kept mapping's touched
pages stay resident, so a small program that grows and drops large
tables no longer carries several megabytes of them to exit.

## 2026-11-03: perf: a growing large list or string reserves room for four times its size

- Kind: perf
- Commit: this commit
- Found by: Lang 4's 0.36.0 release comparison (darwin coll push 1.152x of v0.35.0, bisected to decision 0452's growth curve, 188398ede)
- Go reference: none (Go's growslice copies at every growth past the block's size class; Xo grows a large block in place inside its mapping)
- Test: golden mem_budget_growth (a 24 MB list under XO_MAX_MEM=64M: the reservation is not counted); bench/perf/xo push, putget and update under xo bench --ab on the Mac; linux/arm64 ab-host
- Timing: darwin, main's coll against the change, 5 rounds: push wall 0.863, instructions 0.863, max RSS 0.741 (56,080 to 41,568 KB); against v0.35.0's doubling curve: push wall 0.901, max RSS 0.815. putget and update max RSS 0.952. linux/arm64 ab-host (3 rounds): push wall 1.010 (within noise), max RSS 0.763; putget wall 1.011 (within noise), max RSS 0.880; the self build wall 1.006 (a 3-round "worse" under 1%, 2 of 3 rounds loaded), max RSS 1.001.
- Shipped in: not yet released (the regression shipped in v0.36.0)

A large block that grows (a list past its size class, a string being
built) is moved into a mapping with room for four times its new size
instead of twice: the pages past what is written stay reserved, not
used, and its next growths stay in place. With decision 0452's curve (a
quarter more past 256 elements) twice the size copied the list every
2.4x of growth, each copy faulting in fresh pages, which cost darwin
the most; four times copies it every 4.9x, fewer copies than the old
doubling curve made.
## 2026-11-03: feature: TCP in a wasip2/wasm command

- Kind: feature
- Commit: this commit
- Found by: decision 0447 (wasi:sockets, step R1)
- Go reference: none (Go's wasip1 port has no sockets)
- Test: goldens net_echo, conn_deadline, conn_deadline_move, conn_iface, net_dial_dual_stack, net_lookup_dial_timeout, net_short_str on wasip2/wasm (TestWasmCommandGoldens under wasmtime with the network flags its imports ask for)
- Timing: none
- Shipped in: none yet

`Net.listen`, `Net.dial`, `Net.dial_timeout`, `Net.lookup_host`,
`Net.only`, `Listener` and `Conn` (with `set_deadline` and its read and
write forms) work in a wasip2/wasm command over `wasi:sockets` 0.2, with
the native targets' NetErr values and texts
(std/native/llrt/netwasi.xo; compiler/wasm rt9.xo). Unix domain sockets
and service-name ports are not available there (`Io`).

## 2026-11-03: perf: fs paths already clean are returned unchanged (walk_dir, Fs.join)

- Kind: perf
- Commit: this commit
- Found by: bench/perf/walkdir walk 1.92 of Go's wall; profile: statx 36%, linux.names 20%, fs_clean_slash 20%
- Go reference: path.Clean (go1.26.1) lazy buffer fast path
- Test: bench/perf/walkdir (twincheck), `xo test --std-module=native/prelude`, internal/stdtest corpora, fixed point

`fs_clean_slash` checks first (`fs_is_clean`) whether the path has no empty, "." or ".." segment and no trailing
slash, and returns it without splitting and joining. Xo / Go wall of walk: 1.92 to 1.54. The remaining gap is
the API: Xo's FileInfo holds size, mode and time, so read_dir stats every entry, where Go's DirEntry stats only
on Info(); with Go calling Info() on each entry too (`walk_info`), Xo / Go is 0.95.

## 2026-11-02: fix: standard input on wasm

- Kind: fix
- Commit: this commit
- Found by: decision 0447 stage 3 (wasip2 commands), reading stdin
- Go reference: none (Go's wasip1 port reads fd 0 with fd_read)
- Test: golden stdin_read on wasip1/wasm (Node), on wasip2/wasm (TestWasmCommandGoldens under wasmtime) and natively
- Timing: none
- Shipped in: every release with wasip1/wasm, through v0.35.0

Reading `os.stdio` on wasm faulted (`rt.sys6`, a Linux system call). It
now reads descriptor 0 with `fd_read` on wasip1/wasm and `wasi:cli/stdin`
in a wasip2/wasm command; a `wasi:http` component's standard input is
empty. A second read on wasm was also canceled: `stop.requested()` read
through a state record that only a native start makes
(gotchas/llrt-record-unset-on-wasm.md); with no record it now reports no
stop.

## 2026-11-02: fix: `--race` no longer reports the runtime's own task, fiber and worker records as races

- Kind: fix
- Commit: this commit
- Found by: Lang 1's concurrency stress under `--race` on linux/arm64 (conc_scopes, conc_mutex, async_preempt_regs, conc_stress: sched.task_claim, fiber_yield, pool.preempt, rq_put)
- Go reference: none (Go's race detector resets a block's shadow at allocation, as the race heap's malloc now does)
- Test: the 23 conc_*, async_* and task_* goldens built with `--race` report nothing on a linux/arm64 host; TestRaceSyncTable and TestTSanSameThreadRace still pass (their unsynchronized cases report); TestPreemptMigrate/llvm-race no longer skipped on linux
- Timing: none (release and acquire calls are no-ops without ThreadSanitizer)
- Shipped in: not yet released

A task queued by `rq_put` is released and acquired where it is claimed;
the race heap's malloc and a new worker's record memset run with writes
ignored, so ThreadSanitizer resets their shadow instead of recording the
allocating thread's write (gotchas/race-runtime-records-false-reports.md).

## 2026-11-02: fix: a dial to localhost on Windows waited 2 s for ::1

- Kind: fix
- Commit: this commit
- Found by: Lang 1's Windows concurrency stress (golden tls_coalesced_finish timed out in 128 of 128 runs; alone it took 84 s, 2 s a connection)
- Go reference: net/dial.go dialParallel (RFC 6555, a 300 ms fallback delay)
- Test: golden net_dial_dual_stack (five dials to localhost with a listener on 127.0.0.1 under 5 s: 10 s on Windows before, `fast false`)
- Timing: tls_coalesced_finish on Windows 84 s to under 1 s alone; no change for a name of one family
- Shipped in: v0.7.0 (the C runtime's dial tried addresses in turn) through v0.35.0

`net.dial` tried a name's addresses one at a time. Windows resolves
localhost to `::1` and then `127.0.0.1`, and a refused connect there
takes about 2 s, so a dial to localhost with a server listening on
127.0.0.1 only spent 2 s on `::1` first. A name with IPv6 and IPv4
addresses is now raced as Go does (decision 0462): the other family
starts 300 ms after the first attempt, the first connection wins.

## 2026-11-02: fix: fault and out of memory messages were written from freed memory

- Kind: fix
- Commit: this commit
- Found by: Lang 1's XO0915 lint draft (xo vet --cost, a raw address used after its value was released), after the 2026-11-02 race stress found the class in std
- Go reference: none
- Test: cmd/xo TestVetCostStd with XO0915 (the lint's own push; it reports these lines without the fix)
- Timing: none
- Shipped in: every release with these messages, through v0.35.0

A task's fault trace, a crash report's trace, the fiber stack mmap failure
and the runtime's internal error messages were formatted into a string
and written with `stdio.write_all(2, raw.data(s), s.len())`, whose own
arguments were the string's last use, so it was released before the write
(gotchas/raw-address-outlives-bytes.md). The message came out right while
nothing reused the block; the strings now stay alive past the write.

## 2026-11-02: fix: a connection deadline already passed fails the next read or write at once

- Kind: fix
- Commit: this commit
- Found by: Lang 1's concurrency stress (golden tls_deadline, 6 of 256 runs beside CPU burners on linux/arm64: "3 write ok")
- Go reference: net.Conn's SetDeadline (a deadline in the past fails pending and later calls at once)
- Test: native/llrt/netconn_test.xo "a deadline in the past fails at once on a clock that has not moved" (fails before the fix on the test world's standing clock)
- Timing: none (one compare more on a read or write)
- Shipped in: none (29db8153f, decision 0430 stage A, came after v0.35.0; corrected 2026-11-02)

`c.set_deadline(clock, t)` with t not after now stored the deadline as
the monotonic reading with its low bit set, one ns ahead of an even
reading, so a read or write in the same clock tick ran instead of failing
with `Timeout`. On linux/arm64 the clock ticks every few ns and two
readings often match; on a test world's clock, which stands still between
sleeps, it failed every other time. A passed deadline is now a word that
is always passed.

## 2026-11-02: fix: a dial reported a connection that had not been made

- Kind: fix
- Commit: this commit
- Found by: Lang 1's concurrency stress (golden net_lookup_dial_timeout, "blackhole: connected?" in 3 of 256 runs on linux/arm64 and 1 of 256 on linux/amd64, beside CPU burners)
- Go reference: net/fd_unix.go connect (after a write wake, SO_ERROR 0 is a connection only when getpeername succeeds; ENOTCONN waits again)
- Test: native/llrt/netconn_test.xo "a writable wake before the connect finished is not a connection" (fails before the fix); the stress rerun on both Linux hosts
- Timing: none (one getpeername a dial)
- Shipped in: v0.7.0 (9863f53aa, the C runtime's dial; ported to netconn.xo in v0.12.0 by d8c7f1d69) through v0.35.0

`os.net.dial` and `dial_timeout` on native targets register the socket
with the poller before they connect it, and epoll reports a socket that
is not connecting yet as writable (EPOLLOUT | EPOLLHUP). When the poller
saw that event before the connect started, the dial woke with SO_ERROR
0, which is also the value while a connect is in progress, and returned
a connection that did not exist: a dial to an unroutable address
"connected" instead of timing out, and a dial to a slow host could hand
back a socket whose reads failed. The dial now confirms with
getpeername, as Go's does, and otherwise waits for the next edge.

## 2026-11-01: perf: a growing List uses all of its block

- Kind: perf
- Commit: this commit
- Found by: decision 0448 stage 3 (xo's self build: lists a class too large)
- Go reference: runtime/slice.go growslice (roundupsize)
- Test: std/native/llrt obj_test "a growing List's capacity fills its block"; internal/stdtest; arm64 goldens list_*, pack_lists; bootstrap fixed point; bench/perf/alloc grow

A List that grows (std/native/llrt/obj.xo grow) takes the capacity its heap block
holds, as Go's append does, not only the capacity its curve asked for. bench/perf/alloc
grow on linux/arm64: wall 0.922. xo's self build: wall 0.996, CPU 0.998, max RSS 0.986
(3 rounds, each effect under 3%). Capacity stays unobservable.

## 2026-10-31: perf: a large List grows by Go's slice step, not by doubling

- Kind: perf
- Commit: this commit
- Found by: decision 0448 stage 3 (xo's self build: doubled lists in emit's peak)
- Go reference: runtime/slice.go growslice (newcap += (newcap + 3*threshold) >> 2 past 256 elements)
- Test: arm64 goldens; bootstrap fixed point (s2 = s3); bench/perf/alloc grow and mixed; 20 million pushes against Go

A List of 256 elements or more (std/native/llrt/obj.xo grow) grows by a
quarter plus 192 elements instead of doubling, as Go's append does, so
a large list holds at most about a fifth of its capacity unused, not
half. Shorter lists still double. xo's self build on linux/arm64: wall
0.963, CPU 0.986, max RSS 0.935 (all better). Timing row (20 million pushes into one List, the Mac, medians of five; Xo / Go): time 29.6 / 57.7 ms (0.51), max RSS 212 / 340 MB (0.62); before the change Xo was 29.6 ms and 301 MB. Decision 0452.

## 2026-10-30: feature: a child's own usage, ProcOutput.usage and Child.wait_usage

- Kind: feature
- Commit: this commit
- Found by: xo bench on Windows (children_usage keeps the largest child's
  peak, so an --ab round could not read its own); owner's call 2026-10-30
  (decision 0426's addendum)
- Go reference: os.ProcessState.UserTime, SystemTime and SysUsage
  (syscall.Rusage's Utime, Stime, Maxrss)
- Test: golden proc_child_usage (arm64, LLVM, Go parity, linux/arm64 host,
  native Windows; wasm unsupported, as host_usage)
- Timing: 200 runs of /usr/bin/true each reading its usage (Xo `run` and
  `.usage`, Go os/exec and ProcessState.SysUsage), linux/arm64 offload
  host, --ab 20 rounds, Xo / Go: wall 1.10 (48.25 to 53.14 ms, within
  noise), cpu_ms 1.41 (44.11 to 62.21, too coarse), max_rss_kb 1.0

`run`, `pipeline` and `Cmd.output` give the child's own CPU time and
peak resident set as `ProcOutput.usage`, and `Child.wait_usage()` gives
a spawned child's with its status. The native runtime reads wait4's
rusage (Linux and darwin) or the child's handle (Windows) as it waits;
the Go backend reads the ProcessState (no peak on Windows). A
`ProcOutput` literal without `usage` still compiles (zeros).

## 2026-10-30: perf: List.sort_by sorts in place, each list sized once

- Kind: perf
- Commit: this commit
- Found by: ADR 0448 stage 4 (the self-build's heap profile: merge_sort
  was 6.6% of its allocation, nearly all from the compiler's regalloc)
- Go reference: slices.SortStableFunc (bench/perf/sort/go `by`)
- Test: `xo test --std-module=native/prelude` (47 passed, linux/amd64);
  every amd64 golden (460) at the change
- Timing: bench/perf/sort `by` (sort_by of 200,000 two-Int records five
  times) --ab on the linux/arm64 host: wall 0.965 (0.958 to 1.392, within
  noise), max_rss 0.729 (0.726 to 0.730, better), Xo 86 ms and 17.4 MB;
  against Go: 0.17 s and 22.4 MB (n=10, the same host)

The native List.sort_by sorted its index list with a merge sort that
built a new list each pass, growing every list by push. It now sorts the
index list in its own array (merge_sort_in_place, decision 0374's) and
gives the key, index and result lists their length once. The order is
the same: both sorts are stable. The unused merge_sort is deleted.

## 2026-10-30: behavior: resolver error texts in Xo's words, Go's where Go has them

- Kind: behavior
- Commit: this commit
- Found by: decision 0302's origin audit (glibc's gai_strerror texts were
  copied as data); owner's call 2026-10-30
- Go reference: net's errNoSuchHost and errServerMisbehaving, cgo_unix.go's
  EAI_SYSTEM case (the errno)
- Test: std/native/llrt/netconn_test.xo (`xo test
  --std-module=native/llrt/netconn`, arm64 and LLVM); goldens dns_records,
  net_lookup_dial_timeout (arm64, LLVM, Go parity)
- Timing: none (an error path's text)

A failed resolution on the native backends (os.net.dial, listen,
lookup_host and UDP's) no longer reports glibc's or libSystem's
gai_strerror text: a name with no address is `no such host`, a failed or
unanswering server `server misbehaving` (was `Temporary failure in name
resolution` and `Non-recoverable failure in name resolution`),
EAI_SYSTEM the errno's text, and the rare codes short phrases (`unknown
port` for EAI_SERVICE, was `Servname not supported for ai_socktype`).
macOS's codes are now mapped before use: its EAI_AGAIN (2) had read as
`no such host`, since minus 2 is EAI_NONAME's number on Linux.
darwin's strerror texts now come from Go's table, capitalized; every text
is unchanged.

## 2026-10-30: feature: UInt64.from_bits and to_bits

## 2026-10-30: feature: List.filter_map

- Kind: feature
- Commit: this commit
- Found by: Xo Lang 1's triage of the 2026-10-30 language obstacles note
  (a protocol's unsigned 64 bit field held in an Int)
- Go reference: uint64(x) and int64(u), math.Float64bits style names
- Test: internal/llvm/testdata/golden/uint64_bits (arm64, LLVM, Go
  parity)
- Timing: none: no instruction on any backend (an iconv between two 64
  bit integers)

`UInt64.from_bits(x)` and `u.to_bits()` move an Int's 64 bits into a
UInt64 and back unchanged, where `UInt64(x)` faults on a negative Int.

- Go reference: none in Go's std (Rust's Iterator::filter_map); the Go
  backend's List_filter_map is the map and filter loop in one
- Test: internal/llvm/testdata/golden/list_filter_map (arm64, LLVM, Go
  parity)
- Timing: none of its own: list_map's loop with one Option test

`xs.filter_map(f)` keeps f's Some values in order, a map and a filter in
one pass; an error from f stops it, as map's.

## 2026-10-30: perf: read_mapped, a file mapped read only

- Kind: perf
- Commit: this commit
- Found by: Lang 1 (the link's link.read_store lap, decision 0402)
- Go reference: none in Go's standard library (golang.org/x/exp/mmap's ReaderAt, syscall.Mmap)
- Test: the fs_read_mapped golden (read_mapped's bytes equal read's; appends, transforms and slices of a mapped file copy; a rename over the mapped file; many mapped files dropped; an empty and a missing file) on arm64, LLVM, the Go backend and wasm
- Timing: xo bench --ab of a fib edit build (darwin/arm64, separate compilation, 20 rounds, none loaded), xo before against after: link.read_store 2.535 to 0.06 ms (0.024, better), the link 7.29 to 4.455 ms (0.611, better), max RSS 0.969 (better), the edit's wall 70.15 to 72.34 ms (1.031, 0.942 to 1.084: within noise)

`read_mapped(fs, path)` is `fs.read(path)`, except that on the native
linux and darwin backends the file is mapped read only and private
instead of copied: the result is a view of the mapping, so every write
of it copies, and its last drop unmaps it. The program must not
truncate the file or change it in place while the Bytes lives (rename
over it is safe). xo's link maps std's store with it.


## 2026-10-30: perf: a connection deadline set before each read no longer wakes the poller

- Kind: perf
- Commit: this commit
- Found by: decision 0430 stage A's timing row (Xo 1.13x Go with a read deadline set before each read)
- Go reference: runtime netpoll's per-descriptor timers (modtimer wakes the netpoller only for an earlier when)
- Test: internal/llvm/testdata/golden/conn_deadline_move (arm64, LLVM, Go parity): a deadline moved earlier while a read waits, one moved later that then passes while the poller sleeps on the earlier one, one cleared; conn_deadline and tls_deadline unchanged
- Timing: xo bench --ab on the linux/arm64 offload host, 15 rounds, 100,000 one byte echo round trips over loopback with a read deadline set before each read: 3fd710781 against this commit wall 0.873 (0.863 to 0.876), cpu 0.857, Go against this commit wall 0.956 (0.955 to 0.961), so Xo is now faster than Go here (it was 1.13x); without deadlines Go against this commit 0.970 (0.966 to 0.977)

Each connection keeps one read and one write timer in the runtime's
heap, rearmed in place, instead of a timer made and removed per wait.
Moving a timer later no longer wakes the poller: it already waits no
later than the old deadline and looks again when that passes. A wait
with a deadline wakes workers as often as one without (XO_RTSTATS wake
1,608 against 23,375 before, darwin, 100,000 round trips).

## 2026-10-30: feature: connection deadlines (Conn, ServerConn)

- Kind: feature
- Commit: this commit
- Found by: the backend gap survey of 2026-10-30, gap 5 (queueworker, chathub, grpcsvc), decision 0430
- Go reference: net.Conn SetDeadline, SetReadDeadline, SetWriteDeadline (os.ErrDeadlineExceeded)
- Test: internal/llvm/testdata/golden/conn_deadline (arm64, LLVM, Go parity): a read past its deadline, the deadline staying passed until it moves, a deadline already passed with data waiting, one set while a read waits, a write deadline against a peer that does not read, the ServerConn side
- Timing: xo bench --ab on the linux/arm64 offload host, 15 rounds, not loaded, 100,000 one byte echo round trips over loopback, A Go, B Xo native: without deadlines wall 0.983 (0.863 to 1.052, within noise); with a read deadline set before each read (Go's SetReadDeadline) wall 1.13 (1.124 to 1.134), Go's own deadline costing it 4% and Xo's 18% before setting one stopped leaving a readiness for the next wait (poll.pd_kick); the rest is a timer made per wait where Go resets one per descriptor (a follow-up for Xo Lang 1, the runtime); the program is in the lead's notes (dlbench), not the repo

`set_deadline(clock, t)`, `set_read_deadline` and `set_write_deadline`
give a connection Go's deadlines: a read or write past `t` fails with
`Timeout` and the connection stays usable once the deadline moves.
Natively each connection record keeps a read and a write deadline on
the runtime's timers, which the existing poll waits take; setting one
wakes a waiting read or write. The Go backend calls net.Conn's setters
and keeps the user's deadline for its own cancellation, which used the
same setters.

## 2026-10-30: fix: a relative symbolic link to a directory on Windows

- Kind: fix
- Commit: this commit
- Found by: the lead's native Windows golden rerun after Developer Mode (2026-10-30, fs_symlink_info: "ld: permission denied")
- Go reference: os.Symlink on Windows (isDir resolves a relative oldname against filepath.Dir(newname))
- Test: internal/llvm/testdata/golden/fs_symlink_info, natively on the Windows offload host
- Timing: none (one GetFileAttributesW a link, not a hot path)

`fs.symlink(target, path)` on native windows/amd64 decides between a file
and a directory link by looking at target in the link's own directory,
as Go does, not in the working directory: `root.symlink("d", "ld")` made
a file link to a directory, and stat through it was "permission denied".
Removing a link (`remove`, `remove_all`) that is a directory link uses
RemoveDirectoryW when DeleteFileW fails, as Go's os.Remove does; it
removes the link, never the directory it points to.


## 2026-10-30: feature: Net.interfaces

## 2026-10-30: feature: Proc.usage and Proc.children_usage

- Kind: feature
- Commit: this commit
- Found by: std-coverage gap 9 (the metrics exporter's host facts), decision 0426
- Go reference: net.Interfaces and Interface.Addrs (interface_linux.go netlink, interface_windows.go GetAdaptersAddresses)
- Test: the host_ifaces golden (arm64, LLVM, Go parity, the linux offload hosts, natively on Windows; Io under WASI); every interface's text the same natively and on the Go backend
- Timing: xo bench --ab, 5,000 `os.net.interfaces()` calls summing the addresses, A Go net.Interfaces plus each interface's Addrs (sorted by index), B Xo native arm64, 15 rounds, macOS arm64 under load (ratios only): wall 0.254 (0.248 to 0.258), instructions 0.252, max RSS 0.253; the same sum on both (105,000, 26 interfaces); Go asks the system again for each interface's addresses, Xo reads them in the one dump; the program is in the lead's notes (hfbench), not the repo

`os.net.interfaces()` lists the host's network interfaces sorted by
index, each with its name, MTU, hardware address, flags and addresses as
CIDR text. An attenuated Net answers it too. Breaking: a type that
implements the `Net` interface needs an `interfaces` method.

- Go reference: syscall.Getrusage (RUSAGE_SELF, RUSAGE_CHILDREN); Windows GetProcessTimes and K32GetProcessMemoryInfo
- Test: the host_usage golden (arm64, LLVM, Go parity, natively on Windows; zeros under WASI), user_caps (a user Proc)
- Timing: xo bench --ab, 200,000 `os.proc.usage()` calls, A Go syscall.Getrusage(RUSAGE_SELF), B Xo native arm64, 15 rounds, macOS arm64 under load (ratios only): wall 1.188 (1.151 to 1.203), instructions 1.002 (1.001 to 1.002), max RSS 0.599; the program is in the lead's notes (hfbench), not the repo

`os.proc.usage()` is this process's user and system CPU time and peak
resident set in bytes, `children_usage()` the same for its waited
children. Breaking: a type that implements the `Proc` interface needs
both methods.
## 2026-10-30: fix: Env.hostname on Windows

- Kind: fix
- Commit: this commit
- Found by: ADR 0426 (host facts, std-coverage gap 9)
- Go reference: os.Hostname on Windows (GetComputerNameExW with ComputerNamePhysicalDnsHostname)
- Test: the env_hostname golden's "named true" on arm64, LLVM, the Go backend and natively on the Windows offload host; "named false" under WASI
- Timing: none (one system call, not a hot path)

`os.env.hostname()` on native windows/amd64 is the host's name from
GetComputerNameExW (ComputerNamePhysicalDnsHostname, as Go's), not "".
WASI still answers "".
## 2026-10-30: feature: Fs.disk_usage

- Kind: feature
- Commit: this commit
- Found by: std-coverage gap 9 (the metrics exporter's host facts), decision 0426
- Go reference: syscall.Statfs (Linux, macOS), windows.GetDiskFreeSpaceEx
- Test: the host_disk golden (arm64, LLVM, Go parity, natively on Windows; Io under WASI), user_fs (a user Fs), a MemFs test in cmd/xo/testdata/nogc/same
- Timing: xo bench --ab, 200,000 `fs.disk_usage(".")` calls, A Go syscall.Statfs, B Xo native arm64, 15 rounds, macOS arm64 under load (ratios only): wall 1.087 (1.082 to 1.097), instructions 1.004 (1.003 to 1.004), max RSS 0.543; the program is in the lead's notes (hfbench), not the repo

`fs.disk_usage(path)` is the space of the file system holding `path`:
`DiskUsage{total, free, available, files, files_free}` in bytes and
inodes (no inodes on Windows). Breaking: a type that implements the `Fs`
interface needs a `disk_usage` method.
## 2026-10-30: fix: Proc.ppid on Windows

- Kind: fix
- Commit: this commit
- Found by: the lead's native Windows golden run (2026-10-30, proc_pid: "ppid positive false")
- Go reference: os.Getppid, syscall.Getppid on Windows (go1.26.1: a CreateToolhelp32Snapshot walk)
- Test: internal/llvm/testdata/golden/proc_pid, natively on the Windows offload host

`os.proc.ppid()` on windows/amd64 is the parent's ID, found in a process
snapshot (CreateToolhelp32Snapshot, Process32FirstW/NextW) as Go's is,
instead of 0; 0 still when the snapshot fails. One snapshot a call, not
a hot path, so no timing row.

## 2026-10-30: fix: File.sync of a read handle on native Windows

- Kind: fix
- Commit: this commit
- Found by: the native Windows golden run (fs_atomic: "synced.txt: permission denied")
- Go reference: none (Go's File.Sync returns the error on Windows; Xo keeps the Linux and macOS result)
- Test: fs_atomic on the owner's windows/amd64 box
- Timing: none (an error path)

`File.sync` on a file opened for reading succeeded on Linux and macOS
(fsync of a read only descriptor) but failed on native Windows, where
FlushFileBuffers needs write access and returns ERROR_ACCESS_DENIED; a
read handle has nothing to flush, so that is success there too.

## 2026-10-29: feature: create_dir_all_mode

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-29 std survey (std-coverage.md gap 16, a directory tree made private)
- Go reference: os.MkdirAll with a permission (go1.26.1)
- Test: scripts/stdcorpus prelude.go "create_dir_all_mode gives only the directories it makes the mode" (MemFs; go, arm64 and LLVM); internal/llvm/testdata/golden/fs_mkdir_mode (the real file system, native and Go)

`create_dir_all_mode(fs, dir, mode)` creates dir and its missing parents
and gives each one it created exactly mode (no umask), deepest first;
existing directories keep their mode. A free function, so a user type
that implements `Fs` still does. Not a hot path (a few system calls a
directory), so no timing row.

## 2026-10-29: feature: FileInfo.is_symlink

- Kind: feature (breaking)
- Commit: this commit
- Found by: gap 16 of the 2026-10-29 std survey (a tool that walks a
  tree must not follow or copy links as files); owner approved 2026-10-29
- Go reference: os.Lstat, fs.DirEntry.Type() & fs.ModeSymlink
- Test: internal/llvm/testdata/golden/fs_symlink_info (native and Go),
  corpus prelude (MemFs on every backend)

`FileInfo` has `is_symlink: Bool = false`. `read_dir` and `walk` report
a symbolic link itself, as Go's os.ReadDir entries (Lstat): a link to a
directory is not a directory and walk does not descend into it, as
before; `is_symlink` now says why. `stat` follows the link and reports
the target, so it is false there. Native reads it from lstat's st_mode
(S_IFLNK), Windows from an unfollowed reparse point, WASI from filetype
7. Breaking: a pattern that destructures FileInfo field by field names
the new field; a struct literal keeps working through the default. No
hot path, so no timing row.

## 2026-10-29: feature: write_atomic and File.sync

- Kind: feature
- Commit: this commit
- Found by: gap 16 of the 2026-10-29 std survey (a config or state file
  half written after a crash)
- Go reference: os.File.Sync; the temporary file, sync, rename and
  directory sync of github.com/google/renameio and natefinch/atomic
- Test: internal/llvm/testdata/golden/fs_atomic (native and Go), corpus
  prelude (MemFs on every backend)

`write_atomic(fs, path, data)` writes a temporary file beside path,
syncs it, copies path's permission bits (0o644 for a new file), renames
it over path and syncs the directory (not on Windows). `File.sync`
stayed public: fsync, F_FULLFSYNC on macOS, FlushFileBuffers on
Windows. write_atomic's native path syncs by path (open, fsync, close),
so it needs no File. Also fixed: the Go backend's MemFs rename dropped
the file's mode and link target (a renamed file came back 0o644); it
keeps them now, as native MemFs does. No hot path, so no timing row.

## 2026-10-29: feature: walk_dir and WalkStep

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-29 std survey (std-coverage.md gap 16: a tool skipping .git and node_modules)
- Go reference: path/filepath.WalkDir, fs.SkipDir, fs.SkipAll (go1.26.1)
- Test: scripts/stdcorpus prelude.go walk_dir test (MemFs; go, arm64 and LLVM); internal/llvm/testdata/golden/fs_walk_dir (the real file system: a symbolic link not followed, SkipDir on a directory and on a file, Stop, visit with an effect, a sub, a missing directory); bench/perf/walkdir twin (Go checksum walk 3172000 equal)

`walk_dir(fs, dir, visit)` walks as `walk` does and lets visit skip a
directory or stop; it returns the visited entries. Differences from Go:
dir itself is not visited, and the first read error ends the walk. A
free function, so a user type that implements `Fs` still does. Timing
against Go's filepath.WalkDir (bench/perf/walkdir walk, quiet --ab, 15
rounds, Go as A, 2026-10-29): instructions 1.263 (1.256 to 1.27),
slower; wall 1.35 (1.208 to 1.466) on a loaded machine (28% spread), so
the instruction ratio is the figure. Each entry is an lstat and a
FileInfo where Go's DirEntry reads the type from the directory listing.

## 2026-10-29: feature: Proc.pid and Proc.ppid

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-29 std survey (std-coverage.md S8, os pid)
- Go reference: os.Getpid, os.Getppid; decision 0410
- Test: internal/llvm/testdata/golden/proc_pid (native and Go)

`os.proc.pid()` and `os.proc.ppid()` return the process and parent IDs
(Windows: the parent is 0; WASI: both 0). Breaking: a user type that
implements `Proc` needs the two methods. No hot path, so no timing row.

## 2026-10-29: feature: Proc.notify, stop_notify and request_stop

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-29 std survey (std-coverage.md S2): a gateway's
  reload on SIGHUP
- Go reference: os/signal Notify and Stop (a full channel drops the
  signal; a subscribed SIGINT loses its default), Windows console events
  as SIGINT/SIGTERM; decision 0407
- Test: internal/llvm/testdata/golden/signal_notify (native and Go)

`os.proc.notify([Signal.Hangup, ...])` delivers Hangup, Interrupt, Quit,
Terminate, User1, User2 and WindowChange to a channel; a subscribed Interrupt
or Terminate no longer starts the shutdown until `os.proc.request_stop()`;
`stop_notify` restores the defaults.
## 2026-10-29: feature: Float.to_fixed, to_exp and to_general

- Kind: feature
- Commit: this commit
- Found by: the std-vs-Go survey (item 12: fixed precision float text, `%.3f`, `%e`, `%g`)
- Go reference: strconv.FormatFloat 'f', 'e' and 'g' (bigFtoa's exact decimal, decimal.Round, fmtE, fmtF, %g's exponent rule) with fmt's width, zero and plus
- Test: std/native/prelude_test.xo, 3,800 rows against strconv.FormatFloat (finite and not, shortest and 0 to 25 digits, random bit patterns, ties and subnormals) and the text_format golden on arm64, LLVM and the Go backend
- Timing (2026-10-29, quiet --ab, 10 rounds, 300k iterations of to_fixed(3) and to_fixed(10)): Xo 67.9 ms against Go's strconv.FormatFloat 26.6 ms, so 2.55x slower (instructions 2.6x); before the fixedFtoa port (up to 18 digits from 128 bit powers of ten) and the BytesBuilder layout it was 371.6 ms (15x). Keeping the digits as an integer (no List[Int] per number) cut the wall time to 0.599 of that (quiet --ab, 10 rounds, 0.591 to 0.608; instructions 0.58), about 40.7 ms, so roughly 1.5x Go's (derived from the ratio, not a same-run Go row). What is left is the Str copy.

`f.to_fixed(prec, width, zero, plus)`, `to_exp` and `to_general` give strconv.FormatFloat's text with `prec` digits (below 0 the shortest that round trips) and Int.to_str_width's padding. The digits are found exactly (the significand times a power of two as decimal digits) and rounded half to even, as Go's bigFtoa; Go's fixedFtoa fast path for up to 18 digits is not ported.

## 2026-10-29: feature: List.heap_push, heap_pop, heap_remove, heap_fix and heapify

- Kind: feature
- Commit: this commit
- Found by: the std-vs-Go survey (item 13: a scheduler needed a priority queue with remove and fix)
- Go reference: container/heap (Init, Push, Pop, Remove, Fix; the same up and down, so the same swaps)
- Test: internal/stdtest/testdata/corpus/heapops, generated by scripts/stdcorpus/heapops.go from container/heap on 300 random operation sequences (pushes, pops, removes, fixes, heapify; ties and distinct keys), the returned values and the final list equal on arm64, LLVM and the Go backend
- Timing (2026-10-29, quiet --ab, 10 rounds, 1M pushes of random Ints with a pop every third push, then a drain): Xo 82.3 ms against Go's container/heap (heap.Interface on an IntHeap) 169.8 ms, so 2.06x faster in wall time (0.49 of Go's; instructions 0.32 of Go's). Go's interface calls for Less and Swap are the difference.

A list of `Ord` elements is a binary min heap: `heap_push(x)` adds x, `heap_pop()` removes and returns the least (the first of the list), `heap_remove(i)` and `heap_fix(i)` take an index as Go's do (find it with `xs.index_of(x)`; `xs.set(i, y)` then `xs.heap_fix(i)` is Go's `h[i] = y; heap.Fix(h, i)`), and `heapify()` is `heap.Init`. A priority queue by a key is a heap of (key, value) pairs. Each is O(log n).

## 2026-10-29: feature: Env.hostname

- Kind: feature
- Commit: this commit
- Found by: the std-vs-Go survey (item 9: an exporter or scheduler labels its output with the host's name)
- Go reference: os.Hostname (uname's nodename on Linux, gethostname on macOS)
- Test: the env_hostname golden (a real Env, an Env that lists no variable, and a user's own Env) on arm64, LLVM and the Go backend, and a FakeEnv test
- Timing: none (one system call)

`env.hostname()` is the host's name, or "" where the system cannot say (Windows and WASI for now). It is an environment fact and not a variable, so `env.only([])` still reports it; a FakeEnv answers its HOSTNAME variable, else "test". Breaking: a type that implements the `Env` interface needs a `hostname` method (decision 0410).

## 2026-10-29: feature: Str.pad_left, pad_right and center, and Int.to_str_width

- Kind: feature
- Commit: this commit
- Found by: the std-vs-Go survey (item 12: CLI programs padded text by hand)
- Go reference: fmt/format.go fmtInteger and padString (`%8d`, `%08d`, `%-8d`, `%+d`, `%x`, `%b`, `%*s`, `%-*s`)
- Test: std/native/prelude_test.xo, 720 rows of Int.to_str_width against Go's fmt (every width, zero, minus and plus combination in bases 10, 16 and 2, including the smallest Int) and rune width cases of the pad methods
- Timing: none (not a hot path: one repeat and one concatenation; the 720 rows run in 2 ms)

`s.pad_left(width, fill)`, `pad_right` and `center` pad to `width` runes with one rune of fill (a space by default), returning `s` itself when it is that long already. `n.to_str_width(width, base, zero, plus)` is Go's integer verbs with a width: spaces on the left, zeros after the sign with `zero`, spaces on the right for a negative width, `+` for a positive number with `plus`. Fixed precision floats follow in the next entry (decision 0410 stage 1b).

## 2026-10-29: feature: Net.dial_timeout and Net.lookup_host

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-29 std survey (std-coverage.md S7): an exporter
- Go reference: net.DialTimeout, net.LookupHost (DNSError.IsNotFound's
  "no such host"); decision 0406
- Test: internal/llvm/testdata/golden/net_lookup_dial_timeout (native and Go)

`Net.dial_timeout(addr, d)` bounds the lookup and the connect, `Timeout` past
`d`. `Net.lookup_host(host)` returns the addresses `dial` would try, from the
same resolver; an unknown name is `lookup NAME: no such host`; Net.only and
FakeNet apply. `netip.lookup` is its typed form.

## 2026-10-28: fix: a timer or network wake is not left behind a busy pair of tasks

- Kind: fix
- Commit: ff74fbff0
- Found by: the 0.33.1 release tier (internal/llvm TestGoldenTasks/fair_pairs at XO_THREADS=2)
- Go reference: none (Go's scheduler checks the network poller and timers on every schedule round, runtime/proc.go findRunnable)
- Test: fair_pairs golden (24 ping pong pairs on 2 workers with a 20 ms timer), direct_fair
- Timing: fair_pairs at 2 threads, the worst wake of the 20 ms timer: 5.2 s late before, 57 to 134 ms after; 4 threads 2.6 s before, 57 ms after

With 24 ping pong pairs on 1 or 2 workers the ring and the inject queue were never empty, and the scheduler's fairness step polled timers and the network only when they were, so a `Clock.sleep` or a socket read could wake seconds late. The fairness step now polls first and each worker serves the ring once a slice. See gotchas/run-next-pair-starves-timers.md.

## 2026-10-28: perf: a park hands off straight to the fiber its wake queued

- Kind: perf
- Commit: f9dcc5d3b
- Found by: Xo Lang 3 (decision 0394; ping pong between tasks was 1.244 times Go's wall)
- Go reference: runtime/proc.go runnext (a goroutine readied by the running one runs next, on the same stack switch path)
- Test: direct_fair golden (a pair on one worker with a 20 ms timer and a CPU task: the timer still fires and the sum is unchanged)
- Timing: ping pong of 2M messages, quiet `xo bench --ab`: wall 0.698 (0.694 to 0.702) of XO_DIRECT=0, 0.864 of Go's (was 1.244); bench/perf/conc fanout, mutex, pmap, rwmutex and spawn within noise

A task that parks and wakes the fiber it just queued switches to it with one stack switch instead of two. `XO_DIRECT=0` turns it off. The same change made `find_work` serve the ring, the inject queue and the poller once a slice before the run-next slot: a ping pong pair on one worker had starved a 20 ms timer for 2 to 10 minutes.

## 2026-10-28: feature: BytesBuilder.put_le and put_be store in place

- Kind: feature
- Commit: this commit
- Found by: Xo Lang 2 (the linkers apply relocations by slicing and byte-at-a-time writes: about 5 of 10 ms of a Windows link's layout)
- Go reference: encoding/binary LittleEndian.PutUint16/32/64 and BigEndian on a slice (std/binary.xo has the append and read forms)
- Test: bytesbuilder_put golden (every width, both orders, truncation, a shared builder, the faults)
- Timing: macOS arm64, a 1 MB image with 65536 four byte patches (the linkers' write_patched shape: gap slices plus byte writes, A; one write plus put_le per patch, B), quiet `xo bench --ab`, 10 rounds: wall 36.6 ms to 10.0 ms (ratio 0.273, 0.268 to 0.281), instructions 0.203, cycles 0.242.

`b.put_le(at, v, n)` and `b.put_be(at, v, n)` overwrite the `n` bytes (1, 2, 4 or 8) at `at` with the low bytes of `v`, in place: no copy, no growth. A range outside `0..len()` or another width is a fault. See decision 0399.

## 2026-10-28: perf: Fs.copy clones on macOS and copies in the kernel on Linux

- Kind: perf
- Commit: this commit
- Found by: xo-lang-4 (the run cache's run_store copies each built binary on every edit build: about 2 ms of a 110 to 160 ms fib edit, more for the 7 MB HTTP server)
- Go reference: none (Go's os.File.ReadFrom uses copy_file_range on Linux and fcopyfile/clonefile is not used on macOS; Apple's cp uses clonefile)
- Test: fs_files and fs_ops_edges goldens (new file, over an existing file, directory, missing source; all take the loop or the clone with the same results)
- Timing: macOS, 40 copies of a 7 MB file, quiet `xo bench --ab`, 15 rounds, A the read/write loop, B clonefile: wall 79.5 ms to 7.0 ms (ratio 0.087, 0.063 to 0.09), instructions 0.121. The Linux copy_file_range path is not timed here.

`Fs.copy` on macOS tries clonefile(2) first (APFS copy on write, one volume, the destination
must not exist) and falls back to the read and write loop on any failure (an existing
destination, another volume, no APFS). On Linux the loop's first step is copy_file_range(2), which
copies in the kernel and shares blocks on a reflinking filesystem; a first call it does not serve
(EXDEV, EINVAL, ENOSYS) takes the loop. The contract is unchanged: b gets a's contents and
permission bits, also set on an existing b.

## 2026-10-27: perf: a Str slice in a loop checks its range inline

- Kind: perf
- Commit: this commit
- Found by: path's parts profile (prelude.llrt_str_slice_chk 19%, a call at every `s[lo..hi]`: 102 inliner instructions, refused for its landing pads)
- Go reference: none (Go's slice bounds check is inline code with a call to panicSlice*)
- Test: test/codegen/strs count_ab (arm64 and amd64: the cold str_slice_bad is named and llrt_str_slice_chk is not; both fail on the parent's xo); internal/stdtest corpora; fixed point (s2 and s3 identical)
- Timing: quiet --ab, 15 rounds, the parent's xo and std as A: bench/perf/path parts 0.922 (0.902 to 0.937), instructions 0.896; strsplit split 0.994, strsplit lines 0.998, mail list 1.002, trimchars trim 0.990 (within noise); xo __text 63,508,348 to 63,521,500 bytes (+0.02%), s2 42 s to 43 s

llrt_str_slice_chk tests `0 <= lo <= hi <= n` as two unsigned compares
and the bytes at lo and hi for a UTF-8 continuation, and calls the new
str_slice_bad for anything else, which raises the same faults as before
(the range one, or "splits a UTF-8 sequence"). str_slice_bad is a slow
path of ssa.cold_fn, so the check is 58 inliner instructions: inlined at
call sites in a loop, still a call elsewhere (16 sites over the budget of
40). Counting a cold call once for the whole function, not once per site,
is the way to inline it everywhere (the timing audit's note).

## 2026-10-27: perf: List[Int].sort is Go's pdqsort

- Kind: perf
- Commit: this commit
- Found by: Intel tester 0.33.0 (sort1M 1.64x Go on an i7; the profile put 330 of 440 ms on the merge's mispredicted branch, which isel keeps a branch because its condition is on loaded values)
- Go reference: slices.Sort (pdqsortOrdered, slices/zsortordered.go), ported with its pivots, partial insertion, equal partition and heapsort fallback; pattern breaking draws from an LCG in Int range instead of xorshift (moves which elements swap, never the result)
- Test: prelude_test "sort of Ints": every branch's size (0 to 20000), sorted, reversed, equal, few distinct, sawtooth and organ pipe inputs, extreme values and a near sorted list, each against sort_in_place's stable merge
- Timing: 1M LCG Ints, best of 5: arm64 54.7 ms against Go's 56.5 (0.97; the merge 59.0); Intel i7-9750H 82 to 84 ms against Go's 67 to 69 (1.23; the merge 114 to 117)

`sort` of a `List[Int]` takes this path through the same any_get check
that sends `List[Str]` to list_sort_str; equal Ints are indistinguishable,
so the order is the stable merge's. Float and every other type keep the
merge (`-0.0` and `0.0` compare equal but differ).
## 2026-10-26: perf: Float.fma uses x86-64's FMA3 when the CPU has it (decision 0385 stage 3)

- Kind: perf
- Commit: this commit
- Found by: decision 0385's plan
- Go reference: math.FMA (Go uses vfmadd231sd behind x86HasFMA)
- Test: golden float_fma on the owner's Intel Mac (hardware path), in Docker linux/amd64, and with HW_FMA forced off in Docker (software path): all equal to want.txt; arm64, LLVM, Go parity and wasm goldens unchanged; the golden prints NaN results as `nan` (x86's default NaN is negative)
- Timing: Intel Mac, 10M dependent calls, median of three: 3.0 ns a call against Go's 1.9 (1.58; the call and the hw() check per call), the software path 19.1 against Go's fma.go 15.4

On x86-64 a CPU with FMA, AVX and an OS that saves the AVX state runs
`x.fma(y, z)` as vfmadd231sd; any other runs Go's software algorithm, with
the same results.
## 2026-10-26: perf: Float.fma is one fmadd on arm64 (decision 0385 stage 2)

- Kind: perf
- Commit: this commit
- Found by: decision 0385's plan (stage 1 was the software body)
- Go reference: math.FMA, which Go lowers to fmadd on arm64
- Test: golden float_fma (the 20,000 case digest unchanged) on arm64 TestGoldenRun, LLVM TestGolden at O1 and O2, wasm; float_no_fma unchanged; test/codegen/floats: `x.fma(y, z)` is one fmadd with no call on arm64 and @llvm.fma.f64 on LLVM, `x * y + z` keeps fmul and fadd
- Timing: a chain of 10M dependent fma calls on arm64, median of three: 1.62 ns a call (the software body 10.9) against Go's math.FMA 1.75 (0.93)

On arm64 targets the native and LLVM backends lower `x.fma(y, z)` to the
instruction; amd64 and wasm keep the software body (stage 3 adds
vfmadd231sd behind a CPUID check).
## 2026-10-26: feature: Float.fma, a multiply-add rounded once (decision 0385)

- Kind: feature
- Commit: this commit
- Found by: the owner's choice (2026-10-26): an explicit fused multiply-add, no implicit fusing
- Go reference: math.FMA (math/fma.go ported line by line to std/native/llrt/gomath.xo, BSD-style license; the Go backend calls math.FMA)
- Test: golden float_fma (32 edge cases by bit pattern and a 20,000 case digest, 2,350 of them unlike two roundings) on arm64 TestGoldenRun, LLVM TestGolden, TestGoldenGoParity and the wasm goldens; want.txt is the Go backend's output, Go's math.FMA; float_no_fma unchanged
- Timing: a chain of 10M dependent fma calls on arm64, median of three: Xo 10.9 ns a call against Go's own fma.go compiled as plain Go 10.4 (1.05); Go's math.FMA, which Go lowers to fmadd on arm64, 1.75 (stage 2 lowers Xo's to fmadd too)

`x.fma(y, z)` is `x * y + z` computed exactly and rounded once, Go's
math.FMA bit for bit on every backend; `x * y + z` written out still rounds
twice everywhere. This step is the software body (Go's algorithm: a 128 bit
product, one aligned add with a sticky bit, one rounding to nearest even).
## 2026-10-26: fix: a list swap checks each index against the list it indexes (sort_in_place and anything else that swaps)

- Kind: fix
- Commit: be41164b8
- Found by: reading calls.xo bounds_check for Proof.div_of_len: for rt.list_swap(xs, i, j) and rt.list_swap2(a, i, b, j) it read the index and the list off the operation's own arguments, so j was checked as i against the first list, and a dominating branch that proved i in range dropped j's check entirely; under that, the SSA builder gave the inlined swap call no source position, and bounds_check skips a value it cannot name a location for, so no native backend checked either index
- Go reference: none (a Go list assignment bounds checks every index; std/rt's list methods do too)
- Test: golden list_swap_guard (the in range swap, then `index 3 out of range for List of length 3` at exit 70) on arm64 TestGoldenRun, LLVM TestGolden and TestGoldenGoParity (want_go.txt: the Go backend's known gap, its crypto_op has no rt.list_swap_at), the wasm goldens, test/codegen/intrinsics' two swap shapes (b.hs present for two entry parameters, absent for two lengths the guards prove; the parent's xo fails the first), arm64 and LLVM goldens 387 of 387, tests short
- Timing: bench/results/sortip-swap-checks-2026-10-26.md

A swap now compares both indexes with the list's length before it loads or
stores, on arm64, LLVM and wasm alike, and a fault from a swap is a fault the
calling function's defer sees (faulting_rt). The cost is priced on the one
hot path that keeps the checks: sort_in_place's merge, whose indexes come
from the caller's `hi <= xs.len()` clamp, a fact the callee cannot see and
Proof cannot record (it keeps only a branch's true edge). ip_ints goes to
1.137 of the parent's instructions and ip_u8 to 1.138 (wall 1.031 and 1.136),
ip_strs to 1.056, with the gen controls at 1.000; over the 34 program sweep
+0.008 percent, since text (-578) and json (-324) gain more than the three
programs that link std/internal's probes lose. The file above ranks the two
ways to get it back: memoize the list_len load a check adds, and record the
fall-through fact of a return guard so the merge's indexes prove.

## 2026-10-26: fix: the merge sort's cursors are bounded by a comparison the arm64 proof reads

- Kind: fix
- Commit: this commit
- Found by: measuring the swap index checks the merge paid after the arm64 bounds proof gained guard facts: `while i < left && j < hi` carries the second comparison in a phi (gotchas/bounds-proof-short-circuit.md), and the callee could not see the length fact the caller's clamp had
- Go reference: none (Go's sort does not share this lowering)
- Test: golden list_sort_guard (arm64, LLVM, the Go backend), the arm64 goldens that sort, test/codegen/lists guard_fact and clamp_fact fail on the base commit's xo and pass here, test/codegen/listbounds (the boundary of the fact: le_not_lt, bad_clamp and ge_guard_no_nonneg keep their check, ge_guard_nonneg is a regression guard that already held), compiler/arm64 proof_test.xo

`merge_runs_in_place` says the fact once per merge: `if hi > xs.len()`
calls `merge_end_past(hi, xs.len())`, a non generic `-> Never` helper
whose body is one `rt.fault_raise("merge_runs_in_place: ...", "")`, so one
cold body and one message serve every element type the sort is
instantiated at. The idiom is the prelude's own: this module imports the
`fault` module, which shadows the `fault(...)` expression, so the raise is
`rt.fault_raise` with an empty position, the shape `Bytes.zeroed`'s fault
already has (the message names the function instead of a hand written
`file:line` that drifts). A broken caller stays loud as it did on its
first read past the end, and the proof turns the edge into `hi <=
xs.len()` for everything after it; with a plain `return` the tail would
have gone silently unsorted. The merge then tests its two cursors with
`while i < left { if j >= hi { break } ... }` so both comparisons reach a
branch terminator. Two of the four index checks the
hot loop paid are gone: xo bench --ab on bench/perf/sortip, 2,000,000
elements, 10 rounds inside a measurement window (load 3.21),
instructions 1,122,611,353 to 1,020,403,770 (0.909) and wall
147.2 to 141.5 ms (0.961, 95% interval 0.947 to 0.971) for `ip_ints`;
`ip_strs` instructions 0.973 with wall within noise (1.004, 0.971 to
1.012). The
slot index `w` and the placeholder loop's `lo + k` keep their checks: their
bounds are loop invariants, not comparison facts.

## 2026-10-25: fix: raw.str_slice clamps a start past the end to an empty Str (the helper behind the prelude's Str slicing of paths and names)

- Kind: fix
- Commit: 40f67b3a2
- Found by: reading raw.str_slice for the inliner (its doc says the range is clamped to s, but `lo` was only clamped below: a `lo0` past the end made `hi` equal `lo`, past the end, and the checked `s[lo..hi]` faulted)
- Go reference: none (Go's slicing panics; this helper is not a Go function)
- Test: arm64 goldens 382 of 382, the LLVM sample 40 of 40, stdfuzz strings, bytes, runes, csv, path and url (seeds 1 and 7, 1500 lines each, no divergence)

`lo` is now clamped to the length as well (`lo0.max(0).min(n)`, `hi` between `lo`
and the length), so a start past the end gives "" as the doc says; no std caller
passes one (134 callers read), so only a bug in a caller changes: it returns ""
instead of faulting. The same commit makes str_slice 8 instructions (it was 62)
so it inlines at its 52 sites; see the commit for the sweep and sizes.

## 2026-10-25: perf: the fraction of a formatted Time has no range checks

- Kind: perf
- Commit: this commit
- Found by: the arm64 assembly of `text.append_time`, whose `ns != 0`
  tail carried ten checks (`fault1` to `fault10`): the trailing zero
  countdown, `r = k - j`, `pow10`'s multiply, `digits_word(x / q, j) << 32`,
  three inside `cat`, `r + 1`, `(4 + j) << 8` and `(r + 1) << 12`
- Go reference: none (Go's `time.Time.Format` divides by a power of ten
  taken from a table, and an overflow is not a check in the language)
- Test: the time goldens on arm64, LLVM with Go parity and the wasm
  subset (time_fixed_offsets, time_parse_digits, floor_div_time,
  clock_timers, duration_parse_edges, rt_poll_timer, realtime_section), a
  301,476 line format sweep hashed against the before std, stdfuzz's time
  area (200 cases, three backends, no divergence)
- Timing: bench/results/time-fmt-tail-2026-10-25.md, arm64: `time_fmt`
  goes from 152,186,300 to 142,235,040 instructions retired (0.934, and
  0.977 of LLVM's where it was 1.046), `dur_fmt` from 133,918,809 to
  130,876,372 (0.977), the checksums unchanged

The tail's values are now written so the value ranges prove them: the
countdown masks with `0xF` (it runs at most 8 times), `j` and `r` with
`0x7`, `10^r` is a tree of constants written at the call site because a
call's result has no range there, `cat` is written out, and
`digits_word(x / q, j)` masks with `0x7FFFFFFF` before the shift.
`pow10` itself is a tree of ten constants for `frac_words`, whose
`dur_fmt` rows lose their multiply and zero checks. arm64's
`text.append_time` goes from 703 to 543 lines of assembly, and no value
changes.

## 2026-10-25: perf: sort_in_place's merge moves elements by swapping, so nothing is counted

- Kind: perf
- Commit: this commit
- Found by: step 2's measurement, where the in-place sort cost 1.12 to 1.42
  times `sort`'s instructions because every element move counted and
  released (bench/results/sortip-2026-10-25.md; ADR 0374 step 3)
- Go reference: none (Go's `slices.SortStableFunc` moves values; Xo's
  elements are reference counted, so a move costs a retain and a release
  where a swap costs none)
- Test: std/native/prelude_test.xo on arm64 and llvm (41 rows, including
  the shapes around the 16 element runs, Strs with a count read before and
  after, the shared receiver for both forms, and cycles of several lengths),
  the Go backend through bench/perf/sortip/xo's parity run
- Timing: bench/results/sortip-swap2-2026-10-25.md, 2,000,000 elements,
  arm64: instructions go from 1.12, 1.42 and 1.18 times `sort`'s to 1.06,
  1.18 and 1.07, and extra RSS from 14.3, 22.7 and 1.7 MB to 8.0, 16.0 and
  1.1 MB, because the scratch is reserved once with `List.with_capacity`
  instead of doubling as it fills; Strs wall goes 0.95 to 0.54 s against
  `sort`'s 0.72. The keyed form goes from 1.12, 1.09 and 1.12 to 1.08, 1.07
  and 1.09 times `sort_by`

The merge swaps the left run into a placeholder scratch with `rt.list_swap2`
and swaps every element of the merge back, so an element move is two loads
and two stores and never a retain or a release. The scratch starts as copies
of `xs[0]` (one counted retain a slot, released when it is dropped) and is
reserved at the widest left run's width once; the receiver is made unique
with one counted element write first, because the swaps do no copy-on-write.
The keyed form's cycle permutation rotates its cycles with `rt.list_swap`
transpositions for the same reason, which pays most on Strs (0.97 to 0.80 s
of wall).

## 2026-10-25: feature: List.sort_in_place and List.sort_in_place_by, a stable sort of the receiver

- Kind: feature
- Commit: this commit
- Found by: `sort` costs five words an element on a List[Str] (76.3 MB
  at 2,000,000 Strs on top of the list) and the floor is its signature,
  not its algorithm: returning a new list means two element arrays at
  once (bench/results/sortstr-inplace-2026-10-25.md; ADR 0374)
- Go reference: Go's `slices.SortStableFunc` on the receiver, which is
  stable like `sort` and `sort_by`; the native sort is a bottom-up stable
  merge with at most half a list of scratch
- Test: std/native/prelude_test.xo on arm64 and llvm (empty, one, the
  sizes around the 16 element runs, duplicates, strings, a shared list
  copied first, agreement with sort and sort_by, a failing key propagated
  with the list left alone), the Go backend through
  bench/perf/sortip/xo's parity run
- Timing: bench/results/sortip-2026-10-25.md, 2,000,000 elements,
  arm64: the sort's own max RSS goes 80.0 to 22.7 MB on Strs (0.28),
  16.0 to 14.3 MB on Ints, 2.0 to 1.7 MB on UInt8; instructions go 1.42,
  1.12 and 1.18 times `sort`'s, the reference counting the moves still
  pay (step 3 of ADR 0374 removes it with `rt.list_swap`); Strs are
  0.83 times `sort`'s wall today (0.63 to 0.52 s), Ints and UInt8 within
  the timer's resolution

`sort_in_place` and `sort_in_place_by` sort the receiver and return
nothing, stable exactly like `sort` and `sort_by`. A list another binding
shares is copied first, the way `push` and `set` copy one, so the other
holder keeps its order; a uniquely owned list is sorted in its own array
with the left run of each merge copied into a scratch that never grows
past half the list. The keyed form computes the keys once, sorts the
indices by them, and moves the elements inside the receiver one cycle at
a time.

## 2026-10-25: perf: parse_duration reads its digits from one address

- Kind: perf
- Commit: this commit
- Found by: the duration digit loop's remaining per digit cost, the
  bounds check and the address add of `b[i]` (bench/results/dur-parse-2026-10-24.md)
- Go reference: Go's ParseDuration indexes a string, which is an address
  and a length with no per byte check
- Test: std/native/llrt/text_test.xo on arm64 and llvm; the time and
  strconv corpora; stdfuzz area time; the wasm goldens; the full fixed
  point
- Timing: bench/results/dur-parse-2026-10-24.md

`text.parse_duration` takes the bytes' address once with
`raw.bytes_addr` and reads the digit, fraction and unit loops with
`raw.byte_at`: each loop's own `i < n` is a bound of at most the length,
so the per read check and the address add go. It is 5.6% fewer
instructions on bench/perf/text's dur_parse (0.944), with wall within
noise and RSS flat. raw.data and raw.byte_at read a view's own bytes,
which std/native/llrt/raw_test.xo shows.

## 2026-10-24: perf: one printable bitmap for the BMP, which takes is_print under the arm64 inliner's size

- Kind: perf
- Commit: this commit
- Found by: the two bitmap tests in unicode.is_print put it over the
  arm64 inliner's size limit at a call outside a loop, so class walks
  that ask is_print kept the call
- Go reference: none: unicode.IsPrint searches the ranges, and the
  bitmaps here are the port's own
- Test: TestUnicodeTables; the runes corpora on arm64 and llvm; stdfuzz
  area runes at seeds 1 to 3; the wasm goldens; the full fixed point
- Timing: bench/results/print-bitmap-2026-10-24.md

`unicode.is_print` answers every rune below U+10000 from one bitmap from
U+0080 (`unicode_tables.PRINT_BITS`, 8192 bytes, the sum of the 192 and
7936 it replaces; `PRINT_BMP` is gone), so the U+0800 test goes. The
function is 87 instructions and 21 blocks, which the arm64 inliner
treats as "few": calls inside loops, strconv's quoting and the class
walks, get the body, and calls outside one still call. classify is 0.905
of the base's instructions, the unicode quote row 0.988, the text quote
row 0.999 and the classes row 1.002, the last because its is_print call
is not in a loop.

## 2026-10-24: perf: Duration.parse takes one branch a digit instead of four

- Kind: perf
- Commit: this commit
- Found by: profiling bench/perf/text's dur_parse (107 native samples,
  76 LLVM): both backends spend 79 percent of it in the integer or the
  fraction digit loop of text.parse_duration
- Go reference: time.ParseDuration has the same two guards a digit
  (`if v > 1<<63/10` and the overflow of the multiply), which is where
  the per digit cost came from
- Test: the time and strconv corpora on arm64 and LLVM; stdfuzz area
  time; the wasm goldens; the full fixed point
- Timing: bench/results/dur-parse-2026-10-24.md

The digit test is `!(c >= 0 && c <= 9)` on the computed digit, one
unsigned compare where `c < 0 || c > 9` took two branches and a second
`sub`. The per digit magnitude guard is gone: eighteen digits cannot
leave the range, and a longer run is read again with the guard after the
loop; and with that guard the product is exact, so the second guard
folds into one test after the loop for the one sum that leaves the range
(the last digit of -2^63, which wraps positive). The fraction keeps its
guard a digit, since Go's parse saturates there, and takes the single
range test into a temporary so the value it keeps is the last one that
did not wrap: a fraction of 19 digits that passes 2^63 keeps the digits
it read, as Go's leadingFraction does (the first form of this change
kept the wrapped value and read 1s for `1.9223372036854775809s`; found
by Xo Lead reviewing the commit, 2026-10-25). dur_parse goes from
148,684,444 to 143,660,045 instructions, 0.966, wall 0.971 within noise.
The bounds check against the Bytes' length is what is left per digit.
std/native/llrt/text_test.xo pins the fraction cases, the time corpus
carries them with Go's answers, and stdfuzz's time area draws from them.

## 2026-10-24: perf: quote calls scan.span only where a run can start, and the two byte decode is one function

- Kind: perf
- Commit: this commit
- Found by: the quote profile above, where span was 32.1 percent
  cumulative and half of that was its frame returning `i` unchanged for
  every byte of a rune (a run of Greek text paid one call per rune)
- Go reference: none: strconv's quote loop asks utf8.DecodeRuneInString
  per rune and has no run scanner
- Test: the strings, strconv, template, json, jsonencode, jsondecode,
  runes and csv corpora on arm64 and LLVM; the wasm goldens; the full
  fixed point
- Timing: bench/results/quote-decode-2026-10-24.md

`unicode.quote` tests the byte at `i` and calls `scan.span` only when it
is a byte the loop keeps (0x20 to 0x7E other than `"` and `\`), so a
run of non-ASCII runes costs no span call. The quote row goes 97.8 M to
84.6 M instructions of the previous commit, 0.865, and 0.780 of the
day's base. The two byte decode moved from `unicode.decode2` to
`raw.decode2` so the rules live in one place, and `case_append`'s inline
copy is gone: the inliner puts `decode2`'s fast path in both loops, and
the 1.5 to 2.0 percent the case rows pay is the packed `(rune << 3) |
size` form's shift and mask per rune, not a call (a tuple of rune and
size is the next step there). A 16 byte `stops8` step for span, the
fast path inside `raw.decode`, and a single-return `decode2` all
measured worse and are not in the change.

## 2026-10-24: perf: quote decodes the two byte rune inline and asks a bitmap whether a rune is printable

- Kind: perf
- Commit: this commit
- Found by: the profile of bench/perf/unicode's quote workload (414
  samples): raw.decode 22.2 percent flat, is_print 21.3 percent
  cumulative with 15.0 of it in the ranges search
- Go reference: strconv's quoteWith uses utf8.DecodeRuneInString, which
  answers the two byte case inline, and unicode.IsPrint, whose Latin-1
  table is what the bitmaps here extend
- Test: TestUnicodeTables (it regenerates the tables and compares); the
  runes, json, jsonencode, jsondecode, strings, strconv, template and csv
  corpora on arm64 and LLVM; the wasm goldens; the full fixed point
- Timing: bench/results/quote-decode-2026-10-24.md

`unicode.decode2` answers the two byte sequence, the whole of Greek,
Cyrillic and accented Latin text, without calling `raw.decode`: a lead
byte in 0xC2 to 0xDF with a continuation byte in 0x80 to 0xBF gives the
rune of size 2, and a bad or missing continuation gives U+FFFD of size
1, raw.xo's rules with no call; 3 and 4 byte sequences still decode as
before. `unicode.is_print` takes the runes U+0800 to U+FFFF from a
bitmap generated with the other tables, 7,936 bytes, and keeps the
ranges search for the astral planes. The quote row goes 108.4 M to
102.1 M instructions with the decode (0.941) and to 97.9 M with the
bitmap (0.962), 0.903 of the base of both; the checksum is identical.

## 2026-10-24: perf: the Rune class binary search is unchecked, and is_letter folds its ASCII cases

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 1 (the native-vs-LLVM sweep: bench/perf/unicode
  classes); the loop's midpoint, table offset and index steps each
  carried an overflow check for a value that cannot overflow
- Go reference: unicode's RangeTable search (go1.26.1), which also
  tests `r | 32` against one range for the Latin letters
- Test: corpora runes (every class method against Go's unicode and
  strconv), where, trimchars, regexp, bytes and strings on arm64 and
  LLVM; the wasm goldens; the full fixed point
- Timing: bench/results/in-ranges-2026-10-24.md

`unicode.in_ranges` takes its midpoint with
`lo.wrapping_add(hi).shr_logical(1)` and its table offsets with
`wrapping_mul` and `wrapping_add`: `0 <= lo <= hi <= n`, so no step can
overflow, and the checked forms printed an `adds`/`b.vs` fault path per
iteration (the multiply and the other sums were already proved in
range). `unicode.is_letter` tests the ASCII letters as
`(r | 32)` in `97 to 122`, one unsigned range compare for both cases
where the two ranges were two compares and a branch, after the
`r >= 0x80` dispatch so a non-ASCII rune pays nothing extra. The
classes row is 0.870 instructions and 0.936 wall of the base; latin1
(no search) 0.998 and fieldsfunc (is_space, untouched) 1.000, the
controls.

## 2026-10-24: perf: a two level index for the Rune class search

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 1's measurement of the classes row, where the
  binary search was most of what was left after the overflow checks went
- Go reference: none: Go's RangeTable is the flat sorted table this
  replaces
- Test: corpora runes, words, trimchars, regexp, strs and where on arm64
  and LLVM; TestUnicodeTables (it regenerates the tables and compares);
  the wasm goldens; the full fixed point
- Timing: bench/results/rune-class-index-2026-10-24.md

`unicode.in_ranges` binary searched a class's whole range table on every
rune at or above U+0800: LETTER has 657 ranges, 10 steps. Each class now
has `<CLASS>_IX`, a 16 bit entry per 1,024 rune block naming the first
range a rune of that block can be in, generated with the ranges by
internal/llvm TestUnicodeTables, so a lookup is one load and a scan of
the one or two ranges the block holds. Twelve classes of 1,088 blocks
cost 26,208 bytes of static data. The classes row goes from 0.870 to
0.767 of the base (445.71 M against 581.05 M instructions), and the
checksum is unchanged. A 512 rune block (52,224 bytes) reaches 0.726;
4,096 runes (6,528 bytes) only 0.831.

## 2026-10-24: perf: Str.split_where's pieces are views made with no clamp or slice check

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 1 (the native-vs-LLVM sweep: unicode fieldsfunc at 1.28 of LLVM's instructions; XO_PROF: raw.str_slice 132 ms of samples against LLVM's 36, LLVM inlining it)
- Go reference: none (strings.FieldsFunc slices its spans the same way)
- Test: TestStdCorpora strwhere, bytes and strings on the Go backend, arm64 and LLVM; goldens strwhere, fields_view, split_view, views_split, bytes_split (arm64, LLVM with Go parity)

`str_split_where` sliced each field with raw.str_slice, which clamps both
ends, tests them for UTF-8 boundaries and calls the slice check before the
view; the spans where_spans records are in range by construction, so the
pieces are now raw.str_cut (the boundary test only, a view or a copy as
before). Bytes.split_where already takes views with only a bounds check.
Quiet `xo bench --ab` (one xo, A the parent's std, 15 rounds):
bench/perf/unicode fieldsfunc wall 0.946 (0.897 to 0.961), instructions
0.836. Against LLVM the row is 1.446 of its wall and 1.253 of its
instructions after this; the rest is the closure call per rune in
where_spans (decision 0368).

## 2026-10-24: perf: List.sort of Strs sorts indices by a seven byte prefix, so a merge never counts a string

- Kind: perf
- Commit: this commit
- Found by: bench/perf/sort strs at 1.18 of Go with equal instructions
  (e40e58193): the merges moved Strs, and each move counted the string it
  copied and released the one it replaced, two writes to cold string objects
  per element per pass (the profile: list_sort[Str] 69%, memcmp 23%)
- Go reference: slices.Sort (pdqsort); Xo's sort stays stable
- Test: golden sort_str (sizes around the run length, strings shorter and
  longer than the prefix, a NUL byte against the zero padding, non-ASCII
  bytes, duplicates, sorted and reversed input, each against an insertion
  sort); the 15 goldens that sort
- Timing: bench/perf/sort strs, xo bench --ab, 15 rounds, A and B built by
  one xo on main 9706c67e4: wall 0.894 (0.888 to 0.916), instructions 0.777;
  ints and floats within noise; against Go strs 1.18 to about 1.05 (max RSS
  1.9x Go's, the index lists)

`list_sort` (std/native/prelude.xo) hands a `List[Str]` to `list_sort_str`
(the type is told apart with `rt.any_get`, once a sort): runs of 16 by
insertion, then bottom up merges, of a list of indices ordered by the first
seven bytes of each string and then the strings, and one pass gathers the
result. The order and the stability are unchanged.

Sorting a large `List[Str]` now uses extra memory for the index lists (max
RSS about 1.9 times Go's on 200,000 Strs, 1.28 times the old sort).

## 2026-10-24: perf: Debug quoting starts its builder at the size it needs

- Kind: perf
- Commit: this commit
- Found by: profiling the template Debug quote path, where
  obj.reserve/reserve_slow/resize was 30% of the run: a fresh builder
  holds 16 bytes and doubled three or four times for each string
  (unicode.quote already pre-sizes with rs_with_cap)
- Go reference: none (internal; the Debug text is unchanged byte for
  byte)
- Test: the template, strings, strconv, json, jsonencode and jsondecode
  corpora on arm64 and llvm, the wasm goldens, and the fixed point
  (text.xo is in the fixed point set)
- Timing: bench/results/quote-presize-2026-10-24.md

`text.append_quote` (std/native/llrt/text.xo) gives an empty builder room
for the text, a quarter more for escapes and the quotes, so the Debug
text of one string is built in one allocation instead of three or four.
The quote workload runs 12.7% fewer instructions and 10% less wall time;
nested Debug text, where the builder is not empty and only the guard runs,
pays 0.1%.

## 2026-10-24: perf: fold_copy's SWAR loop, five of its instructions per word

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 1's instruction count of obj.fold_copy's loop: the
  case was tested per word, 0x20 per letter took a shift, a mask and a
  multiply, and the high-byte test repeated an AND between8 had done
- Go reference: none (internal; Go's case mapping output is unchanged)
- Test: the strings, runes, equalfold, strconv and template corpora on
  arm64 and llvm, the wasm goldens, and the fixed point (obj.xo is in
  the fixed point set); every checksum of the workloads is identical to
  the base's
- Timing: bench/results/fold-copy-2026-10-24.md

`fold_copy` (std/native/llrt/obj.xo) copies a span while folding its
ASCII case. The case now picks one of two loops (`fold_copy_lo`,
`fold_copy_up`) once instead of a compare and a select per word, a word
with no letters adds zero so the `letters != 0` branch is gone too, 0x20
per letter is `letters.shr_logical(2)` instead of a shift, a mask and a
multiply, and `between8(...) & (w ^ -1)` replaces the masked form, since
`between8` already keeps only byte high bits. ASCII case mapping runs 16
to 18% fewer instructions and 9 to 10% less wall time; mixed text, where
decoding and the case tables dominate, 1 to 2%.

## 2026-10-24: perf: a 2-byte case in the UTF-8 validation behind every Bytes-to-Str conversion

- Kind: perf
- Commit: this commit
- Found by: profiling a non-ASCII heavy JSON decode, where
  scan.utf8_valid and scan.seq_len were 34% of the run (bench/results/
  decode-2026-10-24.md)
- Go reference: none (internal; utf8.Valid's rules unchanged)
- Test: scan_test.xo (std/native/llrt) covers the 2-byte bounds, the
  overlong C0 and C1 leads, a lone continuation byte, the surrogate
  range at both ends, and truncation at the end of the range for 2-, 3-
  and 4-byte sequences, plus utf8_valid on each malformed sequence
  inside a run of ASCII; the json decode corpora and the wasm goldens
  (validation output is unchanged)
- Timing: bench/results/utf8valid-2026-10-24.md

`seq_len` (std/native/llrt/scan.xo) answers the 2-byte case before its
ladder: a lead byte in `0xC2` to `0xDF` with one continuation byte in
`0x80` to `0xBF` is size 2, anything else size 1, and the ladder now
carries only the 3- and 4-byte leads and the invalid ones. JSON decoding
of Greek and Cyrillic text runs 5.2% fewer instructions; an ASCII only
document is unchanged (it never reaches `seq_len`), CJK and emoji text
pays 0.1% for the extra range test, and the letter-class regexp row does
not move.

## 2026-10-24: perf: Bytes.trim_prefix and trim_suffix compare with words, as Str's do

- Kind: perf
- Commit: this commit
- Found by: `bench/results/bytes-split-2026-10-23.md` had the affix row
  (trim_prefix and trim_suffix) as the one row where Xo ran more
  instructions than Go's bytes package (1.80 gross, 1.96 net of the loop)
- Go reference: bytes.TrimPrefix and TrimSuffix through HasPrefix and
  Equal (go1.26.1)
- Test: corpus/bytes on arm64, LLVM and the Go backend (its TrimPrefix
  and TrimSuffix rows), goldens bytes_split and bytes_transforms (arm64,
  LLVM, Go parity)
- Timing: bench/results/bytes-affix-2026-10-24.md

`Bytes.trim_prefix` and `trim_suffix` tested with `scan.eq_at`, the
general five argument compare, whose branch ladder falls to a byte loop
below four bytes; the profile of the affix workload put 42% of the row
there (its affixes are 1 to 3 bytes, so the word paths never ran). They
now call `scan.eq_bytes`, which is `scan.eq_text`'s shape for Bytes: the
first and last word on `raw.bytes_addr`, no Bytes values and no bounds
checks, since the caller has already tested `p.len() <= b.len()`.
`scan.eq_at` is unchanged for its other callers. Affix row 49.15 M to
44.04 M instructions against Go's 38.2 M at one revision (1.29 to 1.15
gross, 1.32 to 0.97 net of the benchmark's loop).

## 2026-10-24: perf: case_append encodes a changed rune in place

- Kind: perf
- Commit: this commit
- Found by: the mixed case mapping profile; each changed rune took
  raw.push_rune's call and its scalar value check, then the
  rt.rs_push_word path into obj.rs_push_word
- Go reference: strings.ToLower, ToUpper and EqualFold (go1.26.1), whose
  Map writes the mapped rune into its builder directly
- Test: corpora runes, strings, trimchars, equalfold and strwhere on
  arm64, LLVM and the Go backend; goldens unicode_table and
  native_eq_lower on arm64, LLVM and Go parity; the wasm case goldens;
  the full fixed point
- Timing: bench/results/unicode-push-2026-10-24.md

`unicode.case_append` encodes the changed rune where it stands, with
rt.rs_push_byte for one byte and rt.rs_push_word for two, three and
four, since the rune came from the decoder or a case table and is a
scalar value already. lower_u 0.870, upper_u 0.880 and eqlower 0.860 of
the base; runecase and the ASCII rows are controls and stay within
noise. raw.push_rune is unchanged for its other callers.

## 2026-10-24: perf: the Unicode tables are little endian, so u24 is one load

- Kind: perf
- Commit: this commit
- Found by: the mixed case mapping profile (u24, three bounds checked
  index reads per table value, was 34 percent of lower_u)
- Go reference: unicode and strconv's tables behind strings.ToLower,
  ToUpper, unicode.ToLower and ToUpper (go1.26.1); the tables are
  regenerated from them by internal/llvm TestUnicodeTables
- Test: corpora runes, strings, trimchars, equalfold and strwhere on
  arm64, LLVM and the Go backend; goldens unicode_table and
  native_eq_lower on arm64, LLVM and Go parity; TestUnicodeTables
- Timing: bench/results/unicode-u24-2026-10-24.md

`std/native/llrt/unicode_tables.xo` is generated 24 bit little endian
with eight zero bytes at each table's end, and `u24` is
`rt.bytes_word(t, i) & 0xffffff`: one load and a mask in place of three
checked reads and two shifts. lower_u 0.844, upper_u 0.843, runecase
0.759 and eqlower 0.878 of the base; the ASCII case rows are unchanged.
`unicode_classes.xo` (regexp) keeps its big endian layout and its own
reader.

## 2026-10-24: perf: Str.trim_chars trims a one byte cutset's two ends in one pass

- Kind: perf
- Commit: this commit
- Found by: the gap sweep after the mixed-text case work: re-measured
  against Go on today's main, trim was 1.765 of Go's instructions overall
  and 167 per call against 81
- Go reference: strings.Trim, TrimLeft, TrimRight (go1.26.1)
- Test: bench/perf/trimchars against its Go twin, `xo bench --ab`
  at one revision, A the base; corpora strings, bytes and unicode on
  arm64, LLVM and the Go backend; goldens trim_chars_view and
  trim_affix_lens; the full fixed point
- Timing: bench/results/trim-one-byte-2026-10-24.md

A one byte ASCII cutset took trim_chars to trim_end_byte and
trim_start_byte: two frames, two `s.bytes()`, two views. Both ends fit in
one pass over the bytes and one view, and that measures 0.917 of the
base on the trim row (69.4 M to 63.7 M instructions), with
trim_start_chars and trim_end_chars unchanged. The one byte path only:
inlining the ASCII paths too, or the loops of trim_start_chars and
trim_end_chars as well, made those two rows 5 to 8 percent worse through
the inliner's decisions, and was measured and reverted.

## 2026-10-24: perf: a short byte comparison inlines and is at most two loads a side

- Kind: perf
- Commit: this commit
- Found by: DS-2's view repro (`S.cut_prefix(P) ?? b""`, a two byte prefix), after the slice_check split
- Go reference: bytes.HasPrefix compares with a string compare (memequal), which loads, not loops, for short lengths
- Test: TestStdCorpora bytes, strings and strsearch (arm64, LLVM); goldens bytes_search_cut, cut_view and the view goldens

std/native/llrt scan.xo eq_at, which cut_prefix, cut_suffix, has_prefix
and the finds verify with, was 173 instructions with every length's path
in it, so the native inliner left it a call, and it compared 1 to 3
bytes one at a time with a bounds check each. It is now the short paths
only (4 to 7 bytes as two 4 byte loads, 2 or 3 as two 2 byte loads,
overlapping, 1 as one), inlined, so a constant length folds to one path;
8 bytes and more call eq_long. The repro: instructions 0.72 on top of
the slice_check split (0.80 from the loads, 0.90 more from inlining);
bytessearch cut_prefix and cut_suffix 0.96, index 1.009.

## 2026-10-24: perf: find tests the separator's second byte before the whole compare

- Kind: perf
- Commit: (filled in after the push)
- Found by: the profile of bench/perf/bytessearch's cut row. scan.find_bytes
  was 22 to 26% flat and up to 54% cumulative, and every candidate whose
  first byte matched went straight into an eq_at call
- Go reference: bytes.Index (go1.26.1) tests `s[i+1] == c1` before
  `Equal(s[i:i+n], sep)`, and takes `n == len(s)` as one Equal with no
  scan; ported exactly, with our find_byte's 16 byte word scan kept as the
  skip
- Test: the strsearch corpus (22 new shortHayCases edges: one and two byte
  separators, m == n, a match at the first and last start, a near miss on
  the second byte, non-ASCII bytes, haystacks of 64 and 65 bytes) on
  arm64, LLVM and the Go backend; bench outputs identical on all nine
  bytessearch and all eight strsplit workloads; the fixed point
- Timing: bench/results/strfind-2026-10-24.md

`find_bytes` keeps find_byte as its skip and adds Go's prefix test: a
candidate whose first byte matched now compares sub[1] with one load, and
that candidate is rejected without an eq_at call when the second byte
differs; m == 2 never calls eq_at, and m == n - f is one eq_at. Base over
change, xo bench --ab, 10 rounds, two binaries from the same revision with
only scan.xo differing, XO_INLINE_STRICT=1: cut 1.046 (71.85 M against
75.18 M, interval 1.016 to 1.054), strsplit split_multi 1.046 (1.042 to
1.049), split_after 1.008 (1.003 to 1.015), and the fourteen other rows
within noise, none worse.

Go's other half of that path, its brute force for len(s) at most 64
(bytealg.MaxBruteForce; IndexString compares in registers on arm64), was
ported first and rejected: a byte walk over the candidates measured 19.9%
more instructions on cut and 42.6% on split_absent, because find_byte's
word scan beats it. The numbers for both attempts are in
bench/results/strfind-2026-10-24.md.

## 2026-10-24: perf: to_lower and to_upper decode two byte runes inline (the mixed-text rows)

- Kind: perf
- Commit: (filled in after the push)
- Found by: the profile of lower_u and upper_u after the fused fold,
  where the mixed rows stood at 0.59 of Go's instructions
- Go reference: none (the semantics are unchanged: the same U+FFFD
  rules for a bad lead byte, a truncated sequence and a bad
  continuation byte as `utf8.DecodeRuneInString`)
- Test: corpus runes, strings and bytes on arm64, LLVM and the Go
  backend, the unicode, rune_classes and unicode_table goldens
- Timing: bench/results/case-mixed-2026-10-24.md

`unicode.case_append` re-enters only at a non-ASCII byte, since
`scan.span` returns the end of an ASCII run, so the two byte runes of
Latin, Greek and Cyrillic are decoded in the loop instead of through
`raw.decode`'s general path (its ASCII arm was dead there, and it paid
four range checks, a bounds check and a continuation loop per rune).
lower_u 4,596.8 M instructions to 4,206.8 M, upper_u 4,568.9 M to
4,167.1 M, eqlower 11,596.8 M to 10,826.7 M, arm64, best of 5, base
built from the same revision; the `--ab` gives wall 0.876 on lower_u,
0.900 on upper_u and 0.908 on eqlower (instructions 0.924, 0.923, 0.940,
every verdict better). Reading the below U+0800 case tables
through `raw.bytes_addr` instead of three checked index reads was tried
and measured 7.6 percent worse (the address is two runtime calls per
lookup) and reverted; a little endian table layout, one load and a mask,
is the next thing there.

## 2026-10-24: perf: a checked slice's range test inlines; its fault text is a cold call

- Kind: perf
- Commit: this commit
- Found by: DS-2's view retain/release repro (`S.cut_prefix(P) ?? b""` in a loop): every successful cut called prelude.slice_check, whose body built the fault text by interpolation and was too big to inline
- Go reference: Go's bounds checks are an inline compare and a call to runtime.panicSlice* only on failure
- Test: goldens str_slice_fault, cut_view, views_*, bytes_search_cut and the other view goldens (arm64, LLVM, Go parity); internal/stdtest bytes and strings corpora

std/native/prelude.xo slice_check (llrt_bytes_slice_at and the other
checked slices) is now the range test alone, 19 instructions the native
inliner takes, and calls slice_fail (in ssa.cold_fn) only for a bad
range. The repro: instructions 0.741, wall 0.862 (10 rounds); xo's own
__text grows 0.33% (59,454,848 to 59,649,016 bytes), the test inlined at
each checked slice.

## 2026-10-24: perf: to_lower and to_upper fold the ASCII spans they copy, in one pass

- Kind: perf
- Commit: (filled in after the push)
- Found by: the profile of bench/perf/unicode's case workloads: an ASCII
  text is a single copied span that is then folded in place, so every
  byte was read and written twice (obj.rs_append_bytes: move_at, then
  fold)
- Go reference: strings.ToLower and ToUpper map rune by rune into a
  Builder and never copy a span they then revisit; the runtime copies the
  unchanged spans and keeps that per-rune decode out of the ASCII case
- Test: corpus runes, strings, bytes and equalfold on arm64 and LLVM, the
  native_eq_lower and equal_fold_view goldens on both with Go parity, and
  the checksums of bench/perf/unicode identical on arm64, LLVM, wasm and
  Go (lower_az and upper_az are new workloads: a text of letters only,
  where every byte of every eight byte word folds)
- Timing: bench/results/casefold-copy-2026-10-24.md
- Follow-up on: case-fold-2026-10-24.md (whose fold is eight bytes at a
  time already; this change removes the separate copy pass around it)

`rs_append_bytes` now takes the folded path in one pass (`fold_copy`): it
loads the source word, folds it, and stores it to the destination, where
before it copied the span and then walked it again to fold. The same
append is used when the ranges could overlap (`overlaps`: `reserve` can
return the receiver's own buffer), where it still copies then folds.
Instructions retired, best of 5, arm64 native, both sides built with
XO_INLINE_STRICT=1: lower 392.8 M to 378.5 M (0.963), upper 402.5 M to
387.2 M (0.962), lower_az 107.3 M to 104.6 M (0.975), upper_az 107.4 M
to 104.5 M (0.973), lower_u 4,596.8 M to 4,554.8 M (0.991), upper_u
4,568.9 M to 4,515.0 M (0.988), eqlower 11,596.8 M to 11,522.3 M
(0.994); the eleven other workloads are within 0.1%. `xo bench --ab`, 12
interleaved rounds in the 2026-10-24 quiet window: wall 0.934 on lower
(19.96 to 18.65 ms), 0.938 on upper, 0.950 on lower_az, instructions
0.963, 0.961 and 0.973, every verdict better; cpu_ms too coarse. `between8` takes
its two range constants from the caller now; hoisting them out of the
loop measured no change (the compiler already did), and they are hoisted
anyway.

## 2026-10-24: perf: IndexAny and LastIndexAny build one ASCII set a call

- Kind: perf
- Commit: (filled in after the push)
- Found by: XO_PROF=cpu at 20,000,000 iterations on bench/perf/bytessearch
  put scan.ascii_set at 4.8% of find_any and 2.0% of rfind_any on top of
  scan.ascii_set_partial's 10.6% and 6.2%: a chars holding a rune above
  0x7f made index_any_bytes build Go's makeASCIISet, give up at that byte as
  Go does, and then build the partial set it actually uses from the start
- Go reference: bytes.IndexAny and bytes.LastIndexAny (go1.26.1): Go's
  makeASCIISet returns ok=false at the first byte above 0x7f and the slow
  path then ranges over the value, so Go builds one set a call
- Test: corpus/bytes and corpus/strsearch on arm64, LLVM and the Go backend,
  and golden bytes_search_cut on arm64, LLVM, Go parity and wasm, all
  unchanged in output; `xo bench --ab` of the bench program built against
  the base std and against this one, both sides with XO_INLINE_STRICT=1
  (find_any 116.74 to 113.75 M instructions, ratio 0.9745, verdict better;
  rfind_any 0.9865; contains_any 0.9740; the other six rows within noise,
  the base against itself 1.00002). The claim is instructions: wall is
  within noise for find_any (0.998, 0.950 to 1.062) and rfind_any (1.000,
  0.962 to 1.021), and contains_any's 0.974 (0.950 to 0.989) is no wider
  than the base-against-base control's own interval (0.948 to 1.024)
- Timing: bench/results/bytessearch-2026-10-24.md

`ascii_set_partial` now returns the two 64 bit words and whether every byte
was ASCII, so `index_any_bytes` and `last_index_any_bytes` build the set
once: the ASCII word path runs when chars is all ASCII and the value is
longer than 8 bytes, and the same words serve the rune loop below otherwise.
`ascii_set` stays for the trim family, which only needs the give-up.

Instructions per 200,000 calls (bench/results/bytessearch-2026-10-24.md):
find_any 116.74 to 113.75 M, rfind_any 147.25 to 145.30 M, contains_any
115.85 to 112.84 M; contains_rune, cut, cut_prefix, cut_suffix and
split_after do not build the set and do not move.

## 2026-10-24: perf: the empty-separator split sizes its piece list from the rune count, and its pieces are unchecked views (Str and Bytes)

- Kind: perf
- Commit: (filled in after the push)
- Found by: the split benchmark against Go's strings package
  (bench/perf/strsplit): the empty separator's list grew one push at a
  time to one piece per rune (`obj.rl_grow_n`, `alloc.resize`, 841.6 M
  instructions), and every piece paid the checked `raw.str_slice`
- Go reference: strings.genSplit's explode sizes its slice with
  `utf8.RuneCountInString`, capped by n, before the loop, and its pieces
  are plain string headers
- Test: corpus/strings and corpus/bytes on arm64, LLVM and the Go
  backend, the goldens split_view, views_split and bytes_split on arm64
  and LLVM with Go parity and on wasm, and the split and split_empty
  workloads of bench/perf/strsplit, whose checksums match the Go twin on
  native and LLVM
- Timing: bench/results/strsplit-vs-go-2026-10-24.md

`Str.split("")`, `Str.split_n("")`, `Bytes.split("")` and
`Bytes.split_n("")` size their list once from the rune count
(`scan.bytes_rune_count`, capped by n as Go does) instead of growing it,
and make each piece with `rt.str_view`, whose bounds are UTF-8 boundaries
by construction, instead of the checked `raw.str_slice`. Instructions
retired, best of 3, base = the parent commit's std, arm64 native:
split_empty 841.6 M to 327.2 M (0.39) and split, one of whose five
separators is "", 279.2 M to 176.2 M (0.63). split_one, split_multi,
split_absent, split_after, lines, bytes_split, bytes_fields, fields and
the control loop are within 1% of the base. `xo bench --ab`, 12 rounds in
a quiet window: split wall 0.741 (95% 0.713 to 0.766) and instructions
0.631, split_empty wall 0.526 (0.520 to 0.532) and instructions 0.389,
the three rows that do not take the empty-separator path within noise.

Counting the pieces first for the other forms was measured and reverted:
split_one 120.9 M to 129.7 M, fields 234.8 M to 353.4 M, bytes_fields
242.9 M to 362.3 M, split_after 158.5 M to 173.8 M, lines 111.6 M to
134.9 M, bytes_split 154.4 M to 169.4 M. A list's growth is geometric,
so it copies O(pieces) words in all, while counting is a second walk over
every byte (and, for fields, a second decode per field); only the empty
separator, where there is a piece per rune, wins.

## 2026-10-24: feature: Bytes gains the predicate forms (find_where, rfind_where, contains_where, the trim_where family, split_where, map_runes) and Str's share their walks

- Kind: feature
- Commit: (filled in after the push)
- Found by: Str had the predicate forms (strings.IndexFunc and its family,
  2026-10-23) and Bytes had none, so walking or mapping a Bytes value meant
  turning it into a Str first, either a validation pass that rejects invalid
  UTF-8 or a decode that changes the bytes
- Go reference: bytes.IndexFunc, LastIndexFunc, ContainsFunc, TrimFunc,
  TrimLeftFunc, TrimRightFunc, FieldsFunc and Map (go1.26.1); Go's walks call
  the predicate through a func value once per rune
- Test: corpus/bytes (Go's own tables plus 7,500 seeded texts, sets and
  records with invalid UTF-8) on arm64, LLVM and the Go backend, the new
  golden bytes_predicate on arm64, LLVM, Go parity and wasm, the habit cases
  go-bytes-index-func, go-bytes-trim-func and go-bytes-map, XO0911 for the
  trim and split views (cmd/xo vetcost_bytes_test.go), and
  bench/perf/bytespredicate against Go's bytes package

`Bytes` gets the eight predicate methods: `find_where` and `rfind_where`
(Option[Int], the byte offset of the first or last rune the predicate
accepts), `contains_where`, `trim_where`, `trim_start_where`,
`trim_end_where` (their results are views of the receiver on the native
targets, as `trim` is), `split_where` (the runs of runes the predicate
rejects; each piece is a view) and `map_runes` (the runes mapped, with a
`None` dropping the rune). An invalid byte decodes as U+FFFD of size one, as
Go's range over a string sees it, so a predicate can find and map it.

Str's forms and Bytes' share one walk each: `where_index`,
`where_last_index`, `where_trim_left`, `where_trim_right` and `where_spans`
in std/native/prelude.xo run over Bytes and return offsets, and each pair of
Str and Bytes methods makes its own type's result, so the two families cannot
drift. The Str forms were moved onto the shared walks without a change of
output (the strwhere corpus and the predicate goldens unchanged), and they
now call `raw.decode` directly instead of the checked `bytes_decode_rune`
wrapper, which the perf entry below shows is the same rune at fewer
instructions.

Instructions against Go's package, 200,000 calls, net of the loop
(bench/results/bytespredicate-2026-10-24.md, Xo / Go): find_where 0.26,
rfind_where 0.29, contains_where 0.35, trim_start_where 0.60, split_where
0.62, trim_where 0.65, map_runes 1.00, trim_end_where 1.13. Six of the eight
execute fewer instructions than Go's, because Go's walks call the predicate
through a func value and Xo's closure is direct; trim_end_where's backward
walk pays a second decode on a malformed tail, where Go reads at most four
bytes.
## 2026-10-24: perf: the set-search and rune loops skip ASCII bytes and decode through raw.decode

- Kind: perf
- Commit: (filled in after the push)
- Found by: the first Bytes benchmark put the new set searches at 1.5 to 5.2
  times Go's instructions net of the loop (bench/results/bytessearch-2026-10-24.md);
  XO_PROF=cpu at 20,000,000 iterations put contains_rune in raw.decode (31%
  flat), bytes_decode_rune (30% flat, 63% cumulative) and index_rune_bytes
  (20% flat), and find_any added find_short at 24% flat
- Go reference: bytes.IndexAny, LastIndexAny, ContainsAny and ContainsRune
  (go1.26.1); Go's IndexAny ranges over the value with DecodeRuneInString
  inlined, and its ASCII set covers the whole chars or none of it
- Test: corpus/bytes and corpus/strsearch on arm64, LLVM and the Go backend,
  the strwhere corpus, the goldens bytes_search_cut, bytes_split and
  bytes_indexed on arm64, LLVM, Go parity and wasm, and bench/perf/bytessearch
  against Go's bytes package, all unchanged in output; the Str forms were A/B
  checked with the same program shape against an xo built at the base commit
  (find_any 300.4 M to 117.4 M instructions, no row worse)

`scan.index_rune_bytes` reads a byte below 0x80 without decoding it (it is its
own rune), calls `raw.decode` directly, and no longer pays the wrapper's
bounds check and fault location per byte; `raw.decode` is Go's DecodeRune,
which Go inlines into its range loop. `index_any_bytes` and
`last_index_any_bytes` test a byte below 0x80 against the ASCII word set of
chars' ASCII bytes (`ascii_set_partial`, where makeASCIISet gives up when
chars holds a rune above 0x7f) and a byte at 0x80 and above against
`rune_in_set`, which decodes chars once a rune and compares runes as Go's
`for _, c := range chars` does, instead of an `index_rune_bytes` call that
searched chars for every rune of the value. An invalid byte still decodes as
U+FFFD, so a set holding U+FFFD still finds it.

Instructions against Go's package, 200,000 calls, net of the loop
(bench/results/bytessearch-2026-10-24.md, Xo / Go, before in brackets):
find_any 0.67 (2.06), rfind_any 0.77 (1.47), contains_any 0.66 (2.05),
contains_rune 1.69 (5.19); the cuts and split_after, which this does not
touch, are unchanged at 1.2 to 3.4. The Str forms share the cores and moved
with them: find_any 2.55 times fewer instructions, rfind_any 1.72,
contains_any 2.57, contains_rune 2.36, with the same output.

## 2026-10-24: feature: Bytes gains find_any, rfind_any, contains_any, contains_rune, split_after, split_after_n, cut, cut_prefix and cut_suffix

- Kind: feature
- Commit: (filled in after the push)
- Found by: the Bytes methods mirrored Str's one at a time; the set
  searches (bytes.IndexAny, LastIndexAny, ContainsAny, ContainsRune) and
  the cut family (bytes.Cut, CutPrefix, CutSuffix, SplitAfter, SplitAfterN)
  had no Bytes form, though Str had all nine
- Go reference: bytes.IndexAny, LastIndexAny, ContainsAny, ContainsRune,
  Cut, CutPrefix, CutSuffix, SplitAfter and SplitAfterN (go1.26.1)
- Test: internal/stdtest/testdata/corpus/bytes on arm64, LLVM and the Go
  backend (Go's indexAnyTests, lastIndexAnyTests, ContainsRuneTests,
  cutTests, cutPrefixTests, cutSuffixTests and splitaftertests, then seeded
  random bytes with invalid UTF-8); golden bytes_search_cut on arm64, LLVM,
  Go parity and wasm; bench/perf/bytessearch against Go's bytes package

Bytes gets the nine methods Str has had, with Go's semantics: `find_any`,
`rfind_any`, `contains_any` and `contains_rune` take a set of runes, so a
byte that starts an invalid sequence is U+FFFD and a set holding U+FFFD
finds it; `split_after` and `split_after_n` are SplitAfter and SplitAfterN
(an empty sep explodes, each piece keeps its separator); `cut`,
`cut_prefix` and `cut_suffix` return views of b through an Option, so a
piece kept alive keeps b alive, as the other slicing methods do.

The work is shared rather than copied: `scan.index_rune_bytes`,
`index_any_bytes`, `last_index_any_bytes`, `cut_bytes`, `cut_prefix_bytes`,
`cut_suffix_bytes`, `split_save_bytes` and `explode_bytes` hold the code,
and the Str forms (`scan.index_rune`, `index_any`, `last_index_any`) call
them. The two split loops stay one per piece type, since a Str piece and a
Bytes piece are different slices. On the Go backend the nine call Go's
bytes package directly.

Timing, beside Go's package (`bench/results/bytessearch-2026-10-24.md`, 200,000
calls, instructions net of the bench's own loop, Xo / Go): find_any 2.06,
contains_any 2.05, contains_rune 5.19, rfind_any 1.47, cut 1.72, cut_prefix
3.41, cut_suffix 2.82, split_after 1.16. The set searches leave Go's ASCII
set fast path when a set holds a rune above 0x7f and pay a decode per byte,
which is the first of the three follow-ups the results file lists.
## 2026-10-24: perf: the appended runs' case fold goes eight bytes at a time, and a replacement of up to eight bytes is a word store

- Kind: perf
- Commit: (filled in after the push)
- Found by: a CPU profile of a `to_lower`/`to_upper`/`replace` program
  (XO_PROF=cpu, 100 Hz, 128 samples) put 19.5% of the wall time in
  `obj.fold`, the per-byte ASCII case change of every run an append copies,
  and 46% under `cabi.obj_rs_append_bytes`, the general append replace paid
  for a replacement that usually fits a word
- Go reference: unicode.ToUpper and strings.Map walk runes with an inlined
  per-byte table test; Xo's fold loaded and stored every byte through
  intrinsics, and replace appended every replacement with the general path
- Test: corpus/strings, corpus/bytes, corpus/runes and corpus/equalfold on
  arm64, LLVM and the Go backend, goldens native_prelude, unicode_table,
  bytes_transforms and replace_n on arm64, LLVM with Go parity, and wasm,
  all unchanged in output
- Timing: bench/results/case-fold-2026-10-24.md

`obj.fold` now finds the letters of an eight byte word with a range test on
seven bit copies (`between8`) and changes them all in one store, adding
0x20 to 'A' to 'Z' or taking it from 'a' to 'z', which cannot carry into
the next byte; a byte at or above 0x80 never counts as a letter, so a run
that holds unchanged non-ASCII runes stays as it is, and the last n % 8
bytes keep the byte loop. `strings.replace_build` builds a replacement of 1
to 8 bytes as a word once and stores it with `rt.rs_push_word` at each
match. On the bytestransform bench, replace drops to 0.765 of its
instructions and lower and upper to 0.954 and 0.926, which puts replace
below Go (0.89 net of the harness loop, from 1.22). Skipping the fold for a
run with no letter of the case was measured too and reverted: 1.013 of the
instructions, because the runs it skips are a few bytes each.

## 2026-10-24: perf: Str.map_runes keeps unchanged runes as their own bytes and appends runs

- Kind: perf
- Commit: (filled in after the push)
- Found by: the str-func timing row put map_runes at 1.20 times Go's
  instructions on arm64 and 1.38 on LLVM; a CPU profile of the mapfunc
  workload (XO_PROF=cpu, 200 Hz) put half of the mapping time under
  raw.push_rune and its runtime calls (obj.rs_push_byte, obj.reserve),
  against a fifth in the caller's closures and a tenth in decoding
- Go reference: strings.Map writes byte by byte into one strings.Builder
  whose WriteByte is inlined; this one appended a rune at a time
- Test: corpus/strwhere on arm64, LLVM and the Go backend (Go's TestMap
  rows and 2,000 seeded randoms with invalid UTF-8) and the golden
  strwhere on arm64, LLVM, Go parity and wasm, all unchanged in output

Past the first rune that changes, a rune whose mapping is itself now keeps
its own bytes, and a run of them is flushed with one
`rt.rs_append_bytes`; a mapped result below 0x80 goes through
`rt.rs_push_byte` rather than `raw.push_rune`'s scalar checks. The
predicate is still called exactly once per rune, in order, and a
malformed byte that maps to U+FFFD still counts as a change and is
re-encoded, as Go's Map does.

`xo bench --ab --rounds=12` arm64, base vs new on the mapfunc binary:
instructions 0.714, wall 0.771, cpu 0.778 (loaded machine, 1 minute load
28 to 33). Against Go, same host and day: 0.86 of Go's instructions on
arm64 and 0.90 on LLVM, from 1.20 and 1.38
(bench/results/maprunes-batch-2026-10-24.md).

## 2026-10-24: perf: RFC 3339 parsing reads each field's digits without a loop

- Kind: perf
- Commit: (filled in after the push)
- Found by: callgrind of bench/perf/text time_parse, native against LLVM
  (linux/amd64): text.parse_time 68.5 M instructions against 47.9 M, the
  difference in num()'s loop of k digits (k 2 or 4 once inlined), which
  LLVM unrolls and the native backends' unroll_small does not (its body
  has the early return, more than one block)
- Go reference: none (time.Parse's RFC 3339 path checks the same fields)
- Test: internal/llvm/testdata/golden/time_parse_digits (each position
  with a byte just below and above '0' to '9', ranges, fractions,
  offsets) on arm64, LLVM and the Go backend; the old and new std print
  the same

`text.parse_time` (Time's JSON decoding and `llrt_time`) reads a two
digit field as two bytes checked together, `(c0 | c1 | (9 - c0) | (9 - c1))
< 0`, and the year as two of those, in place of a loop with a test per
digit. No output changes.

`xo bench --ab` arm64, base vs new: time_parse instructions 0.879, wall
0.919; dur_parse and time_fmt 1.000. Native against LLVM on time_parse:
1.22 to about 1.07.

## 2026-10-24: perf: replace sizes its buffer from the match count, equal_fold takes Go's ASCII loop, simple_fold buckets its table

- Kind: perf
- Commit: (filled in after the push)
- Found by: XO_PROF=cpu on bench/perf/bytestransform at 20,000,000
  iterations: `simple_fold` 25% flat (a 38-test compare chain for a sparse
  Int match), `cabi.obj_rs_append_bytes` 23% flat and `obj.reserve` 5% on
  replace
- Go reference: bytes.ReplaceAll and bytes.Count (Count, then one buffer of
  the exact size), bytes.EqualFold's ASCII fast loop (go1.26.1)
- Test: internal/stdtest/testdata/corpus/{bytes,strings,strsearch,regexp} on
  arm64, LLVM and the Go backend (Str and Bytes share these paths, and
  std/regexp's case insensitive matching walks simple_fold)

`Str.replace`, `Str.replace_n`, `Bytes.replace` and `Bytes.replace_n` count
the matches first and ask for the result's exact size, as Go's ReplaceAll
does, and skip the append call for an empty segment or an empty `new`.
`equal_fold_bytes` and `eq_lower` compare bytes for equality first and
return at the first difference, as Go's ASCII loop does. `simple_fold`'s 38
odd runes are split into five buckets, so a rune that is not in the table
costs two to five compares instead of a walk of the whole chain.

`xo bench --ab`, instructions, base vs new: replace 0.993, replace_n 0.958,
equal_fold 0.898, the untouched lower, upper and repeat 1.000. Against Go's
bytes package net of the bench's own loop: replace 1.22 (was 1.23),
replace_n 0.93 (was 1.00), equal_fold 0.98 (was 1.26). Replace's remainder
is two intrinsic calls per match through the C ABI, a compiler item
(bench/results/bytestransform-fold-2026-10-24.md).

## 2026-10-23: fix: Str.repeat faults below 0 on the native backends, as the Go backend does

- Kind: fix
- Commit: this commit
- Found by: bringing Bytes.repeat in line with Str.repeat (Bytes.repeat
  faults below 0 on both backends); native `raw.str_repeat` returned ""
  for any count below 0, where the Go backend faults, so the same program
  gave "" on arm64 and LLVM and exited 70 through the Go backend
- Go reference: strings.Repeat (go1.26.1) panics with "negative repeat
  count"; the Go backend already faulted
- Test: golden str_repeat_fault (arm64 and LLVM exit 70 with "fault:
  negative repeat count -1 at main.xo:9", Go parity), corpus/bytes
  (Bytes.repeat's rows), and no call site in compiler/, std/ or bench/
  passes a count that can be negative (the padding ones test the width
  first)

`Str.repeat` now faults with "negative repeat count N" at the caller's line
when the count is below 0, and with Go's messages for a result that would
overflow Int or exceed 4 GiB, on the native backends as on the Go backend
(which gains the last two checks, so a count that only the Go backend could
satisfy is refused the same way). `raw.str_repeat` is the copy alone; the
checks and their caller-relative faults are in std/native/prelude.xo
str_repeat, the arrangement Bytes.repeat already had.

## 2026-10-23: feature: Bytes.replace, replace_n, to_lower, to_upper, equal_fold and repeat

- Kind: feature
- Commit: this commit
- Found by: Bytes had find, rfind, contains, starts_with, ends_with, count,
  split, fields and the trims, so a port of Go code that rewrites, case
  maps or folds bytes fell back to Str, whose methods need valid UTF-8
- Go reference: bytes.ReplaceAll, Replace, ToLower, ToUpper, EqualFold and
  Repeat (go1.26.1), with Go's tables as the oracle
- Test: corpus/bytes on arm64, LLVM and the Go backend (Go's ReplaceTests,
  upperTests, lowerTests, EqualFoldTests and RepeatTests, then 1,500
  random replaces, 1,200 case mappings, 1,500 folds and 1,200 repeats,
  5,454 cases in all, with invalid UTF-8, empty values and views) and
  goldens bytes_transforms (arm64, LLVM, Go parity, wasm) and
  bytes_repeat_fault (arm64, LLVM, Go parity)
- Timing: bench/results/bytestransform-2026-10-23.md

`Bytes.replace` and `replace_n` are Go's ReplaceAll and Replace: an empty
old inserts new before every UTF-8 sequence and at the end, and `replace_n`
takes the count (-1 every copy, 0 none). Both return the receiver itself
when nothing changes, since a Bytes is immutable, where the Str forms copy
as Go does. `to_lower` and `to_upper` are ToLower and ToUpper; an invalid
byte becomes U+FFFD, which is Go's mapping of the replacement rune.
`equal_fold` is EqualFold's Unicode simple folding (`"straße"` is not
`"STRASSE"`, as in Go), and `repeat` is Repeat: empty for 0, a fault below
0 and above 4 GiB (the most a Bytes holds), as Go panics.

The work is shared rather than duplicated: `replace_build` in
std/native/llrt/strings.xo is one loop for the Str and Bytes forms
(building a Str the Bytes forms relabel), `replace` and `replace_n` keep
their exact pre-sizing through its `cap`; `scan.find_bytes` and
`scan.count_bytes` are `find` and `count` moved onto Bytes, with the Str
forms calling them, so Str's search has no second copy; and `map_case`
and `fold_equal` are one per family, shared by Str and Bytes. Str's
behavior is unchanged: the strings and strsearch corpora and the
strings golden are the check.

Habit: a Go program's `bytes.ReplaceAll`, `bytes.Replace`,
`bytes.ToLower`, `bytes.ToUpper`, `bytes.EqualFold` and `bytes.Repeat`
now get the method form and the fix (XO0601), like the other bytes calls.

## 2026-10-23: feature: Bytes.split, split_n, fields, trim, trim_prefix and trim_suffix

- Kind: feature
- Commit: this commit
- Found by: lead request; Bytes had find, rfind, contains, starts_with,
  ends_with and count, so a port of Go code that splits or trims bytes had
  to hand roll it
- Go reference: bytes.Split, SplitN, Fields, TrimSpace, TrimPrefix and
  TrimSuffix (go1.26.1), explode included (an empty separator, the byte
  length as its limit, and no piece at all for an empty value)
- Test: corpus/bytes on arm64, LLVM and the Go backend (Go's splittests,
  fieldstests, trimSpaceTests and the TrimPrefix and TrimSuffix cases of
  trimTests, then 4,800 seeded texts and records with invalid UTF-8, NUL
  inside pieces, 4,854 cases in all, plus a format then parse round trip)
  and golden bytes_split (arm64, LLVM, Go parity)
- Timing: bench/results/bytes-split-2026-10-23.md

`Bytes.split`, `split_n`, `fields`, `trim`, `trim_prefix` and
`trim_suffix` are Go's bytes package functions, as Str's split, fields
and trims are Go's strings ones. `trim` is TrimSpace, named as Str's is:
Str calls TrimSpace `trim` and left and right trims `trim_start` and
`trim_end`, so the bytes methods follow the same names rather than Go's
TrimSpace (owner: consistency with Str wins). Every piece and every trim
is a view of the value (decision 0330): 1000 `trim` calls on a 2 MiB
value take 7 us, so nothing is copied. `fields` and its bytes form share
one field scanner, so Str's fields keeps its single list.

The one Go behavior worth naming is explode's: with an empty separator,
Go splits into at most n pieces by *byte* length and leaves no piece at
all for an empty value, so `"".split("")` is `[]` while `"".split(",")`
is `[""]`. The corpus caught a first version that returned `[""]` for
both.
## 2026-10-23: feature: the predicate forms of Str (Go's strings *Func family)

- Kind: feature
- Commit: this commit
- Found by: lead request; Go's *Func family had no Xo form, so a port that
  asks a question about runes (IndexFunc, ContainsFunc, TrimFunc,
  FieldsFunc, Map) had to hand roll a decoder and a loop each time
- Go reference: strings.IndexFunc, LastIndexFunc, ContainsFunc, TrimFunc,
  TrimLeftFunc, TrimRightFunc, FieldsFunc and Map (go1.26.1)
- Test: corpus/strwhere (Go's indexFuncTests, trimFuncTests,
  FieldsFuncTests with fieldstests and the TestMap cases, plus 2000
  seeded random inputs each, plain and as a view of a longer buffer;
  arm64 and LLVM), golden strwhere (a closure, a named function, a bound
  method value, a failing closure on every method, views kept past their
  source function; arm64, LLVM, Go parity and wasm), and cmd/xo's
  TestVetCostViewStoreWhereTrim, TestVetCostViewStoreWherePiece and
  TestVetCostViewWhereUsedNoNote
- Timing: bench/results/str-func-2026-10-23.md (indexfunc, trimfunc,
  fieldsfunc and mapfunc against Go's four calls: 0.48 to 0.83 of Go's
  instructions on the first three, 1.20 to 1.38 on mapfunc)

`Str.find_where`, `rfind_where`, `contains_where`, `trim_where`,
`trim_start_where`, `trim_end_where`, `split_where` and `map_runes` are
the *Func family with effect polymorphism (`[E, fx: Effects]`, `! E uses
fx`), like List.find and Map.filter, so a predicate that fails propagates
and the walk stops there. The pieces keep the views of the string methods
(split_where and the trims return views of s on native targets) and
XO0911 names them (compiler/ssa/cost.xo view_call and param_view_at,
compiler/gen/own.xo fresh_std). A call of a prelude method with an effect
parameter carries its instantiation in the SSA name
(`prelude.str_split_where[()]`), where the other entries there are bare,
so both lists match through a `named` helper that also accepts a name
followed by `[`; without it the stored-view note never fired for
split_where (cmd/xo/vetcost_rune_where_test.go). `map_runes` takes
`fn(Rune) -> Rune?`:
None is Go's negative mapping, and like Go's Map it copies nothing until
the first rune that changes (a valid U+FFFD mapping to itself is not a
change, a malformed byte is). The output is built with `raw.push_rune`,
the llrt form: `rt.rs_push_rune` is lowered only for
StrBuilder.write_rune, so a hand written call to it is an undefined symbol
on native (gotchas/rt-call-not-lowered-link-error.md). f is called exactly
once per rune by every
method, including split_where (one pass records its spans, as Go's
FieldsFunc does, then the pieces are sliced from them).
`strings.IndexFunc`, `LastIndexFunc`, `ContainsFunc`, `TrimFunc`,
`TrimLeftFunc`, `TrimRightFunc`, `FieldsFunc` and `Map` in another
language get XO0601 with the Xo name (compiler/check/habits.xo).

A predicate is a closure (`fn(r) { r.is_space() }`), a named function or
a bound method value (`m.hits`); `Rune.is_space` is not a value
(XO0601, gotchas/rune-method-not-a-value.md), so the type name cannot
stand for its method.

## 2026-10-23: feature: the rest of the Rune classes, to_title and utf8_len

- Kind: feature
- Commit: this commit
- Found by: lead request; Rune had is_letter, is_digit, is_space and
  is_print only, so a port of Go code classifying punctuation, case or
  symbols had to hand roll it
- Go reference: unicode.IsUpper, IsLower, IsTitle, IsPunct, IsControl,
  IsGraphic, IsMark, IsSymbol and IsNumber, unicode.ToTitle (the
  CaseRanges TitleCase delta and the UpperLower sentinel), and
  utf8.RuneLen (go1.26.1)
- Test: corpus/runes (every Unicode scalar value: per plane counts of
  all 13 classes, the UTF-8 lengths summed and a hash of Go's ToLower,
  ToUpper and ToTitle; the boundaries of every table; and Rune.try
  rejecting surrogates and out of range values; arm64 and LLVM), golden
  unicode_table (arm64, LLVM, Go parity and wasm); timing row against
  Go: bench/results/rune-classes-2026-10-23.md

`Rune.is_upper`, `is_lower`, `is_title`, `is_punct`, `is_control`,
`is_graphic`, `is_mark`, `is_symbol`, `is_number`, `to_title` and
`utf8_len` complete the Rune surface, with the semantics of Go's unicode
package (is_print stays strconv.IsPrint, decision 0079). The tables come
from Go's unicode package through internal/llvm TestUnicodeTables, which
already generated the letter, digit, print and case tables; CASE now
carries Go's title delta as well (15 bytes a range) and TITLE2 the direct
title case of U+0080 to U+07FF. The module keeps decision 0180's shape,
ASCII tested directly, a bitmap for U+0080 to U+07FF and the merged range
search above, rather than Go's Latin-1 properties table, which covers
only U+0000 to U+00FF: one shape for all 13 classes, the same one byte
lookup at Latin-1 and a fast path past it. `utf8_len` is 1, 2, 3 or 4
bytes; a Rune is a Unicode scalar value (core 2.4, decision 0079), so
Go's -1 cannot happen, and `Rune.try(n)` is Go's ValidRune check on an
Int.

## 2026-10-23: feature: Bytes.contains, starts_with, ends_with and count

- Kind: feature
- Commit: this commit
- Found by: Bytes had find and rfind only, so the four questions bytes.Contains, HasPrefix, HasSuffix and Count answer needed a find and a hand comparison each
- Go reference: bytes.Contains, HasPrefix, HasSuffix and Count (go1.26.1)
- Test: corpus bytes (Go's containsTests, the TestCountByte and TestCountByteNoMatch windows, the documented boundaries, and 3000 seeded byte strings with invalid UTF-8: arm64, LLVM and Go), golden bytes_search (arm64, LLVM, Go parity and wasm)

`b.contains(sub)`, `b.starts_with(p)`, `b.ends_with(p)` and `b.count(sub)`
answer on the bytes as they are (no Str cast, so invalid UTF-8 is compared
byte for byte). Go's fast paths are ported: the count of an empty sub is
the rune count plus one; a one byte sub counts 16 bytes a step
(scan.count_byte, the scan equal mask and popcount); a search scans for
sub's first byte 16 bytes a step and compares each candidate, as Go's
Index does; starts_with and ends_with reuse scan.eq_at, 4 to 16 byte
affixes in two loads rather than a loop.

native arm64 against Go's package, 2000 buffers searched 200 times with
contains, starts_with, ends_with, count of one byte and count of three
bytes: buffers of 0 to 80 bytes 0.65 times Go's instructions (0.47 wall),
buffers of 0 to 400 bytes 0.63 (0.63).

## 2026-10-23: perf: the ASCII trims return their view without a slice check

- Kind: perf
- Commit: this commit
- Found by: DS-1's bench/perf/trimchars profile (bench/results/trimchars-2026-10-23.md: the checked slice 22% of trim)
- Go reference: strings.trimLeftByte, trimRightByte, trimLeftASCII, trimRightASCII (they slice unchecked by construction)
- Test: corpus/trimchars (arm64 and LLVM), golden trim_chars_view (arm64, LLVM, Go parity)

Str.trim_chars, trim_start_chars and trim_end_chars with an ASCII cutset
cut where a byte of the set meets a byte that is not, both ASCII or a
string end, so the cut is in range and on a UTF-8 boundary: the view is
made with no range or boundary check. Results are unchanged. arm64 xo
bench --ab bench/perf/trimchars: instructions trim 0.89, trim_end 0.926,
trim_start 0.942.

## 2026-10-23: feature: Str.trim_chars, trim_start_chars and trim_end_chars (strings.Trim, TrimLeft, TrimRight)

- Kind: feature
- Commit: this commit
- Found by: lead request; trim removed Unicode white space only, so a
  cutset of characters, as Go's Trim takes, had to be looped by hand
- Go reference: strings.Trim, TrimLeft and TrimRight with their
  trimLeftByte, trimRightByte, trimLeftASCII, trimRightASCII,
  trimLeftUnicode and trimRightUnicode fast paths, and makeASCIISet
  (go1.26.1)
- Test: corpus/trimchars (Go's trimTests rows for the three functions
  plus 3000 random texts and cutsets; arm64 and LLVM), golden
  trim_chars_view (arm64, LLVM, Go parity and wasm),
  TestVetCostViewStoreTrimChars

`Str.trim_chars(chars)` returns s without its leading and trailing runes
in chars, `trim_start_chars(chars)` its leading ones and
`trim_end_chars(chars)` its trailing ones; the names follow
find_any(chars:). Every result is a view of s, like trim's. Go's fast
paths are ported: an empty s or chars returns s, a one byte ASCII chars
trims that byte, an all-ASCII chars is a bitmap of two Int words (128
bits, no List allocation) tested a byte at a time, and any other chars
decodes runes of s, from the end with decode_last for trim_end_chars, and
looks each up in chars. They are declared in std/prelude.xo, implemented
in Xo in std/native/llrt/scan.xo, and the Go backend compiles the same
body (decision 0312). The corpus is Go's trimTests rows plus seeded
random texts that mix ASCII, multi-byte runes and bytes that are not
valid UTF-8 (read as U+FFFD, decision 0259) against empty, one byte,
ASCII and Unicode cutsets. The three calls join `view_call` in
compiler/ssa/cost.xo, so `xo vet --cost` reports a trimmed view stored
for longer as a view of its source (XO0911).

## 2026-10-23: feature: Str.replace_n, Go's Replace with a count

- Kind: feature
- Commit: this commit
- Found by: Str had replace only, which always replaces every match, so Go's Replace had no Xo form
- Go reference: strings.Replace (and its ReplaceAll path for n below 0 or above the count)
- Test: corpus strsearch REPLACE_N_CASES (Go's ReplaceTests with every n of them, plus 3000 seeded cases: arm64 and LLVM), golden replace_n (arm64, LLVM, Go parity and wasm)

`s.replace_n(old, new, n)` replaces the first n copies of old (below 0
for every one, 0 for none). Go's paths as they are: s itself, not a
copy, when old is new, when n is 0, and when there is no match; every
match goes through replace's ReplaceAll path; otherwise one allocation
sized from the count, and an empty old inserts at rune boundaries and at
the end. native arm64 against Go's strings.Replace, 2000 texts x 120
rounds with a counted and a full replacement each: 1.14 times Go's
instructions (1.20 wall); counted replacements only: 1.11 (1.11).

## 2026-10-23: perf: Float text reads powers of ten from a table

- Kind: perf
- Commit: this commit
- Found by: native vs LLVM comparison (json enc_floats 1.11 times LLVM's instructions)
- Go reference: none (Go's ftoa keeps its own tables)
- Test: TestNumDiff at 200000 cases (arm64 and LLVM against Go)

std/native/llrt/num.xo pow10, which the Float layout calls at each of
its sites, is one load from a table of 10^0 to 10^18 instead of five
tests and five checked multiplies. Output is unchanged. callgrind
linux/amd64: json enc_floats 0.964, enc_small 0.990.

## 2026-10-23: fix: Float32 of a UInt64 rounds once

- Kind: fix
- Commit: this commit
- Found by: Xo Lang 1, checking decision 0361's claims
- Go reference: the conversion float32(u) for a uint64 u
- Test: golden u64_to_float32 (258 values, arm64, LLVM and Go parity against Go's output; Docker amd64)

`Float32(u)` for a UInt64 u above 2^53 converted to a Float first and
then to Float32, rounding twice, on the native and LLVM backends:
2^60 + 2^36 + 1 became 2^60 instead of Go's 2^60 + 2^37. It now rounds
once (rt.u64_to_f32: arm64 `ucvtf` to single, x86_64 the halving
conversion in single precision, LLVM `uitofp` to float, wasm
`f32.convert_i64_u`), as the Go backend and Go always did. Printed
results change for such values; values up to 2^53 are unchanged.

## 2026-10-23: perf: a spawn site of tiny tasks keeps its burst on the spawner (decision 0359)

- Kind: perf
- Commit: this commit
- Found by: the Intel field report on 0.26.0 (spawn 100k tasks 1.25x Go) and the conc rwmutex gap
- Go reference: none (the scheduler)
- Test: golden spawn_sites (arm64, LLVM, Go parity); xo bench --ab on bench/perf/conc rows

The runtime keeps a running average of each spawn site's task run time
(256 slots of the pool record, a hash of the spawn location, a quarter
weight). A spawn from a site whose tasks average under 20 us no longer wakes
a worker: the burst stays on the spawner's ring, the spawner runs it at its
join, and the existing deferral (50 us on a later spawn, the monitor's 1 ms
tick) wakes helpers if the burst outlasts them. A site not seen yet, or with
long or blocking tasks, wakes a worker per spawn as before. No API change.
`xo bench --ab` against main (arm64, 20 rounds, load under 4): conc spawn wall 0.45,
fanout 0.59, pmap, pingpong, mutex and rwmutex within noise (linux/arm64 in Docker: spawn 0.77, fanout 0.63); CRUD HTTP rps and p99 within noise.

## 2026-10-23: feature: Str.split_n, split_after and split_after_n

- Kind: feature
- Commit: this commit
- Found by: lead request; Str.split had no count and no separator-keeping form
- Go reference: strings.SplitN, SplitAfter, SplitAfterN and their genSplit and
  explode (go1.26.1)
- Test: corpus/strsearch (Go's splittests and splitaftertests plus 3000 random
  (s, sep, n); SPLIT_N_CASES) on arm64, LLVM and go; golden split_view (arm64,
  LLVM, Go parity, wasm)

`split` was Go's `Split` only. `split_n` takes Go's count: below 0 every
piece (the same pieces `split` returns, through `split`'s own path), 0 none,
and n above 0 at most n pieces with the unsplit rest in the last.
`split_after` and `split_after_n` keep each separator in its piece, as Go's
`SplitAfter` and `SplitAfterN`. An empty sep is Go's `explode`: one piece per
UTF-8 sequence, the pieces after the first n-1 in the last. Pieces are views
of s, as `split`'s are, and the compiler's fresh-result list and its
parameter-view list name the three methods beside `split`
(compiler/gen/own.xo fresh_std, compiler/ssa/cost.xo param_view_at).


## 2026-10-23: feature: Str.find_any, rfind_any, contains_any, contains_rune

- Kind: feature
- Commit: this commit
- Found by: lead request; Str searched one needle (find, rfind, count), not a
  set of characters or a single Rune
- Go reference: strings.IndexAny, LastIndexAny, ContainsAny, ContainsRune and
  IndexRune (go1.26.1)
- Test: corpus/strsearch (Go's indexAnyTests, lastIndexAnyTests,
  ContainsAnyTests and ContainsRuneTests, plus seeded random texts, sets and
  runes; arm64 and LLVM), golden str_any_view (arm64, LLVM, Go parity and wasm)

`Str.find_any(chars)` and `Str.rfind_any(chars)` give the byte offset of the
first and last rune of `chars` in s, `None` where Go gives -1 (so also for an
empty `chars`); `contains_any(chars)` and `contains_rune(r)` are Go's
ContainsAny and ContainsRune. Declared in std/prelude.xo after `count`,
lowered to `str_*` in std/native/prelude.xo, and implemented in Xo in
std/native/llrt/scan.xo: `index_any` and `last_index_any` use Go's ASCII word
set (two 64 bit words, the bytes of s scanned directly) when `chars` is ASCII
and s is longer than 8 bytes, and decode s rune by rune otherwise, with
`index_rune` (a byte search below U+0080) and a `decode_last` that is Go's
DecodeLastRuneInString. The Go backend calls strings.IndexAny,
LastIndexAny, ContainsAny and ContainsRune in std/rt/str.go.

A `Str` is valid UTF-8, so one branch of Go's IndexAny cannot arise here:
`len(chars) == 1` with a byte of 0x80 or above (Go reads that byte as
U+FFFD), since a lone non-ASCII byte is never a valid Str. An invalid byte of
s reads as U+FFFD (decision 0259), so a U+FFFD needle or set member finds it,
as in Go.
## 2026-10-23: perf: Time.unix and unix_millis round down without a branch

- Kind: perf
- Commit: this commit
- Found by: native vs LLVM comparison (portable unix 1.10 times LLVM's instructions)
- Go reference: none (Go's time.Unix splits seconds and nanoseconds)
- Test: golden floor_div_time (arm64, LLVM, Go parity, Docker amd64)

std/native/llrt/ints.xo floor_div, which Time.unix and unix_millis call on
the native backends, divides `s ^ a` by b and flips the quotient back
with `s`, the sign of a, for a positive b: one division of a value that
is not negative, with no test of the sign or the remainder. Results are
unchanged. callgrind linux/amd64: portable unix 0.846.

## 2026-10-23: perf: Float text trims trailing zeros with Go's trimZeros

- Kind: perf
- Commit: this commit
- Found by: native vs LLVM comparison (json enc_floats 1.14 times LLVM's instructions)
- Go reference: internal/strconv/math.go trimZeros
- Test: TestNumDiff at 200000 cases (arm64 and LLVM against Go)

The shortest Float digits (std/native/llrt/num.xo dbox) drop their
trailing zeros as Go's trimZeros does: x times the inverse of 5^k modulo
2^64, rotated right k bits, is x / 10^k exactly when 10^k divides x, so
each step is one multiply, a rotate and an unsigned compare instead of a
remainder and a quotient by the power of ten, after one test that
returns at once when the digits end in no zero. Output is unchanged.
callgrind linux/amd64: json enc_floats 0.975, enc_small 0.997; arm64
xo bench --ab enc_floats within noise.
## 2026-10-23: feature: Str.cut, cut_prefix and cut_suffix (strings.Cut, CutPrefix, CutSuffix)

- Kind: feature
- Commit: this commit
- Found by: lead request; Str had find and rfind but no way to split at a separator without building a List, as Go's Cut does
- Go reference: strings.Cut, strings.CutPrefix, strings.CutSuffix
- Test: corpus/cut (arm64 and LLVM), golden cut_view (arm64, LLVM, Go parity and wasm)

`Str.cut(sep)` returns `Some((before, after))` for the first sep in s and
`None` when sep does not occur; `Str.cut_prefix(prefix)` returns the rest
of s after prefix or `None`, and `Str.cut_suffix(suffix)` returns s up to
its suffix or `None`. An empty sep, prefix or suffix gives the Go result:
`Some(("", s))` for cut and `Some(s)` for the other two. Option carries
Go's third `found` result, as `find_index` and `get_uvarint` already do
with an Option of a tuple. Every result is a view of s, like a trim or a
split piece, so a cut copies nothing. They are declared in
std/prelude.xo, implemented in Xo in std/native/llrt/scan.xo over `find`,
`starts_with` and `ends_with`, and the Go backend compiles the same body
(decision 0312). The corpus is Go's cut_test.go tables plus seeded random
cases on plain and view inputs. The cost check sees the views: the three
calls join `view_call` in compiler/ssa/cost.xo, and 86f78cd39's payload
walk carries a view through an Option, so `xo vet --cost` reports XO0911
for a `Str?` or `(Str, Str)?` field, for the call stored directly, and
for the shape `line.cut_prefix("x") ?? ""` stored in a field.
TestVetCostViewStoreCutOption in cmd/xo covers the last one. Two shapes
still get no note: a piece bound by a `Some((a, b))` match, and a tuple
from `??` whose field is stored (the stored value is a phi, not the
payload).

## 2026-10-23: feature: Str.equal_fold (strings.EqualFold)

- Kind: feature
- Commit: this commit
- Found by: lead request; Str had no Unicode case insensitive comparison (only eq_lower, which is a lowered equality, not simple folding)
- Go reference: strings.EqualFold, unicode.SimpleFold
- Test: corpus/equalfold (arm64 and LLVM), golden equal_fold_view (arm64, LLVM, Go parity and wasm), std/regexp's module tests and corpus

`Str.equal_fold(other)` reports whether s and other are equal under
Unicode simple case folding, like Go's `strings.EqualFold`, so `"k"` and
`"K"` and the Kelvin sign U+212A are equal, as are the three sigma forms,
while the Turkish dotted and dotless I fold only to themselves. It is
declared in std/prelude.xo, implemented in Xo in std/native/llrt/unicode.xo
(an ASCII prefix fast path, then rune by rune with a SimpleFold orbit
walk), and on the Go backend by strings.EqualFold in std/rt. The fold
table is the one std/regexp already had: `simple_fold` moved from
std/native/regexp.xo into std/native/llrt/unicode.xo and regexp calls it
there, with no change to its matching. The corpus adds Go's EqualFoldTests
and 3000 seeded random pairs drawn from the folding orbits, with a view
test; all backends match Go.

## 2026-10-23: feature: Str.count (strings.Count)

- Kind: feature
- Commit: this commit
- Found by: lead request; Str had find and rfind but no count
- Go reference: strings.Count
- Test: corpus/strsearch (arm64 and LLVM), golden count_view (arm64, LLVM and Go parity)

`Str.count(sub)` returns the number of non-overlapping copies of sub in s,
like Go's `strings.Count`, and for "" the count of s's runes plus one. It
is declared in std/prelude.xo, implemented in Xo in std/native/llrt/scan.xo
over the existing `find` (so a one byte sub takes the same byte scan as
find), and on the Go backend by strings.Count in std/rt. The corpus adds
Go's countTests, 3000 seeded random (text, sub) pairs on their own seed
(empty, one byte, multi byte and pieces of s, so overlapping copies
occur), and a view test; all backends match Go.

## 2026-10-23: perf: Bytes.from_base64 decodes four ordinary characters at once

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 1 (json bytes on amd64 native against LLVM: num.from_base64 176.4M against 147.7M instructions)
- Go reference: encoding/base64 decodeQuantum's fast path (go1.26.1)
- Test: internal/llvm TestNumDiff (Bytes.from_base64 against encoding/base64 on 200,000 texts, malformed ones included), goldens std06_helpers, method_values, json_value, native_json

At the start of each quantum the decoder takes the next four characters
together when all four are ordinary (no padding, no \r or \n, nothing
invalid: one test of their table values), and goes a character at a time
otherwise, as before. Output and errors are unchanged. json bytes: callgrind
on linux/amd64 420.3M to 395.6M instructions (0.941); `xo bench --ab` on
darwin/arm64, 30 rounds: wall 0.811 (0.80 to 0.842), instructions 0.944.

## 2026-10-23: perf: Str.lines, split, replace and UTF-8 checks scan with one bit a byte on amd64

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 1 (text workloads, native amd64 against LLVM: lines 1.234)
- Go reference: none (the runtime's byte scans)
- Test: scan_test.xo on arm64, LLVM and amd64; text bench outputs unchanged

The native runtime's 16 byte scans (std/native/llrt/scan.xo) use
rt.bytes_eq16_bits and rt.bytes_high16_bits on amd64: one bit a byte from
pmovmskb, where the nibble narrowing took eleven SSE instructions a step.
Other targets keep the nibble masks. callgrind on linux/amd64: text lines
0.905, utf8 0.964, replace 0.978, split 0.990, the other text workloads
1.0. Output is unchanged.

## 2026-10-23: feature: Str.fields (strings.Fields)

- Kind: feature
- Commit: this commit
- Found by: lead request; Str had split and lines but no fields
- Go reference: strings.Fields (unicode.IsSpace)
- Test: corpus/strsearch (arm64 and LLVM), golden fields_view (arm64, LLVM, Go parity, and wasm under Node)

`Str.fields()` returns the runs of non-white-space text, split around runs
of Unicode white space as `unicode.IsSpace` defines it, like Go's
`strings.Fields`. It is declared in std/prelude.xo, implemented in Xo in
std/native/llrt/strings.xo (ASCII bytes take a byte path; only a non-ASCII
byte decodes a rune), and on the Go backend by strings.Fields in std/rt.
Pieces are views of s on native targets, as split's are (decision 0330),
and the corpus and golden check a slice of a longer Str. The corpus adds
Go's fieldstests, 3000 seeded random texts over every Unicode space class
(U+0085, U+00A0, U+1680, U+2000 to U+200A, U+2028, U+2029, U+202F, U+205F,
U+3000) and a view test; all backends match Go. Wasm needs no special
path: the fields_view golden builds with --backend=wasm and prints the same
output under Node's WASI.

## 2026-10-23: test: Str find, rfind, contains, split, replace and List[Str].join against Go's strings

- Kind: test
- Commit: this commit
- Found by: lead request after the strings corpus of 2026-10-23 (the search and split methods had none)
- Go reference: strings.Index, LastIndex, Contains, Split, ReplaceAll, Join
- Test: corpus/strsearch on arm64 and LLVM (internal/stdtest/testdata/corpus/strsearch)

Corpus from Go's strings_test.go inputs (indexTests, lastIndexTests,
splittests with n below zero, ReplaceTests with n at -1), 5000 seeded random
search pairs, 3000 split pairs, 3000 replace triples and 1500 join lists,
each on a plain Str and, for the search, split and replace cases, on a view
of a longer text; join is also checked by putting every split back. Both
backends match Go; no std change was needed. Go's IndexAny, LastIndexAny,
Count and Fields tables are left out: no Str method exposes them, and
Str.replace is ReplaceAll (no n).

## 2026-10-23: behavior: FsErr, NetErr, IoErr, EnvErr, ProcErr and ClockErr print plain messages (decision 0358)

- Kind: behavior
- Commit: this commit
- Found by: the Intel field report on 0.26.0 (error texts nested Debug output)
- Go reference: os and net error texts (`open x: no such file or directory`, `i/o timeout`, `connection refused`)
- Test: goldens std_error_display, fs_native, fs_files, fs_ops_edges, net_only, proc_cmd and 12 others (arm64, LLVM, Go parity), test_net_only, test_proc_cmd, test_fakenet_raw, test_unix_kill

`Display` of these enums (and so `${e}` and an uncaught error's report)
printed the Debug form, `NotFound(path: "a.txt")`; it now prints a plain
sentence: `a.txt: no such file or directory`, `a.txt: file exists`,
`d: directory not empty`, `a.txt/x: not a directory`, `d: is a directory`,
`x: permission denied`, `PATH: MSG` for `FsErr.Io` (just MSG for an empty
path); `connection refused`, `i/o timeout`, `invalid address: MSG`, MSG for
`NetErr.Io` and `IoErr.Io` and `ProcErr.Io`, `denied by Net.only: ADDR`,
`denied by Proc.only: CMD`, `closed`, `canceled`; `NAME is not set`;
`CMD: executable file not found`. `Debug` is unchanged, and so is
comparing values. A program that matches the old text must change; the
repository's goldens and tests were updated.

## 2026-10-23: perf: List.sort sorts runs by insertion and merges between two buffers

- Kind: perf
- Commit: this commit
- Found by: the Intel field report on 0.26.0 (sort 1M ints 2.25x Go)
- Go reference: slices.Sort (pdqsort; Xo's sort stays stable, std.md 2.1, which pdqsort is not)
- Test: golden sort_merge (arm64, LLVM, Go parity); timing row bench/perf/sort against slices.Sort

`List.sort` was a bottom up merge sort that pushed every merged element
onto a new list through a `less` closure. It now sorts runs of 16 by
insertion, then merges between two buffers with `<` on the elements,
copying a pair of runs that is already in order. The result and the
stability are the same. `xo bench --ab --rounds=10` against the old sort
(arm64): 1M Ints wall 0.82, instructions 0.86, max RSS 0.58; 200 000 Strs
wall 0.92; 1M Floats wall 0.91. Against Go's slices.Sort (same programs in
bench/perf/sort/go): Ints 1.01 (was 1.23), Floats 1.05 (was 1.14), Strs
1.18 (was 1.29).

## 2026-10-23: perf: select arms on integer channels pass the word

- Kind: perf
- Commit: this commit
- Found by: decision 0353 phase 2 (select arms still boxed a one element List per message)
- Go reference: none (internal representation)
- Test: goldens chan_word, conc_channels, conc_select, select_arm_order (arm64, LLVM, Go parity), select_unbound and channels (gen)

A `select` send arm on a word channel (see the entry below) passes the
word through llrt_select_send_w, and a receive arm reads the word and the
closed flag through llrt_select_take_w, with no list allocated per message.
Behavior is unchanged. Two select loops ping-ponging over word channels
(arm64, `xo bench --ab --rounds=20`): wall 0.951 (0.925 to 0.975),
instructions 0.982, max RSS 0.974.

## 2026-10-23: perf: channels of integers send the word, not a list of one

- Kind: perf
- Commit: this commit
- Found by: XO_PROF=cpu of bench/perf/conc/xo/pingpong (alloc_pop 11.6% of samples: every send boxed its value in a one element List); decision 0353
- Go reference: none (internal representation)
- Test: golden chan_word (arm64, LLVM, Go parity), the channel and select goldens, a deterministic explore test on arm64 and LLVM, bench/perf/conc rows with `xo bench --ab`

A `Chan[T]` whose element is one word with no drop (an integer type, Float
as its bits, Bool, or an enum none of whose variants has a field) is a word channel on the native backends: `send`, `recv`,
`try_send`, `try_recv` and `for v in ch` pass the word through the native
prelude's llrt_chan_*_w, with no list allocated or freed per message, and
`recv` reports closed by a flag (a received 0 is a value). The list entry
points (select arms, generic code) convert at the runtime's boundary, so a
channel is never read in two representations. `select` behavior, closed
channel faults, cancellation and the deterministic scheduler's points are
unchanged. Other element types keep the list path. pingpong (10 rounds, arm64,
`xo bench --ab`): wall 0.835 (0.817 to 0.856), instructions 0.860; fanout,
mutex, spawn, rwmutex and pmap within noise.

## 2026-10-23: test: Str trim, trim_prefix/suffix, starts_with/ends_with against Go's strings

- Kind: test
- Commit: this commit
- Found by: lead request after the trim, eq_text and eq_at changes of 2026-10-23
- Go reference: strings.TrimSpace, TrimLeftFunc, TrimRightFunc, TrimPrefix, TrimSuffix, HasPrefix, HasSuffix
- Test: corpus/strings on arm64 and LLVM (internal/stdtest/testdata/corpus/strings)

Corpus from Go's strings_test.go inputs (trim, index and cut tables), affix
lengths 4 to 32 bytes, and 6000 random cases (Unicode spaces, invalid bytes,
affixes that are a prefix or suffix with and without a flipped bit), each run
on a plain Str and on a view of a longer text. Both backends match Go; no
std change was needed.

## 2026-10-23: perf: scan.eq_text inlines into the trim loop sites

- Kind: perf
- Commit: 24197a209
- Found by: bench/results/views-trim-2026-10-22.md (trim_affix still 1.5x Go's instructions: eq_text was a call per trim)
- Go reference: strings.TrimPrefix, strings.TrimSuffix (compare folded into the caller)
- Test: golden trim_affix_lens, views_split, str_ops_table, strings, text_helpers on arm64 and LLVM with Go parity; wasm goldens; std corpora

`scan.eq_text` was 120 instructions (its 8 byte word loop, `eq_words`, was
inlined into it), over the few sites rule's 100. It compares with 4 byte words
(whole words, then the overlapping last word) for 4 or more bytes, so the
helper `eq_words` is gone; at 93 instructions it is inlined at loop sites.
Output is byte-identical.

## 2026-10-23: perf: trim_prefix and trim_suffix inline in native code

- Kind: perf
- Commit: f7ccf46fe
- Found by: bench/results/views-trim-2026-10-22.md (trim_affix still 1.7x Go's instructions: the suffix trim was a call)
- Go reference: strings.TrimPrefix, strings.TrimSuffix (inlined at the call)
- Test: golden trim_affix_lens, views_split, str_ops_table, strings, text_helpers on arm64 and LLVM with Go parity; wasm goldens; std corpora

The native inliner took `str_trim_suffix` only at a site passing a closure: its
checked `s.len() - p.len()`, and the checked adds in `raw.data`, `eq_text` and
the cut helpers, gave the function landing pads. Those operations are
`rt.int_wrap` now (each is proved in range by its caller), `raw.str_from` and
`raw.str_upto` copy through `raw.str_copy` (no length check: the source is a
Str), and `eq_text` hands its 8 byte loop to `eq_words`. `str_trim_suffix` is
inlined at a loop site, and the prefix trim at its site. Results are
unchanged.

## 2026-10-22: perf: trim_prefix and trim_suffix compare words and cut once (native)

- Kind: perf
- Commit: 2593fbe3d
- Found by: bench/results/views-trim-2026-10-22.md (trim_prefix and trim_suffix ran 123.7 M instructions against Go's 50.5 M)
- Go reference: strings.TrimPrefix, strings.TrimSuffix (a length test and one memequal)
- Test: golden trim_affix_lens (every affix length and cut point, cuts inside a rune), views_split, str_ops_table, strings, text_helpers on arm64 and LLVM with Go parity; wasm goldens; std corpora

`scan.eq_text` compares the affix on the two texts' words (8 byte words and an
overlapping tail, 4, 2 and 1 byte loads for the short ones) with no Bytes
values and no bounds checks, for a caller that proved the range; `eq_at` takes
the same 4 to 7 byte compare. `raw.str_from` and `raw.str_upto` cut with one
boundary test, since the other end is the string's start or end. Results are
unchanged (md5 of the program output, before and after).

## 2026-10-22: perf: split and lines cut views without the second check (native)

- Kind: perf
- Commit: 128a10711
- Found by: bench/results/views-trim-2026-10-22.md (split 446.6 M instructions against Go's 403.8 M, lines 178.8 M against 223.9 M: every piece ran the clamp and the range check twice)
- Go reference: strings.Split, bytes.Split (subslices, no check)
- Test: goldens views_split, str_ops_table, strings, text_helpers and the http goldens on arm64 and LLVM with Go parity; std corpora

`scan.split` (all but the empty-separator loop, whose last sequence may be cut
short) and `scan.lines` call `raw.str_cut`: every end comes from a match or
from the string's own length, so the clamp and the second range check go. The
http1 helpers that trim or cut at a position found in the string (host
without brackets, the fragment cut in url_parts and url_error, trim_ws,
trim_ascii) use it too. `xo bench --ab`, mb/sliceb on the Mac: split wall
0.783, instructions 0.572 (446.6 M to 255.4 M, Go 403.8 M); lines wall 0.774,
instructions 0.652 (178.8 M to 116.6 M, Go 223.9 M).

## 2026-10-22: perf: trim, trim_start, trim_end, trim_prefix and trim_suffix cut a view without the second check (native)

- Kind: perf
- Commit: 2507d45c9
- Found by: bench/results/views-split-2026-10-22.md (trim 295.5 M instructions against Go's 201.2 M, trim_prefix/suffix 182.6 M against 50.5 M: the boundary test and the range check ran twice)
- Go reference: strings.TrimSpace, TrimPrefix, TrimSuffix (a subslice, no check)
- Test: goldens views_split, str_ops_table, strings, text_helpers on arm64 and LLVM with Go parity; std corpora

The five trims know their range is inside the string, so they call
`raw.str_cut` (std/native/llrt/raw.xo): no clamp, no `s[lo..hi]` range
check, one boundary test, then the declared `rt.str_view` (already
lowered inline by the arm64 and LLVM emitters since decision 0330 step
3, so the 0.25.0 seed builds it and no new seed is needed). A cut
inside a UTF-8 sequence still copies, as before. `xo bench --ab`, mb/sliceb on the Mac:
trim wall 0.835, instructions 0.710 (296.0 M to 210.3 M); trim_prefix and
trim_suffix wall 0.816, instructions 0.676 (182.9 M to 123.7 M).

## 2026-10-22: perf: List.insert and List.remove_at move elements with one memmove (native)

- Kind: perf
- Commit: d57e5aa76
- Found by: bench/perf/xo insrem, 80 ms native against 19 ms on the Go backend and 23 ms in Go (XO_PROF: all of it in the prelude's shift loops)
- Go reference: slices.Insert and slices.Delete (copy, one memmove)
- Test: golden list_insrem (every position, shared lists, Int, Bool, Str, a struct, an enum) on arm64 and LLVM against the Go backend's output, 0 leaks; fixed point

On arm64 and LLVM, `xs.insert(i, x)` and `xs.remove_at(i)` with an
index inside the list move the elements after it with one memmove
instead of an index and a set per element (compiler/ssa listmove.xo).
Inserting at the end, removing the last element, and out of range
indexes behave as before. `xo bench --ab`, bench/perf/xo insrem: wall
0.268 (73.4 to 19.7 ms), instructions 0.144, RSS unchanged. wasm keeps
the element loop.

## 2026-10-22: perf: a large Map copied and written forks instead of copying (native)

- Kind: perf
- Commit: 06a1f657c
- Found by: bench/perf/xo branch (`var c = m; c.put(k, v)` from one 50,000-entry Map): 63 ms native against 4.9 ms on the Go backend's log
- Go reference: none (Go's map has no value semantics; the Go backend's std/rt Map log is the model)
- Test: golden map_branch; TestMapBranch (the Map goldens without branching, XO_MAP_LOG=1); TestTSanSharedMaps; fixed point

On the native backends a Map or Set of 1024 or more entries that is
written while another value shares it no longer copies its lists: the
written copy becomes a fork that reads the shared lists in place
(decision 0291 branching addendum), and goes back to lists of its own
after 64 writes by its only holder. `xo bench --ab`, bench/perf/xo:
branch 0.071 of the wall time (65.6 to 4.7 ms) and 0.733 of the peak
RSS; copymut, get, update and iter unchanged; the HTTP CRUD server
within noise at 64 and 10,000 users. `XO_MAP_LOG=1` builds without it.
wasm does not branch.
## 2026-10-22: perf: Fs.cwd and the working directory on macOS take $PWD when it names it

- Kind: perf
- Commit: 532bd6838
- Found by: the Go backend gap ranking (bench/results/go-backend-gap-2026-10-22.md), fsproc cwd at 0.14x of the Go backend
- Go reference: os.Getwd (go1.26.1)
- Test: golden fs_cwd_path (a stale PWD is not used); bench/results/native-cwd-darwin-2026-10-22.md

On macOS libc's getcwd opens `.` and asks the kernel for its path (90% of
the profile of 20,000 `os.fs.cwd()` calls). The native working directory
now does what Go's os.Getwd does first: $PWD when it is absolute and
stat of it and of `.` name the same device and inode, else getcwd. A
stale $PWD (a different directory) is never used. bench/perf/fsproc cwd,
`xo bench --ab`: wall 0.158 of before (227 to 36 ms), instructions 0.156,
system time 0.21 s to 0.02 s; the Go backend 35 ms.

## 2026-10-22: perf: Mutex.with on the Go backend records a 2-frame site

- Kind: perf
- Commit: 47745250c
- Found by: go pprof of bench/perf/ctx mutex on the Go backend (bench/results/go-backend-gap-2026-10-22.md, item 2)
- Go reference: runtime.Callers
- Test: std/rt lock tests (-race); goldens conc_*, deadlock and mutex through TestGoldenGoParity

Each `Mutex.with` on the Go backend records where the lock was taken, for
the re-entrancy and deadlock reports. It walked 14 stack frames per call
(64% of the instructions of a lock heavy loop); the first frame, the Xo
caller the report names, is all it uses, so it now walks two.
bench/perf/ctx mutex, `xo bench --ab`: wall 0.333 of before (903 to 301
ms). The reports are unchanged.

## 2026-10-22: perf: Atomic on the Go backend is lock free

- Kind: perf
- Commit: 37c5a2ad4
- Found by: the Go backend gap ranking (bench/results/go-backend-gap-2026-10-22.md, item 3)
- Go reference: sync/atomic.Int64
- Test: std/rt atomic_test.go (-race); golden atomic_ops and conc_* through TestGoldenGoParity; bench/perf/atomic

The Go backend's `Atomic[Int]` and `Atomic[Bool]` are one `atomic.Int64`
(a Bool as 0 or 1) instead of a `sync.Mutex` and a `defer` per
operation; `add` and `max` are compare and swap loops (an overflowing
`add` still faults and leaves the value). `Task.Yield`, called by every
Atomic and Mutex operation, inlines its test of the deterministic
scheduler. bench/perf/atomic, instructions over setup: get -61%, set
-53%, swap -51%, cas -66%, flag -58%, add -38%, max -63%; `xo bench --ab`
wall time 0.51 (get) to 0.92 (add) of before.

## 2026-10-22: feature: Str.clone and Bytes.clone

- Kind: feature
- Commit: 44099405f
- Found by: decision 0330 item 3 (explicit copy of a view)
- Go reference: strings.Clone, bytes.Clone
- Test: golden views_clone (arm64, LLVM, Go); bench/results/views-retention-2026-10-22.md

`s.clone()` and `b.clone()` return a copy in a buffer of its own. A slice,
a `split` piece or a `trim` result is a view that keeps its whole source
alive on native targets; clone a piece that outlives a large source (2,000
16 byte tokens kept from 1 MiB bodies hold 2.1 GB as views, 4.9 MB cloned).
The Go backend returns the value itself (its strings share by design).

## 2026-10-22: perf: split, lines, trim and trim_prefix/suffix return views (decision 0330)

- Kind: perf
- Commit: d644daae2
- Found by: decision 0330's step list (hot ports), the std survey of 2026-10-22
- Go reference: strings.Split, TrimSpace, TrimPrefix (substrings share the string)
- Test: golden views_split (arm64, LLVM, Go); bench/results/views-split-2026-10-22.md

`Str.split`, `lines`, `trim`, `trim_start`, `trim_end`, `trim_prefix` and
`trim_suffix` (and the parsers built on `raw.str_slice`: http routing,
toml, regexp submatches) return views of their argument on native targets
instead of copies, when both ends of the piece are on UTF-8 boundaries (a
cut inside a sequence still copies). Native arm64, instructions: split
-13.4%, lines -13.9%, trim -10.9%, trim_prefix/suffix -10.6%. A piece
keeps the whole source alive, as in Go.

## 2026-10-22: perf: Str and Bytes slices are views (decision 0330)

- Kind: perf
- Commit: 436574f41
- Found by: bench/results/regexp-time-csv-vs-go-2026-10-21.md, decision 0330
- Go reference: Go's string and byte slices
- Test: goldens views_unique, views_eq (arm64, LLVM, Go)

`s[lo..hi]` on a `Str` or `Bytes` no longer allocates or copies on native
targets: the result shares the buffer (the second word of the value holds
the length in its low 32 bits and the offset in its high 32 bits). Every
value is limited to 4294967295 bytes; concatenating, repeating or appending
past it faults. A view of a large buffer keeps the buffer alive. wasm and
the Go backend are unchanged (they copy, or Go slices). Native arm64,
instructions: mime.parse_media_type 1,798 M to 1,338 M (-25.6%),
multipart.parse -16.1%; time Layout.format +15.8% (byte indexing pays the
offset and mask; step 4). bench/results/views-step3-measured-2026-10-22.md.

## 2026-10-22: perf: Map overwrites a key in place on the Go backend

- Kind: perf
- Commit: (filled in after the push)
- Found by: the Go backend's bench/perf profile (go-backend-map-profile-2026-10-22, the lead's notes)
- Go reference: none (Go's map writes in place; this is the persistent log's owner path)
- Test: std/rt TestMapOverwriteInPlace and the model tests; golden map_inplace_copies (go, arm64, LLVM)

On the Go backend a put of an existing key (and `update`) overwrites the
key's entry in place while no copy of the map shares its table (decision
0335), instead of appending and flattening the log every 2n+32 writes.
bench/perf putget 162 to 100 ms and update 122 to 73 ms (medians of 7,
interleaved, checksums equal); branch, copymut, get and iter unchanged.

## 2026-10-21: feature: Bytes.indexed

- Kind: feature
- Commit: (filled in after the push)
- Found by: the 2026-10-21 language poll (`for (i, c) in b.indexed()` was missing; List has it)
- Go reference: none
- Test: golden bytes_indexed (go, arm64, LLVM)

`b.indexed()` is `List[(Int, UInt8)]`: each byte with its offset, as
`List.indexed`.

## 2026-10-21: feature: List.group_by

- Kind: feature
- Commit: (filled in after the push)
- Found by: the 2026-10-21 language poll (a group-by loop written three times in std)
- Go reference: none (Go's slices has no group-by); Rust's itertools chunk_by is the nearest
- Test: golden list_group_by (go, arm64, LLVM)

`xs.group_by(key)` returns `List[(K, List[T])]`: the keys in first appearance
order, each group in input order (K: Hash; the key function may fail, as
`sort_by`'s). `sort_by` already is the sort by key.

## 2026-10-21: feature: a program's own Stdio, Clock, Rand, and Os on the native backends

- Kind: feature
- Commit: f05128eb5
- Found by: decision 0307's last step (owner, 2026-10-21)
- Go reference: none (Xo's capability interfaces; the Go backend already behaved so)
- Test: internal/llvm/testdata/golden/user_void_caps and os_literal (go, arm64, LLVM, wasm the same), cmd/xo testdata/nogc/userfs, TestUserRandShuffle

A type of the program's own converted to `Stdio`, `Clock`, or `Rand`, and
an `Os` built with a literal, were XO1201 on the native backends; they
now work as on the Go backend. `Rand.shuffle` on such a value is derived
from its `int` on every backend, as before on the Go backend. Every native
capability value's user field shrank from about two words per method of
the capability (an `Fs` was about 66 words) to two words.

## 2026-10-21: feature: Str.rfind, Bytes.find, rfind, and zeroed, Map.map, any, and all, read_text_or and read_or

- Kind: feature
- Commit: 6abd8dd2
- Found by: the agent poll of 2026-10-20 (approved by the owner, 2026-10-21)
- Go reference: strings.LastIndex, bytes.Index, bytes.LastIndex, make([]byte, n), errors.Is(err, fs.ErrNotExist) (go1.26.1)
- Test: internal/llvm/testdata/golden/prelude_additions (go, arm64, LLVM, and wasm the same)

`s.rfind(sub)` is Go's strings.LastIndex. `b.find(sub)` and `b.rfind(sub)`
are bytes.Index and bytes.LastIndex. `Bytes.zeroed(n)` is n zero bytes in
one allocation, where a loop of `write_byte(0)` was needed. `m.map(f)`,
`m.any(f)` and `m.all(f)` take the key and the value, run in insertion
order, stop at the first entry that decides, and return the first error
at once. `read_text_or(fs, path, default)` and
`read_or(fs, path, default)` give the default only when the file does
not exist. They are free functions rather than methods of Fs, so every
type that satisfies Fs still does.

## 2026-10-21: fix: Fs.read_dir and walk report errors instead of a shorter list

- Kind: fix
- Commit: bb85a141
- Found by: Xo Publish (a sweep for errors that become empty results)
- Go reference: os.File.ReadDir and Readdir (dir_unix.go: EINTR retried, an lstat error other than not-exist returned), filepath.WalkDir (go1.26.1)
- Test: internal/llvm/testdata/golden/fs_dir_errors (go, arm64, LLVM)

A directory read that failed partway (getdents64, __getdirentries64,
FindNextFileW) ended the listing and returned the names so far as
success, and an entry whose stat failed for any reason was left out of
`read_dir` (for `walk`, with its whole subtree), on every backend. Now
the read's error is returned (Darwin also retries EINTR on the open and
the read, and Windows checks for ERROR_NO_MORE_FILES), and only an
entry removed since the listing is skipped; any other stat error, such
as `Denied` in a directory without search permission, is the result.

## 2026-10-21: fix: Set.filter, map, any and all build natively

- Kind: fix
- Commit: defe5a7f
- Found by: Xo Lang 3 (log Map tests, decision 0291)
- Go reference: none
- Test: internal/llvm/testdata/golden/set_hofs

The four Set methods were specified (std.md 2.4) and ran on the Go
backend, but a native build stopped with XO1201 ("support is planned").
They now build on arm64, LLVM and wasm, with the Go backend's results;
`map` keeps each result's first appearance in iteration order.
## 2026-10-20: behavior: a connection's reads and writes take the effect of how it was made

- Kind: behavior
- Commit: a48437ba
- Found by: survey
- Go reference: none
- Test: test/errorcheck/connection_effects; internal/llvm/testdata/golden/net_echo

`Listener.accept` returns a `ServerConn`, whose reads and writes are
`net.listen`; a `Conn` from `Net.dial` reads and writes under `net.dial`
(they were `net`). The prelude interface `Stream` takes either. Code that
named `Conn` for an accepted connection now names `ServerConn` or
`Stream` (decision 0280).
## 2026-10-20: port: Go's math, bit for bit on every backend

- Kind: port
- Commit: c41388d3
- Found by: survey
- Go reference: math (exp.go, log.go, pow.go, sin.go, tan.go, trig_reduce.go, ...), math/bits (go1.26.1)
- Test: std/native/llrt/gomath_test.xo (Go's results on 21,465 special and random cases); internal/llvm/testdata/golden/float_no_fma

Float gains pow, exp, exp2, expm1, log, log2, log10, log1p, the
trigonometric and hyperbolic functions, cbrt, hypot, mod, is_inf,
to_bits, and Float.from_bits; Int gains leading_zeros, trailing_zeros,
ones_count (decision 0287). The results are Go's portable algorithms
with every operation rounded, the same bits on every backend and
architecture.

## 2026-10-20: fix: the Go backend no longer fuses Float multiply-adds

- Kind: fix
- Commit: ecfabea4
- Found by: test
- Go reference: the Go spec, Arithmetic operators (fused multiply-add)
- Test: internal/llvm/testdata/golden/float_no_fma

On arm64 Go's compiler fused `a * b + c` in the Go backend's output into
one FMA, so a Float computation could differ from the native backends in
the last bit; a product now prints as `float64(a * b)`.

## 2026-10-20: port: Int.parse_base, Int.to_str_base, Bool.parse, Str.unquote

- Kind: port
- Commit: 0f3ae3d0
- Found by: survey
- Go reference: strconv ParseInt, FormatInt, ParseBool, Unquote (go1.26.1)
- Test: std/native/prelude_test.xo (Go's results on 2,813 random and quote_test.go cases)

Go's strconv parsing and formatting in other bases, booleans, and
unquoting, None where Go returns an error (decision 0287).

## 2026-10-23: perf: Str.starts_with and Str.ends_with compare 4 to 16 byte affixes without a loop

- Kind: perf
- Commit: this commit
- Found by: profile
- Go reference: none (bytes.HasPrefix compares with memequal)
- Test: std/native/prelude_test.xo (every length 0 to 40, a mismatch at each position)

scan.eq_at compares 8 to 16 bytes as two overlapping words and 4 to 7 bytes
as two overlapping 4 byte loads, so a short affix such as "Bearer " takes no
loop. xo bench --ab (10 rounds): instructions 0.939 on starts_with and
ends_with of four literal affixes over a header list.

## 2026-10-25: perf: Str.to_lower and Str.to_upper fold 64 ASCII bytes a step through a lane intrinsic

- Kind: perf
- Commit: this commit
- Found by: profile (bench/results/case-swar-2026-10-25.md)
- Go reference: none (Go folds a byte at a time, strings.Map; its ToLower/ToUpper take the unicode path)
- Test: scripts/stdcorpus/case.go (the lane's boundaries and seeded mixed case texts), bench/perf/unicode
- Timing: bench/results/case-fold16-2026-10-25.md

The ASCII runs of a case conversion are folded by `rt.case_fold16`, one 16
byte lane an instruction sequence (arm64 nine instructions of NEON, x86-64
SSE2, LLVM a `<16 x i8>` sequence, wasm a byte loop), four lanes a step
before the eight byte SWAR loop that stays for the tail. arm64 lower and
upper go from 1.755 and 1.696 of LLVM's instructions to 0.644 of main's
(0.884 and 0.879 in wall); LLVM's own rows improve to 0.965 and 0.932
because its expansion stopped being SWAR words clang no longer widened.
The two mixed rows cost 0.5 to 0.6 percent of instructions with every wall
within noise, which the lead accepted on purpose (decision 0373).

## 2026-10-30: fix: Net.lookup_host reports a name with no address as `no such host`

- Kind: fix
- Commit: this commit
- Found by: the net_lookup_dial_timeout golden on the linux/arm64 offload host (tiers 28 and 29)
- Go reference: net/cgo_unix.go (EAI_NONAME and EAI_NODATA are both errNoSuchHost), the pure Go resolver's errNoSuchHost for a name with no records
- Test: internal/llvm/testdata/golden/net_lookup_dial_timeout on the linux/arm64 host

A lookup the resolver answered with EAI_NODATA (the name exists, or its DNS
server says so, with no address: a Docker resolver answers `.invalid` names
this way) gave `lookup NAME: No address associated with hostname`, the
resolver's own text, where Go says `lookup NAME: no such host`. Both codes
are `no such host` now, as in Go.
