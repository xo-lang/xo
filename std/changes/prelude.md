# prelude changes

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
