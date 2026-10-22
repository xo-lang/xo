# 0342: Compiled pieces record what they assumed, and the build checks it

Status: Proposed (owner approved the tool, 2026-10-22; Xo Publish).

## Context

Two failures on 2026-10-22 came from pieces of one build that disagreed
about a type:

- **maph naming.** cmd/xo TestNoGCTests failed. A cached front end body
  of http.fields_in called `xo.maph.logget[Map[Str, Str]]` as returning
  `Str?`. This build had defined that name as the `Bytes?` helper,
  because the helper's name depended on creation order
  (gotchas/helper-name-creation-order.md, fixed in 3b8975312).
  - LLVM's IR verifier rejected the module.
  - The native path only warned: "the inlined arm64 module does not
    verify ... built without inlining". It then built and ran the
    program, with the mismatch still in it.
- **0.25.1's windows/amd64 xo.exe** faulted on every build (CI
  39800969338): an access violation in `write_dumps(dumps=NULL)`.
  - The Program returned by generate_cached had wrong words.
  - Xo lang 4's leading hypothesis is that functions in that one binary
    disagree on G's or Program's layout because of a stale cached piece.
    That is not confirmed yet.

What the caches already guarantee (from a read of the keys,
2026-10-22):

- **fn key** (compiler/arm64/fcache.xo fn_key) hashes:
  - the function's SSA, with every type written as its `ssa.type_keys`
    entry;
  - `func_sig` of the function and of every callee it names: results,
    parameters, captures and flags;
  - the target;
  - the cache id, which is the codegen key.
- **Type keys** (ssa/rccache.xo type_keys, TypeHasher over type_desc)
  hash a type with everything it reaches. Every backend layout (word
  classes, field offsets, element sizes) follows from type_desc, so a
  type key already stands for a layout.
- **rc and inl keys** carry the type keys. Part keys combine fn keys.
  Object keys are the assembly text itself.

So a stale hit for a changed struct or a changed callee signature needs
one of three things: a key that leaves an input out, a hash collision,
or a piece whose key is not built from the current module. The front end
cache (`fe`, keyed by module sources and flags) is the third kind.

- An fe body names helpers ("wants") by name. Those helpers are
  generated fresh and resolved by name (fcache.xo resolve_wants).
- So the body's own call types are what it assumed, not what this build
  defines.
- `XO_BACKEND_CACHE=verify` regenerates every entry and would see a
  difference. It is a test mode, not something every build runs.

A link-time check of struct-layout hashes alone would not have caught
the maph case: every type key was right, and only the call's result
type disagreed with the callee. It would catch a key omission only if
its hash came from somewhere other than the keys. Hashing the same type
keys again would agree with whatever the cache key said.

## Decision

Every function the native backends compile records its **assumptions**,
taken from its own code, not from the current module:

- for each symbol it calls (callee or helper), the result and parameter
  type keys at the call site;
- for each struct or enum whose fields it reads, writes or builds, that
  type's key in the function's own SSA.

Every definition gives its own signature and its own type keys.

The check runs once per build, in the driver, after every function is
generated or decoded and before assembly (compiler/arm64 parallel.xo
gen_part's caller):

- Each assumption is looked up against the definition in this build.
- A mismatch is a build error, XO1510 (new): the function, the symbol or
  type, and what each side has. A mismatch means a cached piece is
  stale, so there is no fallback.
- The fn and part cache entries store the assumptions next to the code.
  A hit is checked like a fresh function.

The pieces of the check:

- **Where the hashes come from.** They are the existing `ssa.type_keys`
  values (name#hash) and `func_sig`, both already computed for the fn
  keys of every function. The check reads the caller's own call and
  field instructions. That is what makes it independent of the keys: a
  key that omitted an input would still hit, but the hit's recorded
  assumptions would not match the fresh definition.
- **Cost.**
  - One string per reference in each fn entry. xo has about 10,000
    functions with, say, 10 to 20 references each: under 200,000 short
    strings and a Map lookup each.
  - That is measured in milliseconds against a 113 s cold build, or a
    warm build's several seconds. It will be measured with `xo bench
    --ab` (instructions and wall time, warm and cold) before the check is
    on by default.
  - The entries grow by the same strings: a few percent of the 328 MB
    of part text.
- **Cache keys.** The assumptions are data in the entry, not inputs to
  any key. Keys stay as they are. FN_CACHE_VERSION and the part format
  version go up once, because old entries carry no assumptions.
- **The inline verifier.** A failed inline verification becomes an error
  in the release and gate builds (XO_INLINE_STRICT=1 in scripts/gate.sh,
  release.sh and the hourly check). It stays a warning in ordinary
  builds until the check above lands. The verifier sees the same
  disagreement one stage earlier, for inlined code only.
- **LLVM.** clang's IR verifier already rejects a call whose types
  disagree with the callee. The check covers the native arm64 and amd64
  backends, which have no such verifier.

## Consequences

- Each failure class above stops the build with a precise message
  instead of producing a binary that faults later. For the maph case the
  message would have named http.fields_in, the helper, and `Str?`
  against `Bytes?`.
- Whether this would have caught the Windows xo.exe depends on its cause.
  - If two functions disagree on G's or Program's type key (a stale
    piece), yes.
  - If the cause is in code generation for one target with matching
    keys, no. Then the fix lies in the generator, and TestWindowsAmd64Identity
    and the Docker amd64 goldens are the guard.
  - This ADR does not claim the first case before Xo lang 4's triage
    reports.
- One new code, XO1510 (after XO1501 to XO1509, the build codes), goes
  into spec section 10.
- The check runs in-process before assembly, so a build that links with
  cc is covered too. Objects and the linkers gain no metadata section.

## Alternatives

- **Hash every struct layout into each object and compare at link
  time.** This is what was first asked for. It does not catch a call
  that disagrees on a result type when all type keys agree, which is the
  maph case. Its hashes would come from the same type keys as the cache
  keys, so a key omission would pass both. It would also need a metadata
  section in the Mach-O, ELF and PE linkers and in cc links.
- **Run `XO_BACKEND_CACHE=verify` in every build.** That regenerates
  every function, so a warm build loses the cache. It is kept for tests.
- **Add the helper names' resolved types to the fe key.** That fixes this
  instance only, and still trusts every other key to be complete.

## Plan

1. Assumption records: fresh functions and fn entries (compiler/arm64
   fcache.xo, parallel.xo), the check in the driver, and XO1510. Test: a
   fn entry edited by hand to assume another result type fails the build
   with XO1510. Measure with `xo bench --ab` (cold and warm xo builds).
2. Parts: decode_part carries the records.
3. amd64: the x86 path shares parallel.xo. Run the Docker amd64 sample.
4. XO_INLINE_STRICT=1 in gate.sh, release.sh and hourly.sh.
