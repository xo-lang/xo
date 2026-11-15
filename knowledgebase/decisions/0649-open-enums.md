# 0649: Open enums: a marker under which code outside the package must have a wildcard arm

Status: Accepted (owner, 2026-11-15, through the lead: in 0.52 with XO0306 at warning level, an error in the release after). Drafted by Lang 3 at the lead's request. Build order (seed rule): the parser and checker land first; the `open` keyword stays out of compiler/, std/native and the llrt files the seed imports until Lang 2 cuts a seed that parses it; enums in those files are marked open in a second step after that seed.

## Context

Decision 0567's addendum makes a new variant of any `pub` std enum a breaking change, because user code may match it
exhaustively and XO0301 then fails the build. It deferred to 1.0 "whether such enums are declared open (a required `_ =>`
arm outside std) so that adding a target stops being a break". Three breaks since have shown the cost of waiting:

- `TargetArch.Riscv64` (0571) and `TargetOs.FreeBSD` (0573) broke four goldens, two bench programs and three frozen blind
  solutions (gotchas/std-enum-variant-breaks-exhaustive-matches.md).
- `ClientErr.Tls` (0644, 0.52) broke two published SDKs (xo-lang/aws transport, three matches; xo-lang/gcp storage, one),
  which had to ship wildcard arms before the std release so they build on 0.51 and 0.52 alike.

Today the declaring module has no way to say "this set will grow". Spec 3.5 already tells users that enums of other modules
may use `_` "since their owners can add variants", and XO0305 (a warning) only fires on a `_` that hides variants of an enum
of the same module, but nothing makes the outside `_` required, so the safe style is advice, not a rule. Error enums are
the main case: every std module that talks to a peer (http, net, tls, sql, grpc, amqp) learns new failure kinds over time.

## Decision

### 1. Syntax

A contextual modifier `open` before `enum`:

```
/// What can go wrong with a client request.
pub open enum ClientErr {
  Connect(msg: Str)
  Timeout
  ...
  Tls(err: tls.TlsErr)
}
```

- Grammar (spec 15): `EnumDecl = [ "open" ] "enum" Ident ...`. `open` is contextual: it is a keyword only directly before
  `enum`, so no existing identifier named `open` (std/fs's `open`, user functions) changes meaning.
- `xo fmt` keeps it; `xo doc` and `xo def` show it in the declaration line; the LSP hover shows it.
- It is part of the language, not a std doc marker (`@move_only`, `@internal` are read only in std files): SDK packages
  (aws, gcp, azure) have the same problem with their own error enums and must be able to declare it.

### 2. Rule

For a match whose scrutinee's type is an open enum (directly, or as a field of a tuple pattern or nested pattern):

- **Inside the declaring package** (the xo.mod module that declares the enum; for std, all of std, which is built and
  released together) nothing changes: matches may be exhaustive, and XO0305 still warns on a `_` that hides variants.
- **Outside the declaring package** the match must have an arm that matches any value: `_`, a bare binding (`e =>`), or
  a variant-less catch all. Listing every variant is not enough.
- Expressions that are not matches (constructors, `==`, `is` tests with a fallback, `if let`) are unaffected.
- A value enum's catch-all variant (decision 0493, `Unknown(value: Str)`) already covers unknown wire values at decode
  time; such an enum may also be `open` so that adding a named variant later is not breaking.
- Code generation is unchanged on every backend: the checker requires the arm, and the arm compiles like any `_` arm.

### 3. Checker codes (new; no existing code changes meaning)

- **XO0306** (error): "match on open enum `http.ClientErr` (declared in std) needs a `_ =>` arm: its owner may add
  variants". Span: the match keyword; the note lists the variants the match already handles, and the fix hint is the arm
  text `_ => ...`. During the migration release (section 5) it is reported as a warning with the same code.
- **XO0307** (error): `open` on an enum that is not `pub` (the rule can never apply), or `open` written anywhere other
  than directly before `enum`.
- **XO0308** (warning, `xo vet --open-enums` only, off by default): a `_` arm on an open enum outside the package hides
  a variant the other arms do not name. It is Swift's `@unknown default` warning: code that wants to handle every known
  variant explicitly learns of a new one on upgrade, as a warning rather than a failed build. Optional; it can be
  dropped without touching the rest.

XO0301 keeps its meaning (a non-exhaustive match). On an open enum outside the package, XO0306 is reported instead of
XO0301 when the only missing arm is the wildcard.

### 4. Which std enums opt in

Criterion: open when std may add a variant without changing what the existing variants mean; closed when the set is
fixed by definition or by a format that versions itself.

- **Open:** the error enums, which grow as peers and platforms do:
  - prelude: `NetErr`, `FsErr`, `IoErr`, `ProcErr`, `EnvErr`, `ClockErr`, `KeyErr`;
  - std/http: `ClientErr`, `HttpErr`; std/tls `TlsErr`'s kind enum; std/x509's error kinds;
  - the module Err enums: `AmqpErr`, `Asn1Err`, `BcryptErr`, `BufErr`, `CborErr`, `CliErr`, grpc `FrameErr`, jsonrpc
    `FrameErr` and `CallErr`, `JpegErr`, `JwtErr`, otlp `ExportErr`, `PngErr`, `ProtoErr`, `RateErr`, `SseErr`,
    `SqlErr`, time `ZoneErr` and `CivilErr`, toml `DecodeErr`, `TextErr`, and the rest a sweep finds
    (`xo symbols --all --dir=std`, every `enum .*Err`);
  - platform lists: `TargetOs`, `TargetArch`.
- **Closed:** `Ordering`, `Option`/`Result`-like sums, `Weekday`, `Month`, std/json's value kinds, log `Level`, and
  protocol enums whose set a version number fixes (TLS versions stay closed only if std adds versions as breaking).
- The sweep is a table in this ADR's implementation commit, one line per std enum with open or closed and why, so the
  choice is reviewed once rather than per module.

### 5. Migration (0567's rules: deprecate first, diagnostic names the fix)

Marking an existing enum `open` breaks outside code that matches it exhaustively without `_`. That is a one-time break per
enum, done once for all of std:

1. **0.N (introduction):** the `open` modifier, the std sweep, and XO0306 as a **warning**. CHANGELOG **Deprecated** line:
   "exhaustive matches on std error enums need a `_ =>` arm from 0.N+1 (XO0306)". `xo fix` adds no arm, since the body
   is the author's choice; the warning names the enum and the arm to write, which 0567 accepts as the named replacement.
   The SDK repos (aws, gcp, azure) and examples are swept in the same week; aws and gcp already have wildcards since
   aws#6 and gcp#1.
2. **0.N+1:** XO0306 becomes an error. CHANGELOG **Breaking** line with the same migration text. Spec "Changes in 0.N+1"
   lists it.
3. After that, a new variant of an open enum is an ordinary addition: CHANGELOG **Added**, any release that may add API,
   no Breaking line. A new variant of a closed `pub` enum stays breaking, as now.
4. Std's own goldens, bench programs and site snippets are fixed in the introduction release. `bench/paper/` blind
   solutions are frozen and are not edited: their runs pin the toolchain they were written for, or record the failure as
   the expected cost of the rule (a note in bench/paper's README).

### 6. Effect on 0567's compatibility promise

- 0567 addendum rule 2 becomes: "a new variant of a **closed** `pub` std enum is a breaking change ...; a new variant of
  an **open** enum is not". Its deferred 1.0 question is answered now instead of at 1.0.
- New breaking kinds, listed in 0567: making a published enum `open` (outside matches without `_` stop compiling);
  removing `open` is not breaking (the outside `_` arms only become possibly unreachable, which is no diagnostic outside
  the package).
- At 1.0 the set of open std enums is part of the frozen promise, the same as the public API: std may add variants to
  them in any minor, and may never close one without a Breaking line in a major.
- The policy covers packages too: the guidance for package authors (spec 13 and the site's compatibility page) says to
  declare error enums `open` from their first release.

## Consequences

- Adding `ClientErr.Tls`-like variants stops breaking SDKs and user code; std error enums can grow every minor.
- Outside code loses the compiler's help for spotting a new variant it should handle; XO0308 (`xo vet`) gives that back
  for those who opt in, as Swift's `@unknown default` warning does.
- Spec: core 3.5 (the rule), 10 (XO0306 to XO0308 in the code list, TestSpecListsEveryCode), 13 (the policy line), 15 (the
  grammar). std.md: the open enums are marked in their declarations.
- Checker work is small: compiler/check/exhaust.xo already knows the declaring module (XO0305); the package boundary is
  the module's xo.mod root, which the checker has for `pub` visibility. The parser gains one contextual keyword; the Go
  backend's generated code and the native backends are unchanged.
- `xo explain XO0306` and the LSP quick fix that inserts `_ => todo()`-style text (the body left for the author) are
  part of the build.

## Alternatives

- **Open by default, `@frozen` to close (Swift's library evolution mode, in reverse).** Most future-proof, but it would
  break every exhaustive match on every user-visible enum at once, including closed sets like `Ordering`, and remove the
  exhaustiveness check most code wants. Rejected.
- **Rust's `#[non_exhaustive]`** (RFC 2008): this ADR's rule, with an attribute. Xo has no attribute syntax; doc markers
  (`@open` in a doc comment) are std only and are comment text in user code, and SDKs need the rule. A doc marker would
  still be the cheapest first step if the owner prefers std only: `/// @open` read by decl.xo like `@move_only`, with the
  same codes, and the `open` keyword later for packages.
- **Keep 0567 as is (every new variant breaking, CHANGELOG line and migration).** Works, but every release with a new
  error kind breaks downstream builds, and SDK authors must pre-emptively write `_` arms with no rule telling them to.
- **Add new failure kinds only as a generic catch-all variant (`Other(msg: Str)`).** Avoids breaks but loses the typed
  detail that motivated `ClientErr.Tls` (owner decision, option 2 in 0644).

## Follow-ups (small, from the same survey; independent of the above)

- **`Bytes.from_list(List[Int]) -> Bytes`** (spec/std.md 2.2 has `from_hex` and `from_base64` only). Byte-level code
  (llrt tests, the rsa OAEP block builder, ECH framing) builds single bytes through `rt.rs_push_byte`; a prelude
  constructor (each element masked to 0..255, or `XO`-coded panic on out-of-range, to be decided with the spec text)
  removes that. A std change: std/changes entry and spec std.md line.
- **An llrt `_test.xo` cannot `use` the runtime module it tests.** Recorded 2026-11-15 (xo at fd6e4c63c): adding
  `use native/llrt/rsa` to std/native/llrt/rsa_test.xo and running `xo test --std-module=native/llrt/rsa` reports
  `rsa_test.xo:26:1: error XO0604: import cycle: `native/llrt/rsa` imports `native/llrt/rsa`, which imports it back`.
  The test file is already part of rsa's module and calls its names bare, so nothing is blocked, but the message reads
  as a real cycle and sent me to a differently named file (tlssign_test.xo). Fix: a new code (XO0604 keeps its meaning)
  for a module that imports itself, with the note "a `_test.xo` is part of the module it tests: drop the `use` and call
  the names bare"; optionally, later, Go's external test form (a test that sees only the module's `pub` API).
