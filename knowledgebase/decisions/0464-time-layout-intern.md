# 0464: parsed time layouts are interned, so a string layout costs a const Layout's walk

Status: Proposed (Xo Lang 3, 2026-11-03; the Lead asked for a hoist of literal layouts, this is the library side alternative, pending the Lead's answer)

## Context

`time.format(t, z, "...")` and `time.parse` parse the layout text on every call
(`layout_items`, 51% of the format profile). bench/results/std-twin-rows2-2026-11-03.md:
string layouts run at format 5.4 and parse 4.7 of Go's wall; the same layouts as a
`time.Layout` const (decision 0332) run at 2.03 and 1.90.

The Lead's request was to turn a literal layout argument into a const Layout at compile
time. Reading compiler/check/timelayout.xo: the layout check (XO0420) runs after name
resolution and only reads `call_infos`. A hoist needs either a rewrite before resolution
(it cannot tell `time.format` from a shadowing `time`) or a synthesized `xo.const[...]`
entry plus `rt.const_once` sites in SSA lowering, with constant numbering in the front
end cache (fecache.xo) and all three backends. That is a multi file compiler change.

## Decision (proposed)

`format` and `parse` with a string layout look the layout up in an interned table of
parsed layouts: 64 direct mapped slots keyed by a hash of the text, each slot an immutable
entry published with a compare and swap (the `rt.const_once` pattern), compared by content
on a hit, never replaced once filled (a program that builds layouts at run time cannot
grow it). A miss parses as today and tries to publish. Compile time XO0420 checks and the
run time fault on a bad layout are unchanged.

## What the first prototype found (2026-11-03)

The safe Xo form, `const CACHE: Mutex[List[Cached]] = Mutex[...].new(...)` with `with_read` for
hits and `with` to fill a slot, is rejected: XO0416, "a constant is an immutable value shared by
every task, not a global variable" (decision 0332); Atomic and every other handle type are
rejected the same way. So the table is std's first process wide mutable state and needs a
primitive outside the language:

- native (arm64, LLVM): a slot in the services record (ctx.xo SV_*, as SV_CONSTS and SV_ROOTS
  hold theirs) pointing at a 64 word block made at first use with `rt.slot_cas` (the pattern of
  ctx.xo add_root and consts.xo table). Each slot holds the address of an immortal entry (its
  reference count set to IMMORTAL, so a reader needs no retain and the leak check sees the
  bounded 64 entries freed at exit through the consts_free list); a filled slot is never
  replaced, so a reader that loaded an address can use it without a lock. Publish is
  `rt.slot_cas(block, i, 0, entry, ACQ_REL)`; the loser drops its entry.
- Go backend: a package variable in std/rt, `[64]atomic.Pointer[tlEntry]`, filled with
  `CompareAndSwap(nil, e)`; the Go garbage collector owns the entries. Same bound and rule.
- wasm: single threaded, so a plain 64 slot table in linear memory, same slot rule (no
  atomics needed; rt4.xo has `slot_*` lowerings if threads arrive).

Cost of the primitive: one new services record word (SV_SIZE 296 grows by 8, a layout constant
shared with xo_rt.c's older readers), an immortal-entry constructor, and TestCodegenInputs
classification for the new state. Tests: a race test (16 tasks formatting and parsing 10
distinct layouts and 200 colliding ones, under `--race`), the layout bound (65th colliding
layout still correct), and the existing time corpus on all three backends.

## Consequences

- Expected: string layout format and parse near the const Layout figures (2.0 and 1.9),
  also for layouts built at run time.
- A hash and a content compare per call replace the walk; a layout that collides with a
  filled slot is parsed each call as today.
- No compiler change, no golden or fecache fallout.

## Alternatives

- Compiler hoist of a literal layout into a const Layout (the Lead's request): removes the
  hash and compare, but needs the multi file change above; can still be done later and
  would simply stop hitting the table.
- A Mutex held Map cache: measured slower in bench/results/time-layout-parse-cost-2026-10-21.md.
- Do nothing: document that hot code uses `time.Layout`.
