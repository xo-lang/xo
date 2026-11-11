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

## Amendment (owner, 2026-11-11 morning, via the lead): rt.Loc and rt.fault_at

Point 1's list grows by one pair: a merged std module may fault at its
caller's location, as native std does today with a trailing `at: rt.Loc`
parameter and `rt.fault_at(msg, at)`. This unblocks std/fnv and std/task,
whose only native calls are these (fnv's update128 length check; task's
semaphore and map_concurrent limit checks). rt.fault_raise and
rt.task_deadline_left (task) stay off the list.

Today the pair works only between a bodyless std declaration and its
native implementation: the declaration has no `at` parameter, and SSA
fills the implementation's extra trailing `rt.Loc` parameters with the
call's position (compiler/ssa/call.xo loc_args). The Go backend has no
fault_at; std/rt's hand-written twins fault themselves. In a merged
module the declaration is the implementation, so using the pair needs,
in the push that first does it: (a) the checker treats a trailing
`rt.Loc` parameter of a merged std function as filled at each call (the
caller writes no argument for it), and adds fault_at to
PORTABLE_INTRINSICS (compiler/check/call.xo); (b) SSA fills it at a
direct call as loc_args does, and a function value of such a function
stays refused (decision 0094's rule); (c) the Go backend fills it with
the call's position and lowers rt.fault_at to its fault with that
location; (d) spec/core.md or spec/std.md says where a std fault
reports its location.

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
- Part 3, Go PartReader (2026-11-10): std/rt/http_part_reader.go runs
  the generated `Multipart_reader` over the body source instead of Go's
  mime/multipart, so no Go multipart code is left in std/rt.
- Part 2, binary (2026-11-10): rewritten with prelude operations
  (write_le, write_be, byte reads after one check), --ab fixed 0.823
  (0.772 to 0.838, n=7) on host 1: merged whole, no native part.
  url: escape over BytesBuilder was 1.075 (1.053 to 1.107) and with one
  write_be per escape 1.041 (1.023 to 1.068), so its escape stays native.
- Part 2, the split (2026-11-10, stage A): a merged module's fast path is
  a bodyless declaration in std/M.xo. The parser takes one in a top level
  std/M.xo file (parser.xo std_top) and the checker reports XO0172 for
  one in any module that is not merged std (load.xo parse_into); the
  loader loads std/native/M_native.xo as a merged module's
  implementation (load_impl), SSA routes a call of the bodyless function
  to it as an unmerged module's (build.xo native_part, call.xo), and the
  Go backend calls its hand written twin in std/rt (gen.xo rt_twin,
  rt.Url_escape). Stage B, url split this way, waits for a seed: an
  older xo stops at the bodyless declaration (XO0172) when it builds
  compiler/xo, which imports std/url.
- Part 2, std/html (2026-11-10): merged with escape_string as the
  bodyless fast path (std/native/html_native.xo, the Go backend's twin
  Html_escape_string in std/rt/html.go). The portable form, prelude
  BytesBuilder writes of the runs and the replacements, took 1.674 times
  the native one (1.659 to 1.692, n=6; host 1, bench/perf/html escape),
  so the fast path stays. unescape_string was already portable. html
  needs no seed: compiler/xo does not import it.
- Part 2, std/url (2026-11-11, stage B, after seed 0.48.0): merged with
  `escape` as the bodyless fast path (std/native/url_native.xo; the Go
  backend's twin Url_escape in std/rt/url.go, which keeps urlText for
  std/rt's http). url is rt-hosted (std/rt_hosted.txt): std/rt's http and
  the generated multipart code call its functions.
- Part 2, protowkt (2026-11-11): the last module of the table. Its
  native file imported `native/protodesc` for the wire helpers; they are
  the merged internal module std/protowire now, and protowkt merged with
  protodesc and protojson (decision 0588 progress). None of the three is
  rt-hosted: an rt-hosted module's Go code is in package rt, which
  cannot call a go_compiled module such as protobuf
  (gotchas/rt-hosted-calls-go-compiled.md). regexp is the table's other
  open module.
- Part 2, std/regexp (2026-11-11): merged; the rt, raw, scan and
  unicode_classes uses became portable helpers (byte_at, sl, decode_at,
  find_from), the class tables the internal module std/uniclass, and
  `must`, `simple_fold` and `zeros` (a push loop measured 1.067 on
  bench/perf/regexp match) the bodyless native part. regexp and uniclass
  are rt-hosted. Timing: bench/perf/regexp, xo bench --ab on linux/arm64 host 1 against the base 9e8095f26: compile 0.968 (0.963 to 0.982), match 0.994, replace_all 1.012 and captures 0.998 within noise, find_all 1.036 (1.029 to 1.044) slower: the port reads input bytes with a bounds-checked b[pos] where the native code used an unchecked raw.byte_at, and the arm64 bounds analysis (opt.xo, decision 0184) has no loop lower bound to drop the check; a compiler item removes it.
- std/image (2026-11-11): merged whole (its native file imported nothing
  native), rt-hosted since std/rt/gif.go builds Image_Image; std/binary,
  which it uses, is rt-hosted with it (an rt-hosted module's merged
  imports must be generated into std/rt too, else its Go names
  `MISSING_PKG_x_std_binary`). to_png's `?? return None` became a match:
  that form in an rt-hosted method generates code go vet rejects
  (unreachable code).
- Part 1, std/netip (2026-11-11): merged; its two `rt.cast` calls are on
  part 1's list. rt-hosted, since std/rt's client_ip and x509 build its
  values; std/rt/netip.go keeps only the runtime's conversions to and
  from Go's net/netip, and the display methods, which were Go's net/netip
  on the Go backend, are the port's there too (the netip corpus checks
  them against Go on every backend).
- std/norm (part 1, its one `rt.cast`) and std/backoff (nothing native)
  (2026-11-11): merged; backoff is rt-hosted, since std/otlp, which is
  generated into std/rt, uses it (gotchas/rt-hosted-merged-imports.md). backoff.retry and retry_notify
  left compiler/ssa/waits.xo's NO_WAIT: they have Xo bodies now.
  std/idna (one `rt.cast`) merged with them, not rt-hosted (nothing
  generated into std/rt uses it); its Go twin, only IdnaErr's display,
  is gone.
  std/protobuf cannot merge this way: marshal and unmarshal are the
  compiler's (derive Proto), with no body to move.
- The amendment's first use, std/fnv (2026-11-11): merged. The checker
  lists Loc and fault_at as portable and counts a std function's
  trailing rt.Loc parameter as filled (check/call.xo is_loc_type); SSA
  fills it with `file:line` at a direct call, as loc_args does for a
  native implementation (ssa/call.xo); the Go backend fills it the same
  way (gen/call.xo) and std/rt has Rt_Loc (a string) and Rt_fault_at
  (Fault at that position). spec/core.md 8 says a std argument fault
  reports `FILE:LINE`. Golden fnv_caller_loc checks the position on
  arm64, LLVM, the Go backend and wasm. std/task still waits: it also
  calls rt.fault_raise and rt.task_deadline_left.
- std/amqp (nothing native) (2026-11-11): merged, not rt-hosted. In its
  own module its wildcard arms over its own enums warned (XO0305: seven,
  now listing the variants) and its test peers' net.listen effect was
  checked (eleven gained `uses net.listen`); twenty Connection, Receiver,
  Sender and amqp functions left compiler/ssa/waits.xo's NO_WAIT.
- std/term and std/bufio (nothing native) (2026-11-11): merged, not
  rt-hosted; the Go backend compiles them from source, so their Go twins
  (std/rt/bufio.go, the std/term part of std/rt/term.go) are gone, and
  term.color_enabled and term.painter left compiler/ssa/waits.xo's NO_WAIT.
- std/png, std/jpeg and std/htmlparse (nothing native) (2026-11-11):
  merged; png and jpeg are rt-hosted, since std/image, generated into
  std/rt, uses both; htmlparse is not. Their Go twins (only the error
  displays) are gone. TestStdRuntimeCallsHaveGoLowering now scans a
  merged module's std/M.xo too, where its rt.* calls moved. std/cbor
  cannot merge, like std/protobuf (derive Cbor). std/amqp is next: in
  its own module its wildcard arms over its own enums warn (XO0305) and
  its tests' effects are checked.

- std/trace and std/metrics (2026-11-11): merged and rt-hosted (std/otlp,
  generated into std/rt, uses both; gotchas/rt-hosted-merged-imports.md).
  Neither imports anything native. trace's seven OpenTelemetry Go oracle
  tests encode with std/otlp, which uses std/trace, so in trace's own
  tests they would be an import cycle: they moved to
  std/native/otlp_test.xo (scripts/oteloracle/gen.py writes them there).
  rand_hex gained `uses rand`, which the native file had not declared.
