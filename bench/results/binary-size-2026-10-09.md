# Go backend binary size, build cache growth, and compile memory (2026-10-09)

darwin/arm64 (M4 Pro), Go 1.26.1. Before: main at ae2f84c. After: the
four commits of this change (decision 0095, gotcha
`go-generic-method-sets-per-package.md`).

## Method

`bench/size/measure.sh` (`XO=<xo binary> bench/size/measure.sh <dirs>`,
`MODE=test` for `xo test`): a fresh `XO_GOCACHE` and `XO_BUILD_DIR`, two throwaway
programs built first so the runtime and std packages are cached (one with
std/http, one without, since the runtime now has two variants), then
`/usr/bin/time -l xo build -o out <dir>` once per program. Cache growth
is `du -sk` of the cache before and after. "Build RSS" is the largest
child process (`maximum resident set size` of `/usr/bin/time`); the per
package compile RSS comes from a `-toolexec` wrapper that runs each
compile and link under `/usr/bin/time -l` (fresh cache, so rt compiles
too). Two runs each; they agreed to the byte and within 0.1 s.

Programs: hello world (`fn main(os: Os) { os.stdio.println("hello") }`),
`examples/notes-service` (HTTP service, std/http and std/json), golden
`json_codec` (JSON heavy, no std/http), and `xo test` of notes-service.

## Results

| Program | Binary before | after | Cache growth before | after | Build RSS before | after | Warm build before | after |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| hello world | 7.92 MB | 3.61 MB | 34.3 MB | 2.9 MB | 281 MB | 124 MB | 1.15 s | 0.24 s |
| notes-service | 12.78 MB | 7.69 MB | 53.6 MB | 6.9 MB | 526 MB | 177 MB | 1.87 s | 0.48 s |
| json_codec | 9.03 MB | 4.07 MB | 43.6 MB | 4.9 MB | 370 MB | 148 MB | 1.46 s | 0.34 s |
| notes-service `xo test` | - | - | 68.0 MB | 8.4 MB | 666 MB | 213 MB | 2.62 s | 0.91 s |

Compile RSS per package (fresh cache):

| Package | before | after |
|---|---:|---:|
| rt (once per cache and variant) | 766 MB, 1.54 s | 306 MB, 0.62 s |
| hello world's package | 286 MB, 0.43 s | 123 MB, 0.10 s |
| hello world's package main | 285 MB, 0.44 s | 20 MB |
| hello world link | 204 MB | 86 MB |
| notes-service's package | 555 MB, 0.96 s | 182 MB, 0.26 s |
| notes-service link | 256 MB | 123 MB |

Targets: cache growth at or below 5 MB: met for hello world (2.9 MB) and
json_codec (4.9 MB); notes-service, a larger program, is 6.9 MB. Compile
RSS at least halved: met (286 to 123 MB, 555 to 182 MB, rt 766 to 306 MB).
Hello world at or below 3 MB: not met, 3.61 MB (see What is left).
For reference: plain Go `fmt.Println` is 2.48 MB, 1.64 MB with `-s -w`;
native arm64 302 KB; LLVM 86 KB.

Binary size by step (hello world): 7.92 MB; protocol only generics 7.17
MB; no DWARF or symbol table 4.91 MB; no net/http 3.61 MB.

Workload times (`bench/perf/run.sh`, best of 5, Xo column): unchanged
within noise (putget 132.8 / 131.8 ms, sortint 152.3 / 148.3 ms, maxby
51.9 / 51.8 ms, push 7.3 / 6.8 ms). The helpers that became functions
inline as the methods did (`go build -gcflags=-m=2`: `List_PutM`,
`List_PtrM`, `List_PushFast`, `List_AtM`, `List_AtU`, `List_RefM`,
`List_Mine`, `List_Unshared`, `List_Items`, `List_len` inline into
callers).

## Where the 17 MB per package came from

Hello world's package before: 17.56 MB archive, 41,053 symbols, 4,435
functions, 0.9 MB of text. Go object blocks: relocations 5.27 MB, data
(DWARF, pcdata, type data) 4.68 MB, symbol name strings 2.82 MB, content
hashed symbol definitions and hashes 1.79 MB, aux 0.77 MB, symbol
definitions 0.40 MB, export data 0.31 MB. Without DWARF it was 11.8 MB
(relocations 2.0 MB, data 3.0 MB): DWARF was a third, the rest scaled
with the symbol count.

Symbols by origin (count, bytes): type descriptors of generic
instantiations 8,163 (673 KB), DWARF 1,084, then method families across
all instantiations: XoDebug 861, XoGenAny 744, XoEq 701, XoHash 686,
XoJsonEnc 576, All 542, or_fault 376, XoCmp 370, XoJsonDec 319, XoMark
288, Pairs 259, is_none/is_some 244 each, len 186, contains 183, ...
Protocol methods (Xo*) were 4,707 symbols, other methods 7,527. Generic
functions instantiated per shape (17 shapes each): JsonDec (51 KB), Gen
(42 KB), JsonRead, Eq, JsonEncP, Cmp, JsonEnc, Hash, MarkP. By receiver:
Map 907 functions, Option 884, List 764, Set 485, Result 473, Tuple2 180.

The 82 instantiations (26 Option, 19 Result, 6 each Map, Tuple2, mentry,
mtable, 5 each List and Set, Chan, its queue entries) were all over the
runtime's own types: `Result[Option[FileInfo], FsErr]` from `Fs.stat`,
`Option[FsErr]` from `Result.err()`, `Set[FileInfo]` and
`Map[FileInfo, Unit]` from `List.to_set()`, `Tuple2[string, string]` from
`Map.Pairs`, `List[*route]` and `Option[*route]` from the router.

Why every package had them (probes: a package with `var X T; func F(x T)`
for one T):

| T | archive | symbols |
|---|---:|---:|
| `rt.Unit` | 17.5 KB | 12 |
| a call of `rt.EnableArenas`, `rt.SetBuildInfo`, `rt.GCConfig` | 19.6 KB | 22 |
| `*rt.Task` | 12.5 MB | 29,299 |
| `rt.Option[int64]` | 12.5 MB | 29,299 |
| `rt.Os` | 17.5 MB | 41,052 |

Go's compiler reads, for every type a package mentions, its fields and
all its method signatures, transitively, and compiles all methods of every
generic instantiation it reaches in that package (the `importedDef` TODO
in cmd/compile/internal/noder/reader.go: instantiations from an imported
package are never reused). `Option[int64]`'s methods take `*Task`, and
Task's fields and methods reach the whole runtime API, so any package
paid for everything. Not the cause: reflection, init time tables, or the
generated code referencing everything (the generated code was 16 lines);
package main paid the same as the module's package because its closure
named `rt.Task` and `rt.Os`.

Removing one method family from every generic type (probe `rt.Os`,
17.52 MB): XoGenAny 16.44 MB, XoEq 16.71, XoHash 16.78, XoCmp 17.06, the
three JSON methods 15.04, the fx codec 17.12. Turning all 101 non-protocol
methods into functions: 4.63 MB (3.03 MB without DWARF); keeping the 26
helpers the generator calls as methods: 5.92 MB. Hence the full
conversion.

After: hello world's package is 2.88 MB, 7,747 symbols, 892 functions,
35 instantiations (20 Result, 5 Option, 4 List, one Map); package main is
13 KB. What remains per package is the protocol methods of the
instantiations rt's API names (XoGenAny 271 symbols, XoDebug 242, XoEq
222, XoHash 205, XoMark 147, XoJsonEnc 92) and their type descriptors
(1,796).

## Binary composition after (hello world, 3.61 MB)

Text 1.51 MB: Go runtime 479 KB, rt 240 KB plus about 60 KB of its
generics, encoding/json 79 KB, time 64 KB, slices 60 KB, reflect 56 KB,
runtime/pprof 51 KB, fmt 36 KB, compress/flate 22 KB, crypto fips
packages about 50 KB (package inits of crypto/sha256, hmac, pbkdf2), net
and netip about 30 KB. The pclntab is about as large as the text.

What pulls them (linker `-dumpdep`, shortest paths):
- runtime/pprof, compress/flate, text/tabwriter: the `XO_CPUPROFILE`,
  `XO_MEMPROFILE`, `XO_BLOCKPROFILE` hooks of RunMain, and the deadlock
  report's goroutine labels (`labelThread`) and goroutine profile
  (`lazyStacks`, `pprof.Lookup("goroutine").WriteTo(_, 1)`, which reaches
  the proto and gzip writers too). Experiments: dropping only the profile
  hooks saves 50 KB, only the labels 34 KB, both 338 KB (3.27 MB).
- runtime/trace: `XO_TRACE` (part of the 50 KB above).
- encoding/json: std/json's parser (`jsonParse`, `json.Valid`), replay
  loading, crash bundles, log records, the fx codec.
- crypto: package inits of crypto.go's imports.

## What is left, ranked by expected effect

1. Deadlock report without runtime/pprof (about 300 KB with item 2):
   record each thread's goroutine id when it first blocks (parse the
   header of `runtime.Stack(buf[:64], false)` in `labelThread`, a map from
   goroutine id to thread id), and read `runtime.Stack(buf, true)` in
   `lazyStacks` instead of the labeled goroutine profile; inherited labels
   are not needed (`clearLabels` becomes a no op). Measure first:
   `labelThread` cost per request under `bench/perf/http/run.sh` with a
   contended mutex, against today's `SetGoroutineLabels`. Tests:
   `TestGoldenNoMono/deadlock_real`, `go_callback_deadlock_real`,
   `std/rt` gotimers tests.
2. Profiling hooks behind a build tag `xo_profile` (needs a user facing
   switch, for example `XO_PROFILE=1 xo build`, documented in
   toolchain.md where `XO_CPUPROFILE` is): with item 1, hello world
   about 3.27 MB.
3. encoding/json out of the runtime's own paths (replay files, crash
   bundles, log records: hand written encoders like `appendJSONString`),
   and std/json's tokenizer replaced by `JsonR` (jsonfast.go) for the
   tree decode: about 150 to 250 KB. Then hello world should be under
   3 MB.
4. Per package cache growth: the remaining protocol methods. `XoGenAny`
   is only for property tests and is the largest family (271 symbols in
   hello world's package): generate values by reflection over the
   container types (`reflect.New`, `ListOf` through a non-generic
   constructor) or register generators per type in test binaries only.
   Then `XoMark` (arenas), `XoCmp`. Probe as done here: copy a build
   directory, delete the method family from rt's generic types, and
   compare the archive of a package with `var X rt.Os` (`go list -export
   -f '{{.Export}}'`, `go tool nm`).
5. Fewer instantiations in rt's API: 20 `Result` instantiations remain
   (one per capability method shape); a shared `Result[Unit, IoErr]`
   family cannot shrink further without changing the std API.
