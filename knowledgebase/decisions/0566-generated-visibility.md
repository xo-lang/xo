# 0566: @generated visibility for derive helpers

Status: Accepted (owner, 2026-11-08, on the lead's recommendation). Proposed (Xo Lang 2, 2026-11-08; the owner decides: it narrows
what user code may call in std, and adds a diagnostic)

## Context

A derive (decisions 0315, 0512, 0527, 0559) expands into ordinary Xo
functions, parsed into the user's module as a file of their own
(compiler/check derive.xo), and those functions call helpers in the std
module: `cbor.unm_new`, `cbor.put_text`, `cbor.det_keys`, and yaml's
readers. std's declaration files write no `pub`, and the checker makes
every std declaration public (decision 0440's context), so each helper
is public API in all but name:

- User code may call it, and nothing says it may change: std/cbor.xo
  marks them only with a comment ("The helpers of the generated code",
  "The direct writers of the generated code").
- `xo doc std/cbor` lists them beside `marshal` and `unmarshal`
  (`unm_new` is in its index), as do `xo symbols` and completion.
- spec/std.md does not list them, so the spec and the surface
  disagree; decision 0559 planned to list the writers in std.md as the
  generated code's helpers, which would make them documented API.
- TestStdAPICovered (internal/repotest stdcoverage_test.go) cannot see
  a call from generated code, so internal/stdcoverage/allow.txt carries
  20 "called only by derive ... generated code" lines: 19 cbor names and
  the wildcard `yaml.*`, which exempts every yaml declaration, the
  public API included, from the coverage check.

Each new derive (Proto's builder forms in std/protobuf.xo are the next)
adds more.

## Decision (proposed)

1. A doc comment line `@generated` on a std function or method keeps it
   to code a derive generated and to std's own modules. A call from any
   other user code is a new diagnostic (an error, `XO` code assigned
   when it is built, in spec section 10): "`cbor.unm_new` is a helper of
   derive Cbor's generated code, not API". It is the prelude's encoding
   rule beside `@internal` (decision 0440) and `@move_only`. A new code,
   not a wider XO0605, so XO0605's meaning stays as decided.
2. The checker knows a call's origin: the derive file is synthetic
   (Checker gets a flag per file as it parses the expansion), so the
   rule is one check in call resolution, as `std_internal` is in
   compiler/check decl.xo.
3. `xo doc`, `xo symbols` and completion leave `@generated` declarations
   out unless `--all` is given; spec/std.md keeps not listing them, and
   says in its encoding rules that `@generated` helpers are not API.
4. Coverage: stdcoverage skips `@generated` declarations, and a new
   check requires each to be named in a derive generator's output
   (compiler/check derive.xo's emitted text), so a helper no generator
   calls fails as dead. The 19 named allow lines and the `yaml.*`
   wildcard go, and yaml's public API comes back under the coverage
   check (whatever it then reports is covered or allowed by name).
5. Migration: std/cbor.xo's and std/yaml.xo's helpers gain the line;
   no user code in the repository calls them (to be confirmed with
   `xo refs` over the corpora and examples when built).

## Consequences

- BREAKING only for user code that calls a helper directly, which the
  spec never offered. The error names the public function to use where
  there is one (`cbor.marshal` for `put_*`).
- Helpers may change with their generator, in the same commit, without
  a std change log entry for an API change.
- One more doc marker for std authors to know; std.md's encoding rules
  list it.

## Alternatives

- Keep the comments (status quo): the allowlist and the `yaml.*` hole
  stay, and each derive widens std's de facto API.
- Move the helpers into a hidden module (`std/internal/cborgen`) that
  generated code imports: the generated file needs an import user code
  could also write, so the module needs the same "generated only" rule,
  and the helpers lose access to the module's private functions.
- Inline the helpers into each generated function: larger output per
  type and slower builds; decision 0559 moved the other way, to shared
  writers.
- `pub(generated)` as syntax: a grammar change for a std only need; the
  doc marker follows 0440's precedent and can become syntax later.
