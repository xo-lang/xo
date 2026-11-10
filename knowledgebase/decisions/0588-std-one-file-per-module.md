# 0588: One file per std module: merge std/M.xo (declarations) with std/native/M.xo (bodies)

Status: Accepted (owner, 2026-11-10, relayed by the lead: "do the proper
fix"). Drafted by Lang 3. Order: the std/http router fixes first, then this.

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

## Decision

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

## Seed question (to answer first, in stage 0)

Whether an old seed can build a tree whose std has bodied files is an open
fact, not an assumption: the seed's `load_std` parses a std file as
declarations (`parse_into(..., true)`). Stage 0 builds the prototype tree
with the current seed (`go run ./scripts/xobuild`). If it builds, modules
can move with no seed bump and the loader change (stage 1) only makes the
compiler accept the layout for itself. If it does not, the plan is a
loader-only change first, pushed with a minor bump and a seed cut for the
owner to approve, and the module moves after it; this is reported to the
lead before any module moves.

## Seed finding (2026-11-10, from reading the loader and the build graph)

- The parser already accepts a function with a body in a signature file
  (`parse_file(..., sig)`: "function declarations may omit their body"), so
  a merged file parses with the current seed; what the seed lacks is
  *compiling* those bodies and not looking for a native twin.
- The seed builds only `compiler/xo` from the tree. The std modules that
  graph imports directly are crypto, task, internal, http, regexp, json,
  testing, debug, archive, protoparse, path, embed and binary (plus what
  they import). A module outside that closure is never compiled by the seed,
  only by the new xo it built, so **the loader change and every module
  outside the compiler's closure can land with the current seed, no cut**.
  A module inside the closure (path, crypto, http, json, ...) can merge only
  after a seed whose loader understands the layout; that one needs the lead
  to cut a seed at the commit that lands the loader.

## Staged plan

0. Prototype and measurements on `uuid`, `base32` (no push).
1. Loader and tools support, `std/M_test.xo`, a new seed (owner approves the
   cut).
2. The pure modules, a few per push, each: corpus and goldens on go/arm64/llvm,
   std/changes entry (decision 0276, kind refactor, no behavior change).
3. The intrinsic modules.
4. The llrt modules, directory modules where a file is too large.

## Stage 1 implemented (2026-11-10, Lang 3)

The loader change landed with `std/uuid` as the first merged module:

- `std/merged.txt` lists the merged modules (`std/uuid`, one a line). `load_std`
  sets `Module.merged` and parses the file with its bodies (`parse_into(..., false)`),
  so no native twin is looked for. Only `pub` items export (`Checker.exports_all`
  replaces the std rule that every item is public); the rest is private as in a
  user module, and its fields and helpers are XO0605 to a program.
- Checking: a merged module's bodies are checked like a native module's (lazily
  in a native build, `lazy_mod`; in full for the Go backend).
- Native backends (compiler/ssa): `is_std_mod` is false for a merged module, so
  its functions and methods build from their bodies like a program's, and its
  Debug and Display methods are found (`method_of`).
- Go backend (compiler/gen): a merged module is generated as an ordinary Go
  package (its types, methods and functions), so its std/rt twin is deleted
  (`std/rt/uuid.go`, `std_gen_uuid.go`, `std_sigs_uuid.go`) and
  `internal/stdgen` skips modules in `std/merged.txt`.
- uuid's test file moved to `std/uuid_test.xo` (private access, methods for the
  natives); `internal.uuid_half` and `uuid_word` are now covered through the
  module's own bodies (internal/stdcoverage/allow.txt lost their lines).
- Gate: `xo test --std-module=uuid`, the uuid corpus on go, arm64 and llvm, the
  uuid goldens, wasm, codegen patterns, the self build.

Seed: modules outside the compiler's import closure can merge with the current
seed; a module inside it (crypto, task, internal, http, regexp, json, testing,
debug, archive, protoparse, path, embed, binary) needs a seed cut at the loader
commit (the lead cuts it).

## Stage 2 progress (2026-11-10, Lang 3)

- `std/base32` merged (std/base32.xo with its bodies, std/base32_test.xo;
  std/native/base32.xo, std_gen_base32.go and std_sigs_base32.go removed;
  `DecodeErr.at` is `pub` so the field stays readable).
- Two tool fixes it needed, now in place for every later module:
  `xo dev dump go --std-types` and `--std-sigs` (compiler/gen stdtypes.xo)
  skip merged modules, since a merged module another std module's
  implementation loads (otp uses base32) was dumped into std/rt's
  std_gen_prelude.go as unprefixed `Base32` and `DecodeErr`; and
  internal/stdcoverage skips `std/*_test.xo` (a merged module's test
  helpers are not std API).
- A merged module needs an xo with the stage 1 loader: an older bin/xo on
  a newer std reads std/M.xo as declarations and fails with XO1201 on the
  first body call (gotchas/merged-std-module-needs-current-xo.md).
- `std/otp`, `std/smtp` and `std/cookiejar` merged the same way, with a
  script (merge_mod.py in the lead's notes) that keeps each declaration's
  signature, including its parameter names (they are API: named
  arguments), and renames the body's uses of the native names. std/rt's
  hand Go `Smtp_SmtpErr.M_display` (errtext_http.go) went with it: the Go
  backend compiles the merged module's own `display`.
- Two limits found on the way, for choosing the next modules:
  - a module whose types another unmerged declaration file names cannot
    merge alone: `std/backoff` was dropped from the batch because
    std/otlp.xo's `Exporter.backoff: backoff.Backoff` made std_gen_otlp.go
    refer to a type std/rt no longer has. Merge such a module together
    with the declaration files that name it (backoff, metrics and trace
    with otlp; protobuf with protodesc, protojson and protowkt), or after
    them.
  - a module with a hand written Go twin in std/rt (bcrypt.go, cron.go,
    deque.go, ...) needs that file removed or reduced in the same change.
- `std/timego` and `std/xmlcodec` merged. Two more limits:
  - a std module cannot `use native/M` (XO0601 from std/M.xo), so a
    module whose implementation imports another's native file waits for
    that one: htmlent (std/native/html.xo and htmltok import
    native/htmlent), htmltok, and protowkt (imports native/protodesc).
  - the merge puts the declaration's effect clause on the body for the
    first time: std/amqptest's `Broker.put`, `ready`, `locked`,
    `scheduled` and `event_hub` are declared pure but their bodies read
    the broker's clock (XO0701). A native body's effects were never
    checked against its declaration; amqptest waits for a decision on
    adding `uses clock` to those methods (an API change).
- internal/stdcoverage reads `pub fn (` methods since 3dd0401cf, and its
  walk now reaches cbor.put_int (its allow.txt line, added at the same
  time, removed here).
- `std/deque` merged with its hand Go twin removed (std/rt/deque.go): a
  merged module is generated as its own Go package whether or not it is
  in std/go_compiled.txt, generic and `var` methods included, so the Go
  backend compiles the Xo deque (corpus deque on go, arm64, llvm; gen
  golden std_opaque_copy). The merge script now also takes an opaque
  declaration (`struct Deque[T] {}`) from the native struct (fields stay
  private) and native methods defined as methods.
- `std/heap` merged the same way as deque (std/rt/heap.go removed; the
  generated prelude loses Heap_Heap's Eq, Hash and Debug); its declared
  `_items`, `_less`, `_moved` fields become the native struct's private
  ones.
- `std/tabwriter` merged the same way (std/rt/tabwriter.go removed; the
  native TabWriter's fields are private).
- `std/rate` merged (std/rt/rate.go removed; the native file's private
  copy of `INF` dropped for the declared one; compiler/ssa/waits.xo
  NO_WAIT drops the Limiter and Reservation methods, which have Xo
  bodies now: TestWaitsClassified).
- `std/cron` merged the same way (std/rt/cron.go, 300 lines, removed).
