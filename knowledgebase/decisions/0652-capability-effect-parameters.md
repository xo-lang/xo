# 0652: capability interfaces take their effect as a parameter, so a pure capability value keeps its caller pure

Status: Accepted (owner, 2026-11-16, scoped: Rand and Clock first, Fs and Net sub-labels settled in their own step (open question 1); only std/native/llrt may build a pure Rand over the runtime source (open question 2); verify first that effect arguments are not part of the monomorphization or interface-table key; proposed by Xo Lang 1, 2026-11-15; the lead's language survey for the
owner, 2026-11-15). Draft only: the owner approves before any building.
Builds on 0004 (effects in signatures, authority from capability values),
0014 and 0432 (effect variables), 0540 (interface parameters' effect
variables) and 0582 (covariant interface effect parameters).

## Context

Authority in Xo comes from capability values rooted at `main(os: Os)`
(decision 0004), but the effect a capability performs is fixed by its
interface, not by the value. The prelude declares

```
interface Rand {
  fn int(self, lo: Int, hi: Int) -> Int uses rand
  fn bytes(self, n: Int) -> Bytes uses rand
  ...
}
```

so every call through a `Rand` performs `rand`, whichever value it holds.
A program's own Rand that answers fixed bytes, or a seeded generator with
no I/O, still makes its caller perform `rand`. A pub function that calls it
must declare `uses rand`, and so must every caller up to `main`.

That is wrong for sans-I/O code, which std writes on purpose: TLS
(decision 0202, the server's state machine is sans I/O), HPKE (decision
0646), ML-KEM and the protocol cores that tests drive with fixed inputs.
Such code needs randomness as an input, not as authority. Today it has
two ways out, and both cost something:

- It declares `uses rand`. A pure caller (a test vector replay, a
  deterministic simulation, a `pure` layer of an app) then gets XO0701 for
  calling it.
- It takes the random bytes as an argument and drops the Rand. The API
  then splits into a "draw" step and a "use" step that callers must
  pair correctly. The HPKE sender was rewritten this way after XO0701
  (Xo Lang 3, 2026-11-15; details below).

The same holds for `Clock` (a virtual clock in a simulation is pure but
types as `uses clock`), and in principle for `Env` and `Fs` over an
in-memory tree.

Case (Xo Lang 3, 2026-11-15, branch l3-httptls at 6bc785b68, commits
33b7d78d6 and ad37a2ada): TLS 1.3 Encrypted Client Hello seals the inner
ClientHello with HPKE inside the client's sans-I/O handshake
(std/native/llrt/tls13.xo:681). The natural call is std/hpke.xo:670
`pub fn new_sender(r: Rand, pk: PublicKey, kdf: Kdf, aead: Aead, info:
Bytes = b"") -> Sender ! HpkeErr uses rand`, then `Sender.seal`. From the
handshake, which declares no `uses rand` (it is pure by design and draws
from llrt's `rand.next_bytes`, as Go's crypto/rand does), that call is
XO0701. The workaround is std/native/llrt/tlsech.xo:528-640: a private
`Sender`, `sender(p: Picked)` and `seal`, RFC 9180's base mode written a
second time on the handshake's primitives (x25519, nistec, labeled
extract and expand over llrt hkdf). That is about 110 lines duplicating
std/hpke, kept in step by hand.

## Decision (proposed)

1. **Capability interfaces take an effect parameter.** `Rand`, `Clock`,
   `Env`, `Fs`, `Net`, `Stdio` and `Proc` become
   `interface Rand[fx: Effects] { fn int(self, lo: Int, hi: Int) -> Int uses fx ... }`.
   Each method's fixed label (`uses rand`, `uses fs.read`, ...) becomes
   `uses fx`. Where an interface has several labels today (Fs's
   `fs.read` and `fs.write`), a method whose label is narrower keeps it
   as a bound. See open question 1.
2. **The OS values keep their labels.** `os.rand` is a `Rand[rand]`,
   `os.clock` a `Clock[clock]`, `os.fs` an `Fs[fs]`, and so on, so the
   authority model and record and replay (which intercept the OS values)
   are unchanged. A test's seeded `t.rand` stays `Rand[rand]`; a later
   decision may make it pure.
3. **A pure implementation is a `Rand[()]`.** A struct whose methods
   perform no effects converts to `Rand[()]`. The checker already checks
   a method's effects against the interface method's at conversion
   (core 2.4). By decision 0582 the parameter is covariant (named only in
   `uses` clauses), so a `Rand[()]` is accepted wherever a `Rand[rand]`
   is.
4. **Omitted arguments keep existing code compiling.** By core 6.2, a
   parameter typed `Rand` with the argument omitted gets an implicit
   effect variable, and the function may perform it without naming it.
   So `pub fn seal(r: Rand, ...) uses rand` still checks. Its `uses rand`
   becomes a declared-more note from `xo vet` (allowed, core 6.2), and
   `xo fix` can drop it. A caller passing `os.rand` instantiates the
   variable to `rand` and must declare `rand`, as today. A caller passing
   a `Rand[()]` performs nothing. In a struct field, an omitted argument
   becomes an implicit effect parameter of the struct (core 6.2), so a
   sans-I/O `Sender { r: Rand, ... }` is pure when built with a pure Rand.
5. **No new syntax.** Everything here is effect parameters on
   interfaces, implicit effect variables and covariance, all of which
   exist. The change is to the prelude's declarations and to the
   checker's treatment of the builtin capability types, which are
   special-cased today.

## Open questions

1. Fs and Net have sub-labels (`fs.read`, `fs.write`, `net.dial`,
   `net.listen`). Per-method bounds (`fn read(self, ...) uses fx & fs.read`)
   or one parameter per sub-label? Recommended: start with Rand and
   Clock, which have one label each, and settle this in the Fs step.
2. The runtime's own randomness (raised by Xo Lang 3). llrt's
   `rand.next_bytes` reads the OS generator with no effect today. llrt is
   the trusted runtime and sits outside the effect system, as the
   allocator and scheduler do. A `Rand[()]` built over it would be honest
   only under that exemption. Recommended: say so explicitly. Only
   std/native/llrt may construct a Rand over the runtime source (a
   llrt-private constructor, not exported to std or user code), as llrt's
   TLS handshake already draws from it unannounced. Std and user code
   wanting pure randomness use `rand.fixed` or `rand.seeded`, or take a
   `Rand[fx]` and let the caller choose. The alternative, making the
   runtime source a `Rand[rand]`, is more honest about entropy, but every
   TLS handshake would then perform `rand`, and every `Conn.handshake`
   caller up to `main` would declare it. That is a large breaking change
   for no authority gained: the runtime reads entropy whatever the code
   says.

## Checker rules

- The builtin capability interfaces are ordinary interfaces with one
  covariant effect parameter. The special case that ties `Rand` to the
  `rand` label goes. The label survives only on the OS values' types and
  in `uses` clauses.
- XO0701's wording gains a hint when the missing effect comes from a
  capability parameter's implicit variable: "`r` is a `Rand` whose
  effects are the caller's: pass a `Rand[()]` to keep this pure."
- Effect variables from capability parameters join the existing
  inference, and 0432's union applies. An explicit `Rand[rand]` in a
  signature stays exact where the position is not covariant (a field of
  a struct that is itself an invariant position).
- `--strict-effects` (if added) could later require the argument to be
  written on pub APIs. Not part of this decision.

## Codegen cost

None expected. Effects are erased before SSA: interface dispatch, method
tables and closure layout do not depend on effect arguments.
To verify before building (step 1 below): effect arguments must not be
part of the monomorphization or interface-table key. If they were, each
distinct `Rand[fx]` would instantiate generic code again, growing code
size and build time. If they are, the fix is to erase them from the key.
Check time: a few more effect variables per signature that takes a
capability, which the inference already handles in bulk (0432).

## Std migration

- prelude.xo: seven interfaces gain `[fx: Effects]` and their methods
  `uses fx`. OS value types name their label.
- std/crypto, hpke, tls (tls13, tlsio), mlkem, jwt, uuid, random: their
  `uses rand` on functions that take a `Rand` become redundant. `xo fix`
  removes them, and the change logs (0276) record "now pure with a pure
  Rand". HPKE's sender can take a `Rand` again (undoing the split), as a
  separate std change after this one.
- A std helper, `rand.fixed(bytes: Bytes) -> Rand[()]` (answers the given
  bytes, then faults) and `rand.seeded(seed: Int) -> Rand[()]` (a
  ChaCha8 stream like Go's math/rand/v2 ChaCha8), so tests and
  simulations need no hand-written Rand. Names to settle in the std
  change.
- Clock: `clock.virtual(start: Time) -> Clock[()]` for sans-I/O timers,
  after the Rand step proves the pattern.
- Breaking: none for code that omits the argument. Code that wrote
  `Rand` in a position where an implicit variable is not allowed (a
  `List[Rand]` field of a pub struct) gets the struct's implicit effect
  parameter. The std audit lists such sites first (`xo refs Rand`).

## Plan (if approved)

1. Measure: `xo refs Rand`, `Clock`, `Fs` across std and bench/paper for
   positions that change meaning. Check the mono key question above
   (compiler/check, the instantiation key).
2. Rand only: the prelude change, the checker's special case removed,
   XO0701's hint, goldens for a `Rand[()]` sans-I/O caller (pure caller
   compiles) and a `Rand[rand]` caller (still needs `uses rand`),
   `rand.fixed` and `rand.seeded`, spec core 6.2 and std 3.4.
3. Clock, then Env and Fs, each its own push with its own goldens.
4. Std cleanup: `xo fix` over std, HPKE's sender back to taking a Rand.

## Consequences

- Sans-I/O code takes its capabilities as values and stays pure when given
  pure ones, which is how the code is already written. Only the types
  disagreed.
- The audit story is unchanged. Authority still flows only from `main`'s
  `Os`, and a `uses rand` in a signature still means "may touch the OS
  generator" whenever the value can be the OS one.
- Record and replay see no difference: they intercept the OS values, and
  pure values need no recording (they are deterministic by construction).
- One more concept in error messages (a capability's effect argument).
  XO0701's hint names it.

## Alternatives

- **Effect handlers** (Koka, Eff, OCaml 5's effects): `handle rand with
  fixed(b) { ... }` reinterprets an effect for a dynamic extent. This is
  more general (a handler can resume, abort, or log), but it needs
  runtime support (a handler stack lookup per operation, or one-shot
  continuations for resumption), and it moves authority from values to
  dynamic scope, which 0004 rejected for auditability. It is the right
  tool for user-defined effects. Xo has no user-defined effects, so it
  costs more than it buys here.
- **Capability passing alone** (E, Wyvern, Austral; Scala 3's capture
  checking `{r} Rand`): Xo already passes capabilities. What it lacks is
  tying the effect to the value, which is what Scala's capture sets do.
  This decision is the same idea in Xo's existing effect-variable
  machinery.
- **The bytes-as-argument workaround** (today's HPKE): no language
  change, but every sans-I/O API splits into draw and use steps, and the
  pairing is the caller's to get right.
- **A separate pure interface** (`PureRand` with no `uses`): the API
  doubles, and code generic over both needs an effect variable anyway.
- **An `assume pure` escape hatch**: it hides authority and breaks the
  audit property that is the point of 0004.

## Related items from the same survey (Xo Lang 1)

These are not decided here. They are recorded so the owner sees them
together.

- **Closures that mutate captured variables (XO0502).** Closures
  capture by value, and mutating a captured `var` is XO0502 (core 3.3).
  The documented way is a `Mutex` or `Atomic` handle, which is heavy for
  a single-threaded counter or accumulator. This week's case: the
  regression golden internal/llvm/testdata/golden/inline_immortal_depth
  (2026-11-15) had to be restructured to avoid an accumulating closure.
  Options for a separate ADR:
  - (a) Capture by reference for closures that cannot escape: a closure
    passed directly to a parameter the callee only calls during the call
    (List.each, map, sort_by), checked by an escape rule like Swift's
    non-escaping closures.
  - (b) A prelude `Cell[T]` (`get`/`set`, unique, not shareable across
    tasks) as the documented idiom, cheaper than a Mutex.

  (b) is small and could come first. (a) removes the friction for the
  common loop-callback case.
- **An inline verifier that explains its failures** (tooling, no ADR).
  When verification fails after inlining, the warning "built without
  inlining" names no value, block or fact. The 2026-11-15 imm_at depth
  bug (gotchas/inline-verify-immortal-depth.md) took hours to reduce
  because of that. The warning should name the function, the failing
  fact (held on one path at a join), the value and its source line, and
  print the path depth reached. Xo Lang 1 adds this the next time it
  works in compiler/ssa/rc.xo.
