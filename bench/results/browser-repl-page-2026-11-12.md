# The /play/ REPL tab's memory: the 30-entry session on the page (2026-11-12)

Decision 0583's owner bar: the REPL's compiler under 50 MB at peak. Measured by Xo Lang 2 for the
lead, on the Mac Studio (darwin/arm64), headless Chrome 157.

## Method

`page-session.mjs` (in the lead's notes, ~/xo-lead/notes/xo-lang-2/repl30/; no npm) serves a built
site's `public/` on a local port, opens `/play/` in headless Chrome over the DevTools protocol,
accepts the download notice, opens the repl tab, and types the 30 entries of the 0583 session
(`repl30/session.txt`, the session of bench/results/browser-repl-2026-11-09.md step 3) one at a
time into the tab's input, submitting each and waiting for the tab's entry count. After each entry it
records:

- **compiler memory**: the page's `data-repl-mem`, the REPL compiler instance's wasm memory in MB
  (toolchain.js: `memory.buffer.byteLength`). Wasm memory never shrinks, so the last entry's value is
  the session's peak. This is the 50 MB bar's measure (decision 0583, "The playground page").
- **entry time**: `data-repl-ms`, from submit to the reply (with the run in the runner worker).
- **renderer RSS**: the sum of `ps` RSS over the profile's Chrome renderer processes (the page, the
  toolchain and runner workers, V8's code). Not the bar; reported for scale.

Every entry's value matched the terminal REPL's (the log tails in the JSON files).

Pages measured:

- **0.49.1** (live on xolang.dev since 2026-11-12): site 96b3ea85a, assets built from public
  ddd37586d (v0.49.1).
- **main**: site 96b3ea85a with assets rebuilt by `scripts/build-playground.py` from main 955a6c8a7
  (compiler 16,951,526 bytes, 2,243,342 brotli; first load 2,916,540 brotli).

## Result

| | 0.49.1 | main 955a6c8a7 |
|---|---|---|
| compiler memory, peak | **49 MB** | **49 MB** |
| after entry 1 (`use std/adler32`, std's record loaded) | 35 MB | 35 MB |
| first entry | 414 ms | 401 ms |
| later entries, median | 29 ms | 29 ms |
| renderer RSS before entry 1 / peak | 516 / 1,523 MB | 514 / 1,360 MB |

Under the 50 MB bar, by 1 MB. The lead's 87 MB (2026-11-10) was decision 0583's step (c) harness
figure ("record and three-quarter classes"); steps (d) to (f) on 2026-11-11 brought the harness to
49 MB, and the page measured the same (0583, "The playground page", 42160d539). The memory grows in
four steps: 35 MB (entry 1, std's record decoded and installed), 41 (entry 2, the first check and
build), 44 (entry 4, `xs.sort()`, the first entry that lowers a std body), 49 (entry 15, the first
method declaration), then flat to entry 30.

## What the REPL adds to the tab (renderer RSS)

The same page (main 955a6c8a7's assets), three runs of each, headless Chrome on the Mac:

- **REPL tab**: consent (the default program is checked), the repl tab opened, the 30 entries
  (each built entry runs in the page's runner worker). Renderer RSS 514 to 522 MB before the first
  entry, peak 1,490, 1,492 and 1,499 MB (a fourth, earlier run: 1,360 MB).
- **playground alone** (`NOREPL=1`): consent and check, the repl tab never opened, the default
  program run 30 times. 751 to 813 MB after consent and the check, 852 to 1,035 MB after the first
  run, peak 1,355, 1,396 and 1,400 MB over the 30 runs.

The REPL session's tab peaks about 100 MB (7%) above the playground's own; most of either is the
page's compiler instances (V8's code for the 16.9 MB compiler module) and the runner workers. The
REPL compiler's own memory, the bar, is the 49 MB above.

## Per entry (MB, ms)

| # | entry | mem 0.49.1 | mem main | ms 0.49.1 | ms main | RSS 0.49.1 | RSS main |
|---|---|---|---|---|---|---|---|
| 1 | `use std/adler32` | 35 | 35 | 414 | 401 | 852 | 860 |
| 2 | `1 + 2` | 41 | 41 | 98 | 95 | 1102 | 1096 |
| 3 | `let xs = [3, 1, 2]` | 41 | 41 | 33 | 30 | 1049 | 1040 |
| 4 | `xs.sort()` | 44 | 44 | 71 | 71 | 1155 | 1151 |
| 5 | `fn sq(x: Int) -> Int { x * x }` | 44 | 44 | 24 | 23 | 1270 | 1267 |
| 6 | `sq(7)` | 44 | 44 | 26 | 25 | 1311 | 1308 |
| 7 | `fn sq(x: Int) -> Int { x * x * 1 }` | 44 | 44 | 22 | 22 | 1312 | 1308 |
| 8 | `sq(8)` | 44 | 44 | 23 | 23 | 1312 | 1309 |
| 9 | `struct P { x: Int }` | 44 | 44 | 23 | 22 | 1314 | 1310 |
| 10 | `let p = P{x: 1}` | 44 | 44 | 26 | 25 | 1424 | 1360 |
| 11 | `p.x` | 44 | 49 | 26 | 26 | 1501 | 1316 |
| 12 | `struct P { x: Int, y: Int }` | 44 | 49 | 29 | 29 | 1505 | 1319 |
| 13 | `let p = P{x: 1, y: 2}` | 44 | 49 | 23 | 22 | 1505 | 1320 |
| 14 | `p.x + p.y` | 44 | 49 | 23 | 22 | 1506 | 1321 |
| 15 | `fn (p: P) sum() -> Int { p.x + p.y }` | 49 | 49 | 24 | 22 | 1511 | 1322 |
| 16 | `p.sum()` | 49 | 49 | 23 | 23 | 1509 | 1320 |
| 17 | `var n = 0` | 49 | 49 | 22 | 22 | 1509 | 1321 |
| 18 | `n += 5` | 49 | 49 | 23 | 22 | 1510 | 1322 |
| 19 | `n` | 49 | 49 | 22 | 22 | 1511 | 1322 |
| 20 | `let s = "a,b,c".split(",")` | 49 | 49 | 44 | 44 | 1513 | 1325 |
| 21 | `s.len()` | 49 | 49 | 30 | 29 | 1515 | 1326 |
| 22 | `adler32.checksum(b"abc")` | 49 | 49 | 32 | 30 | 1515 | 1327 |
| 23 | `let f = fn(x: Int) -> Int { x + 1 }` | 49 | 49 | 32 | 30 | 1516 | 1327 |
| 24 | `f(41)` | 49 | 49 | 32 | 30 | 1517 | 1328 |
| 25 | `enum Color { Red, Green }` | 49 | 49 | 30 | 29 | 1517 | 1329 |
| 26 | `let c = Color.Green` | 49 | 49 | 31 | 30 | 1518 | 1330 |
| 27 | `c` | 49 | 49 | 30 | 29 | 1519 | 1330 |
| 28 | `"${n}!"` | 49 | 49 | 30 | 30 | 1520 | 1331 |
| 29 | `let m: Map[Str, Int] = {"a": 1}` | 49 | 49 | 60 | 57 | 1522 | 1333 |
| 30 | `m.get("a")` | 49 | 49 | 53 | 54 | 1523 | 1334 |

Raw: repl30/page-0491.json, repl30/page-main-955a6c8a7.json (lead's notes).

## Where the 49 MB is: live bytes against the heap (scratch probe)

A scratch build of main 955a6c8a7 (never pushed) wrapped the wasm runtime's `$alloc` and `$free`
(compiler/wasm/rt.xo) to count the bytes of live blocks (headers and size class rounding included)
and their peak, exported as `xo_live_bytes` and `xo_peak_bytes`, and the scratch page's toolchain.js
reported them after each entry. The same session on that page:

| after entry | wasm memory | live between entries | peak live so far |
|---|---|---|---|
| 1 `use std/adler32` (std's record decoded, relocated, installed) | 35 MB | 21.8 MB | 28.1 MB |
| 2 `1 + 2` (the first check and build) | 41 MB | 21.8 MB | 32.8 MB |
| 4 `xs.sort()` (the first std body lowered) | 44 MB | 21.8 MB | 36.3 MB |
| 11 | 49 MB | 21.8 MB | 36.3 MB |
| 30 | 49 MB | 21.8 MB | 36.5 MB |

- The session holds 21.8 MB between entries, from entry 1 to entry 30 (nothing accumulates).
- Live bytes peak at 36.5 MB, inside an entry (entry 4 and later: an entry's check and build on top
  of the warm session), not at the record's load (28.1 MB at entry 1).
- The heap's end is 12.5 MB above the live peak: free blocks the later requests do not fit, and the
  1 MiB the grow step adds past the end.

So releasing the decoded record before its relocated copy is built (decision 0583's last named item)
lowers entry 1's transient, which is 8 MB below the session's live peak: it cannot lower the 49 MB
alone. The levers on the number are an entry's own peak over the warm session (14.7 MB at entry 4:
the entry's checker copying std's object list, 0583 (f)) and the 12.5 MB between the live peak and
the heap's end (where the free space sits, by block size, is the next probe).

## The free big blocks, and std's record relocated in place

The probe also listed the free big blocks (256 KiB or more) after each entry. On main, entry 1 (the
record's load) leaves 1.2, 2.6 and 5.7 MB free below the session's data; entry 2 adds 4.3 MB, entry
20 another 4.4 MB, and the heap's top climbs from 34.3 MB (entry 1) to 48.9 MB (entry 20).

Relocating the decoded record's object and type lists in place (decision 0583, "std's record
relocated in place"), measured the same way:

| | main 955a6c8a7 | in place |
|---|---|---|
| wasm memory after entry 1 | 35 MB | 33 MB |
| free big blocks after entry 1 | 1.2, 2.6, 5.7 MB | 1.2, 6.9 MB |
| heap top after entry 30 | 48.9 MB | 47.5 MB |
| **compiler memory, peak** | **49 MB** | **48 MB** |
| live between entries / peak live | 21.8 / 36.5 MB | 21.8 / 36.5 MB |

All 30 values the same.

## The builder's copy of the object list (2026-11-12)

A second probe (scratch, never pushed) logged every big block request and free (256 KiB or more)
with its wasm stack, named from the WAT of the same build (XO_KEEP_WAT=1). Every entry copied the
checker's 4.3 MB object list twice: `check.Checker.new_module` (the entry's check, `var c = base`;
base keeps its own) and `check.Checker.need_tree` or `force_body` under the SSA builder, whose
checker still shared the list with play_repl (which read the entry's types after the build) and
with `ssa.build_program_fe` (which keeps its checker for a second build). Entry 4's live peak held
three object lists.

With the types taken before the build and the builder handed the checker (`ssa.build_program_handed`,
decision 0583 "the SSA builder owns the entry's checker"), the same 30-entry session in the same
probe page:

| | relocated in place (8d404d9e3) | builder owns the checker |
|---|---|---|
| wasm memory after entry 1 | 33 MB | 33 MB |
| peak live | 36.4 MB | 31.4 MB |
| free big blocks after entry 30 | 1.2, 6.9, 4.3, 4.4 MB | 1.2, 6.9 MB |
| heap top after entry 30 | 47.5 MB | 38.4 MB |
| **compiler memory, peak** | **48 MB** | **39 MB** |
| later entries, median | 35 ms | 31 ms |

All 30 values the same; natively, every reply (types too) of this session after a Mutex and Chan
session, and of `:reset` and `:undo` back to no concurrency, is the same as 8d404d9e3's compiler's.
The renderer's RSS is not compared: one run each, and it ranged 1,370 to 1,581 MB across runs of the
same page in this probe.

## install_std takes the decoded object list (2026-11-12)

The trace's entry 1 then showed install_std's new object list (4.3 MB) beside the decoded one (5.7
MB, sized at half again its length for copies to reuse), freed into the 6.9 MB block. With
install_std taking the decoded list over (decision 0583, "install_std takes the decoded object
list"), the decoded list sized at 9/8 of its length plus 1/32, the same session in the same probe
page:

| | builder owns the checker | install_std takes the list |
|---|---|---|
| wasm memory after entry 1 | 33 MB | 32 MB |
| free big blocks after entry 30 | 1.2, 6.9 MB | 1.2, 1.2, 4.3 MB |
| heap top after entry 30 | 38.4 MB | 37.1 MB |
| **compiler memory, peak** | **39 MB** | **37 MB** |

All 30 values the same. The 4.3 MB block is each entry's copy of the object list (`var c = base`),
freed after the entry and taken again by the next. The 1.2 MB blocks are the record's read buffer
and decode_std_entry's copy of its bytes.

## decode_std_entry reads the record in place

decode_std_entry decoded `d[at..]`, and on wasm a Bytes slice is a copy (the runtime's
`$str_slice`; slices are views only on the native backends, decision 0330): a second 1.2 MB block
beside the record's read buffer. It now reads the record from offset `at` (decode_std_export_at).
scripts/browser-memory.sh (decision 0615) on the Mac, the parent ba5912f10 against the change, the
same site (l2-rel0500's test):

| | parent | in place |
|---|---|---|
| /repl/ compiler peak | 37 MB | 36 MB |

All 30 values the same. (The /play/ footprints, 463 and 492 MB, are within that test's run-to-run
spread; /play/ does not load the record.)

## The warm session by owner (native heap profile)

The browser has no heap profiler, so the same session was run on a native build of compiler/wasm
(`--play repl`, the page's protocol, with each entry's run answered empty, so state but not values),
on main 2cfa0898b's tree with a scratch `debug.mem_label` before each request (never pushed), under
`XO_PROF=heap XO_PROF_HEAP_RATE=4096` and `XO_PROF_HEAP_AT`. The driver is drive-2026-11-12.py in
the lead's notes (xo-lang-2/replown). Native sizes are not wasm sizes (both use 8 byte words for a
struct's fields, but the allocator's headers and size classes differ: 15.3 MB native here against
the 21.8 MB counted on wasm), so the shares are the result:

| owner (allocation site) | live after warm-up | after entry 30 | share |
|---|---|---|---|
| std's object list (decode_std_export_at, export.xo:797: `List[Obj]` with 1/8 + 1/32 room) | 6.67 MB | 6.67 MB | 43% |
| the record file's bytes (play_check's `fs.read`, natively kept alive by the names sliced from it, which are views; on wasm a slice copies, so there the buffer is freed and the names are copied) | 1.17 MB | 1.17 MB | 8% |
| each object's params and int lists (ExR.obj :571, ExR.ints :265) | 1.95 MB | 1.95 MB | 13% |
| std declarations parsed where a signature needs one (decision 0428: parse_fn_decl and below) | 1.9 MB | 1.9 MB | 12% |
| std's file ranges (decode_std_export_at :923), relocation, install_std, join_trees | 1.2 MB | 1.2 MB | 8% |
| maps (`rebuildk[Str]`, put, mlog), about | 1.4 MB | 1.4 MB | 9% |
| total | 15.26 MB | 15.39 MB | |

Nothing accumulates over the 30 entries (+0.13 MB). The object list dominates, and what makes it
large is `Obj` itself: 75 fields, each at least one 8 byte word (compiler/ssa cost.xo
`type_words`: Bool, Int, List and Map are 1 word, Str 2), about 82 words or 656 bytes an object.
23 of the fields are Bools, 23 words (184 bytes, 28% of every object). Each entry's checker also
copies this list (`var c = base`, the 4.3 MB block above), so the list's size counts twice at the
peak.

## Obj's Bools in one word

Lever 1 below, done: `Obj`'s 23 Bools in one `ObjFlags` word (`x.flags.used()`,
`x.flags.set_used(v)`; decision 0620's measurement). scripts/browser-memory.sh, parent 311a65a63
against the change: the REPL's compiler peak 36 to 35 MB. Less than the native estimate: on wasm a
Bool is a 4 byte leaf, so the Bools were about 20% of Obj there. xo's self build on linux/arm64
(`--ab`, 7 rounds): wall 0.991, RSS within noise.

## Next

The margin is now 15 MB. Levers on the object list, in order of payoff:

1. (Done above.) Pack Obj's 23 Bools into one flags word (`flags: Int`, methods for each). This saves about 22 of
   82 words: about 27% of the list, and of each entry's copy, so roughly 2 to 2.5 MB at the REPL's
   peak. Every native build's checker gains too. It is a mechanical change across compiler/check and
   its readers.
2. Or the general form: the native backends pack a struct's Bool fields into shared words. That is
   a layout change on every backend, so an ADR first. It shrinks every struct with Bools, not only
   Obj.
3. Move the fields only some objects use into side tables: Json's 7 fields for derive Json types;
   a local's let_at, let_value, vctx, frame and group_frame (std's record has no locals); and
   realtime_span, internal_hint and elided_from.

Then: (1) the small blocks' free lists (6.9 MB free after entry 30).
