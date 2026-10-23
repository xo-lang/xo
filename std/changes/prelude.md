# prelude changes

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
