# 0588: One file per std module: merge std/M.xo (declarations) with std/native/M.xo (bodies)

Status: Proposed (Lang 3, 2026-11-10; owner request via the lead). The owner
decides before any file moves.

## Context

A std module is up to three parts today:

- `std/M.xo`: the public API as bodyless signatures (77 files, 8.6k lines).
- `std/native/M.xo`: the implementation the native backends and the Go
  backend (for modules in std/go_compiled.txt, decision 0312) compile
  (86 files, 134k lines), joined to the declaration by name: a function
  `f` by its name, a method `fn (e: T) m()` by `t_m` (compiler/ssa
  build.xo:2202 lowercases the type), e.g. `charseterr_display` for
  `CharsetErr.display`.
- `std/rt/M.go`: the Go backend's hand written Go for the modules not in
  std_compiled (344 Go files).

Why the split exists: the declaration file was the single source the Go
backend's std/rt was checked against (internal/stdgen emits std/rt's types
and signature checks from it, decisions 0164, 0172), and the native
backends needed a place for code that uses runtime intrinsics (`rt.*`),
`native/llrt` modules and `use c` adapters, which the user-facing
declaration must not expose. Decision 0312 made the pure Xo modules compile
on the Go backend from their native file, which removed the Go twin for 38
modules but left the two Xo files.

Cost: a reader or an editor sees signatures in std/M.xo and has to find the
body in another directory under a mangled name (`xo def` and Azure 1's lsp
work now jump to the body, but the source is still split). Go's std is one
file per function with doc comment and body together.

## Survey (2026-11-10, origin/main)

Modules with a declaration file, by what their native file uses:

- Pure (no `native/llrt`, no `rt.` intrinsic, no `use c`): 38: amqp,
  amqptest, backoff, base32, base64, bcrypt, bufio, cbor, charset,
  cookiejar, cron, deque, grpc, heap, htmlent, htmltok, jwt, mail, metrics,
  mime, multipart, otlp, otp, path, png, protobuf, protodesc, protoparse,
  protowkt, rate, smtp, tabwriter, term, timego, trace, uuid, xmlcodec, yaml.
- `rt.` intrinsics, no llrt: 9: adler32, crc64, fnv, idna, jpeg, netip, norm,
  sqlite, task.
- `native/llrt`: 29: archive, big, binary, cli, compress, crc32, crypto,
  csv, debug, dns, embed, html, http, internal, json, log, prelude, regexp,
  sql, template, testing, time, tls, toml, udp, url, websocket, x509, xml.
- No native file: go (Go-only).

(std/prelude.xo, a declaration file the compiler itself knows, is a special
case below.)

## Decision (proposed)

1. **One file per module with real bodies**: `std/M.xo` holds the public
   declarations *with* their bodies, doc comments on the code, as a user
   package does. `std/native/M.xo` disappears for a merged module. Private
   helpers sit in the same file (unexported, as in Go).
2. **What stays declared**: a function whose body is a runtime intrinsic or
   a C adapter keeps a bodyless declaration in `std/M.xo` and its body in a
   `std/native/M_native.xo` (or the existing llrt module), so the public
   file is still the whole API and shows which functions are intrinsic.
   Go-only modules (go) and `std/prelude.xo` (types the compiler knows)
   stay as they are.
3. **Order of modules**: the 38 pure modules first (nothing but the loader
   changes), then the 9 intrinsic ones (the intrinsic part splits out), the
   llrt ones last (http 5k lines, json, time: their native files are the
   product; merging may mean a module directory std/http/ with several
   files rather than one file; the owner decides when we get there).
4. **Loader**: compiler/check/load.xo `load_std` learns that a std file may
   carry bodies (today it parses std files as declarations only,
   `parse_into(..., true)`), checks them as a user module with the std
   privileges (`rt`, llrt imports), and drops the lookup for a native
   twin when the file has bodies. The SSA join by mangled name is kept for
   declared-only functions.
5. **Tools**: stdgen reads signatures from the merged file (a body is
   stripped to its signature for std/rt's checks); stdindex, `xo doc` and
   spec/std.md's generated index already print signatures and docs, not
   bodies, so they print the same text; lsp go-to-definition lands on the
   real body. spec/std.md keeps its signature listing (generated from the
   same parse).
6. **Seed and fixed point**: the seed xo reads the std tree of the release
   it builds, so a seed that cannot load a bodied std file cannot build the
   new tree (gotchas/seed-older-than-intrinsics.md). Stage 1 adds the loader
   support only; it is a minor version bump with a new seed cut (CLAUDE.md,
   "Seeds"), after which stage 2 moves modules. std/native is part of what
   compiler/xo builds from, so each move that touches a module the
   compiler imports (`path`, `bufio`, `term`?) needs the self build and the
   fixed point; the others need xobuild, the module's corpus and
   goldens on go, arm64 and llvm.
7. **Compatibility of tests**: `std/native/M_test.xo` files move next to
   the module as `std/M_test.xo` (the loader already reads `_test.xo` by
   name, `load_std_tests`).

## First module (to be measured in stage 0, before the owner decides)

Candidates, small and pure, with a corpus: `uuid` (44 + 101 lines, in
go_compiled.txt already), `base32` (25 + 162, tests), `otp`. Stage 0 is a
prototype branch that merges `uuid` and `base32` by hand and measures:
the check time of `xo check std/M.xo` and of a hello world importing it
(the merged file is parsed with bodies on every import, where today only
the declaration is read for a user program that does not build the std
module; the lazy checking of decision 0310-era work may make this free or
may cost), the binary cache keys (a body change now changes the
declaration file's hash: users' separate-compilation caches that key on
declaration hashes would recompile dependents; this is the main risk and
must be measured), and the self build time.

## Consequences

- One place to read, edit and review a module; the declaration/body drift
  class disappears (decision 0312's "two implementations" cost, now only
  for intrinsics).
- The loader and the checker's std privilege rules change: a std file may
  import `native/llrt` and `rt` only where the split kept an intrinsic body
  (a lint: a public file importing them is allowed only for modules listed
  in a table).
- Public-API diff reviews now show body changes in the API file; stdsync
  and habit goldens keyed on declaration hashes (cmd/xo `prelude.sha256`)
  must hash signatures only.
- Cost: a seed cut and a minor bump for stage 1; module moves are large
  mechanical diffs (rename-heavy; `git log --follow` works if the native
  file's history is kept by moving the native file over the declaration).

## Alternatives

- Keep the split, improve tools only (lsp jumps, `xo doc --body`): cheapest,
  already under way (Azure 1); does not fix the second-file edit habit.
- Generate the declaration file from the native file (one source, a derived
  signature file): the reader still sees one file; but the derived file
  still has to be checked in for the seed and spec, and it only covers pure
  modules.
- Merge the other way (bodies into the declaration for all modules including
  llrt ones in one step): too large to review; rejected in favor of the
  staged plan.

## Staged plan

0. Prototype and measurements on `uuid`, `base32` (no push).
1. Loader and tools support, `std/M_test.xo`, a new seed (owner approves the
   cut).
2. The pure modules, a few per push, each: corpus and goldens on go/arm64/llvm,
   std/changes entry (decision 0276, kind refactor, no behavior change).
3. The intrinsic modules.
4. The llrt modules, directory modules where a file is too large.
