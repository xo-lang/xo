# 0597: Merged std modules that need native code: portable intrinsics, split fast paths, public parsers

Status: Accepted (owner via lead, 2026-11-10: yes to all three parts;
proposed by Xo Lang 3 the same day). Asked for by the
lead (owner's go to write the proposal, 2026-11-10) after decision 0588's
stage 2 stopped at modules whose implementation imports native code.
Related: 0588 (one file per std module), 0074 (implementation modules and
intrinsics), 0180 (runtime modules in Xo), 0319 (an implementation's
`use native/M` of another), 0214 (std/internal).

## Context

Decision 0588 merges `std/M.xo` (declarations) with `std/native/M.xo`
(bodies) into one file. Twenty modules are merged (std/merged.txt). The
rest cannot be merged as they are, for two reasons.

**1. The checker's import rule.** compiler/check/load.xo (the `use`
resolution, about line 1910 on 12546d433) lets a module import
`native/rt`, `native/llrt/*` or another module's `native/M` only when it
is an implementation module (`Module.native`, a file under std/native).
A merged `std/M.xo` is a std module, not an implementation, so its
`use native/...` resolves to nothing (XO0601). This is deliberate: the
intrinsics are unchecked memory access (`rt.mem_store`, `rt.mem_copy`,
`raw.bytes_buf`) and must not reach user code.

The Go backend adds a second limit: it compiles a merged module's bodies
as their own Go package, and std/rt has a Go twin for only one intrinsic,
`Rt_cast` (std/rt/str.go:560, a Bytes/Str view). The go_compiled modules
that import native/rt today (path, netip, norm, idna) use nothing else.

What the blocked modules use (their std/native file on 12546d433):

| Module | Lines | Native imports | Intrinsic calls |
|---|---|---|---|
| path | 572 | rt | rt.cast 3 |
| url | 334 | rt, llrt/raw | mem_store 4, mem_copy 2, bytes_addr 2, bytes_buf, alive, cast |
| binary | 127 | rt, llrt/raw | bytes_addr 6, mem_store 3, mem_load 3, bytes_buf 3 |
| html | 310 | rt, llrt/raw, native/htmlent | mem_store 6, bytes_count_eq32 5, popcount 2, mem_copy 2, bytes_word 2, ctz, cast |
| regexp | 3,529 | rt, llrt raw, scan, unicode, unicode_classes | zero_ints 17, simple_fold 15, scan.find 7, byte_at 4, ... |
| htmltok, protowkt | 1,214, 340 | native/htmlent, native/protodesc | none |
| multipart | 1,097 | none | raw.len 5, write, freeze (its own BytesBuilder helpers) |
| mime | 736 | none | none |

The intrinsic calls are fast paths (word-at-a-time scans, buffers filled
in place), most of them added after a `--ab` measurement.

**2. Private items used across modules.** Two pure modules are blocked by
their users, not by imports:

- `std/multipart`: std/native/http.xo uses the streaming parser that only
  the implementation file has (`Feed`, `Head`, `Stream`, `new_stream`).
  After a merge they would be private items of std/multipart, which
  native/http cannot see.
- `std/mime`: std/rt's Go http (http.go:801 and 819, multipart.go:33,
  http_part_reader.go:52, gzipresp.go:102) calls the hand-written Go twin
  `Mime_parse_media_type` and `Mime_detect_content_type` (std/rt/mime.go).
  A merged module's Go code is a generated package that imports `rt`, so
  `rt` cannot call back into it.

## Decision (proposed)

Three parts, one per kind of blocker. Each can be accepted on its own.

1. **Portable intrinsics.** A merged std module may `use native/rt`, and
   may call only the intrinsics on a short list that every backend
   implements, the Go backend included. Today the list is `rt.cast`.
   The loader enforces the list at each call, with a new diagnostic code
   naming the intrinsic. Users still cannot import native/rt. This
   unblocks `path`, and later netip, norm and idna. A new entry on the
   list needs Go, native and wasm lowerings, and the same rules as any
   new intrinsic (gotchas/seed-older-than-intrinsics.md).
2. **Fast paths stay split.** No merged file imports `native/llrt` or
   calls a memory intrinsic. A module whose speed rests on them (url,
   binary, html, regexp) merges everything else. Each fast path stays
   behind a bodyless declaration in `std/M.xo`, with its body in
   `std/native/M_native.xo` and a Go twin. This is what 0588's point 2
   already says for intrinsics, applied to fast paths. Before a module
   splits, its intrinsic calls are rewritten with prelude operations
   (BytesBuilder, Str and Bytes views, `Bytes.find`) and measured with
   `xo bench --ab` on the linux/arm64 host. A call whose rewrite is within
   noise is removed instead of split, so the native-only part shrinks to
   the calls that pay for themselves.
3. **Shared parsers become public API, as in Go.** Std code that another
   std module uses is made public with Go's shape, not given a new
   visibility level:
   - std/multipart gains a streaming reader (Go's `multipart.Reader` with
     `next_part`), which native/http then uses through the public API.
   - std/rt's Go http keeps calling Go code for mime, but code generated
     from std/mime's Xo bodies: stdgen emits a `Mime_parse_media_type`
     wrapper into package rt, so the hand-written std/rt/mime.go goes and
     only one implementation is left.
   - htmltok and protowkt merge after htmlent and protodesc, so their
     `use native/htmlent` and `use native/protodesc` become `use
     std/htmlent` and `use std/protodesc` (0588's order).

## Consequences

- One file per module for the API, the docs and the portable bodies. The
  remaining split files say what they are: native-only code, each part
  measured.
- Point 1 keeps the unsafe intrinsics unreachable from user code and from
  merged files. The only thing a merged file gains is a list of
  intrinsics already safe on every backend.
- Point 2 may leave url, binary and html split as today if every fast
  path pays for itself. Then the only gain is the merged declarations and
  the measurements.
- Point 3 adds public API. Go has `multipart.Reader`, `NextPart` and
  `mime.ParseMediaType`, so Xo matches Go. It is permanent, so the
  streaming reader's shape needs its own short review (spec/std.md).
- Generating mime's Go into package rt makes stdgen's output depend on
  one more module. TestStdIndexCurrent and stdgen -check cover it.
- Each module's merge still needs xobuild, its corpus on go, arm64 and
  llvm, and the self build when compiler/xo imports it (path, url, binary
  are in the compiler's closure).

## Alternatives

- **Let merged files import native/llrt and every intrinsic, from a
  table of privileged modules** (0588's consequence line). This is the
  smallest checker change. But the public API file then holds unchecked
  memory access, and the Go backend needs a Go twin for every intrinsic,
  or a hand Go twin of the whole module, which brings back the two
  implementations 0588 removes. Rejected for the memory intrinsics, and
  kept (as point 1) for the safe ones.
- **A `pub(std)` visibility** for items std modules share. It solves
  multipart without new public API, but it is a language feature (spec
  core.md, the checker, the LSP, `xo doc`) for one module. Go solved the
  same need with public API, or with `internal` packages, which Xo
  already has (std/internal, 0214). If point 3's reader is rejected, the
  fallback is to move multipart's streaming parser to std/internal, which
  both std/multipart and native/http can import.
- **Leave these modules split** (do nothing). They work as they are; the
  cost is the declaration-and-body drift 0588 removes, for about 8,000
  lines.
- **Rewrite every fast path portably, with no split at all.** This
  removes the native-only code, but goes against the owner's rule to
  port Go's fast paths (CLAUDE.md, std change logs). It is right only
  where `--ab` shows the rewrite costs nothing, which point 2 measures.

## Owner's answers (2026-11-10, via the lead)

Yes to all three: (1) the portable intrinsics list, `rt.cast` only for
now; (2) rewrite the url, binary, html and regexp fast paths with prelude
operations, `--ab` each on linux/arm64 host 1, and split into
M_native.xo only what still pays; (3) a public std/multipart streaming
reader, and mime's Go generated into package rt. Order of work: (1) with
the path merge, then (3) multipart and mime, then (2) one module per push
with its `--ab` row; spec/core.md and spec/std.md change in the same
pushes.

## Open questions (answered above)

1. Accept point 1 (a list of portable intrinsics, `rt.cast` only for now)?
2. Accept point 2's order: rewrite where measured free, split the rest?
3. Accept point 3: a public std/multipart streaming reader, and mime's
   Go generated into package rt? Or std/internal for the parser?

## Progress

- Part 1 (2026-11-10): a merged std module may `use native/rt`
  (compiler/check load.xo), and naming any member but those of
  `PORTABLE_INTRINSICS` (call.xo: `cast`) is XO0609 (spec core.md 10.6,
  `xo explain XO0609`; checked by adding an `rt.mem_load` call to the
  merged std/path.xo). std/path merged with it (0588 stage 2): its three
  `rt.cast` views stay. scripts merge tool: a local variable named like
  the module (`path.len()`) keeps its prefix.
- Part 3, mime (2026-11-10): std/rt_hosted.txt lists merged modules whose
  Go scripts/stdgen generates into package rt, bodies and all; the
  checker records it on the module (`rt_hosted`, std export v12),
  compiler/gen treats such a module as std (calls `rt.Mime_NAME`, a
  lazy constant `rt.Mime_NAME.Get(tk)`) and std_types emits its functions
  and constants with those names. A separate file and not a mark in
  std/merged.txt, so an older xo (a seed) still reads the module as
  merged. std/mime merged under it; std/rt/mime.go and std_sigs_mime.go
  removed.
- Part 3, multipart (2026-11-10): std/multipart merged and rt-hosted;
  the streaming parser is public as `multipart.Reader[fx]` (`reader`,
  `next_part`, `read`, `PartHead.header_values`, spec/std.md 26), and
  std/native/http's PartReader uses it, so no std-internal `pub` is
  left in it. The Go backend's comparison roots in rt-hosted bodies
  (gen.xo `rt_hosted_roots`, gotchas/merged-std-module-go-proto-roots.md).
  Left: std/rt's Go PartReader still wraps Go's mime/multipart.Reader
  (http_part_reader.go); moving it onto `Multipart_reader` is the next step.
