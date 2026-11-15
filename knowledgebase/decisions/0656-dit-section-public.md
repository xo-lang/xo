# 0656: crypto.with_data_independent_timing, a public DIT section

Status: Accepted (owner, 2026-11-15, through the lead: option B of
~/xo-lead/notes/aws-43/decision-dit-public.md). Amends 0619. Drafted by
aws-43. Built in aws-43's closure crypto batch.

## Context

Go 1.24's crypto/subtle.WithDataIndependentTiming(f) runs f with Arm's
DIT (PSTATE.DIT) set on its locked OS thread. Decision 0619 sets DIT
around std's own constant time code and gives no public form, so
scripts/stdaudit/map.txt kept `subtle.WithDataIndependentTiming = gap`,
std/crypto's last open audit gap after the C1 batch. 0619 point 4: DIT is
per OS thread and a fiber switch does not save it, so a section must not
park or yield; XO_LOCKCHECK=1 stops a program that parks with DIT held
(`xo: DIT held`, status 70).

## Decision

1. `fn with_data_independent_timing[T](f: fn() -> T) -> T` in std/crypto
   (Go's subtle.WithDataIndependentTiming): DIT on, f, DIT back to what it
   was (a nested section leaves it set, as Go's). It returns f's value.
2. Compile-time rule, new error **XO0704** (spec core section 10, the
   effects family): the call is an error when f's effects can park or
   yield: any of net, fs, clock (sleep and timers), spawn, and a body that
   uses Chan, Mutex, Atomic waits or task await. f with no effects, or only
   pure computation, is accepted. The rule reads the closure's inferred
   effects and the blocking prelude operations it calls, as the checker
   already computes them for `uses`.
3. Runtime backstop: XO_LOCKCHECK=1's `xo: DIT held` (0619) also covers a
   section that parks through a path the checker cannot see (a callee
   through an interface or function value whose effects it does not know).
4. Targets: arm64 (native and LLVM) set and clear DIT as 0619's dit_on and
   dit_off do; on amd64, riscv64 and wasm the section runs f with no mode
   change (0619 point 5), and spec/std.md says so.

## Consequences

- map.txt maps subtle.WithDataIndependentTiming to
  std/crypto.with_data_independent_timing; std/crypto has no open gap.
- Goldens on arm64 and llvm: the value returned, nesting, DIT observed set
  inside and restored after (darwin and linux arm64 read PSTATE.DIT), and an
  XO0704 error test per blocking effect.

## Alternatives

- A, not planned: no API. The gap would have stayed open.
- C, runtime pinning so a section may block: scheduler work on hot paths,
  for a use no known user needs.
