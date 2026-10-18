# 0265: dbg on every backend, compiled out of release builds

Status: Accepted (owner, 2026-10-18; proposed by Xo Lang 2 from the
owner's language survey through the lead)

## Context

`dbg(expr)` (core 11.4) prints the expression's source and its `Debug`
value and returns the value. It needs no capability, so pure functions
may call it; `xo build --ci` rejects it and `xo vet` warns (XO0901).

It does not work where agents most need it. On the native backends with
`--no-gc`, the mode the compiler itself and every std/native module
build in, it is XO1201 ("not available without a garbage collector
yet"). On 2026-10-18 a pure SSA pass (compiler/arm64 reach.xo) kept
1,009 of 1,015 functions alive for a reason no test showed; to see its
counts the pass's return type was changed from Int to Str and the
driver printed the string. That is the general shape: without dbg, the
only way to see what pure code computed is to change its signature or
write a test that asserts on it.

`dbg` is also not stripped from release builds: `--release` keeps it,
and only `--ci` (a separate gate) refuses it.

## Decision

1. `dbg(expr)` works on every backend and in `--no-gc` builds. The
   native backends print the value through the same `Debug` formatting
   the Go backend uses (the derived structural printer of core 6), as
   generated code per type, with no heap beyond the line it builds and
   frees. Until a type's printer exists natively, its fields print as
   `<TypeName>` instead of XO1201, so dbg never blocks a build.
2. Output: one line on standard error, `[file.xo:LINE:COL] expr = value`
   (the Go backend's format since core 11.4 first had dbg),
   written with a single write so concurrent tasks do not interleave
   within a line. Standard error is the only channel: no capability, no
   configuration.
3. The capability exception is stated in core 11.4: dbg is the one
   output a function may produce without `Stdio`, because it is a
   debugging aid that a release build removes (point 4). Its effect is
   not part of a function's effect signature, so adding a dbg never
   changes a signature or a caller.
4. `xo build --release` compiles `dbg(expr)` to `expr`: the value is
   computed, nothing prints, and the binary carries no formatting code
   for it. `xo run` and `xo test` keep it (they are debug builds).
5. Guards against shipping it stay as they are and are documented
   together: `--ci` rejects a dbg (11.4), `xo vet` warns (XO0901), and a
   release build strips it, so a forgotten dbg can make a debug build
   noisy but never a release build.
6. Real-time functions keep rejecting dbg (XO1405): its write blocks.

## Consequences

- Pure code, the compiler, and std/native can print intermediate values
  without signature changes.
- The native backends need per-type Debug printers (the bulk of the
  work); the placeholder in point 1 lets the rest land first.
- A program whose behavior depends on dbg's output cannot exist: the
  output goes to stderr and disappears under --release.

## Alternatives

- A `trace` capability or `Stdio` for debugging: makes adding a print
  change signatures up the call chain, the friction this removes.
- A separate `dprint(Str)` that takes a formatted string: two ways to do
  one thing; dbg already returns its value, which fits expressions.
- Keep dbg Go-backend only and debug native code with `xo debug`: works
  for a stopped program, not for a pass that runs over a thousand
  functions.
