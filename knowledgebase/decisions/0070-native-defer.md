# 0070: defer on the native backends: inline at every exit, shielded by the runtime

Status: Accepted (2026-10-08). Implemented in `internal/ssa/build_defer.go`
and `xo_shield_begin`/`xo_shield_end` in `std/llrt/xo_rt.c`. Goldens:
`internal/llvm/testdata/golden/defer_blocks`, `conc_defer`.

## Context
`defer` (core 3.5, decisions 0009 and 0022) was a build error on the
native backends (decision 0068 listed it). The Go backend runs a block
with `defer` as a closure and uses Go's defer and panics; the native
backends have no unwinding tables, and cancellation unwinds through
explicit checks (decision 0068).

## Decision
1. **Inline lowering.** The SSA builder keeps a stack of open blocks
   (with the `defer` statements met so far) and scopes. Every way out
   lowers the pending deferred statements again at that point, last
   registered first: the end of a block (after its value is computed),
   `return` and `?` (every open block of the function), `break` and
   `continue` (the blocks inside the loop), with the scopes in between
   closed at their place in the nesting (a scope body's deferred code
   runs before the scope waits for its tasks). Variables are read where
   the deferred code is lowered, so it sees their values at exit.
2. **Unwinding** runs the deferred code of the blocks inside the
   innermost scope and jumps to that scope's unwind block, which closes
   the scope and goes on outward (each unwind check gets its own path
   when deferred code is pending; checks after calls that cannot unwind
   are removed with their paths, decision 0068).
3. **Shielding in the runtime.** Each deferred statement runs between
   `rt.shield_begin` (returns a token: start time and the unwinding flag;
   clears the flag; raises the context's shield count) and
   `rt.shield_end` (restores both; XO-F005 when it ran longer than 5 s,
   checked at the end as on the Go backend). While shielded, cancellation
   points do not report cancellation and waits are not canceled; the
   builder emits no unwinding checks in deferred code. Programs built
   without unwinding checks (nothing can cancel them) skip the calls.
4. **Faults do not run deferred code natively.** A fault exits the
   process at once (as a fault in a task already did, decision 0068).
   Running it would need unwinding through every frame on every
   faulting operation.

## Consequences
- `defer_blocks` (internal/gen's golden without its fault) and
  `conc_defer` (cancellation through functions and nested scopes, a
  shielded send in a canceled task) print the same on the Go, LLVM, and
  arm64 backends, with no leaks, under ThreadSanitizer.
- Deferred code is duplicated at each exit (code size grows with the
  number of exits times the deferred code).
- Core 3.5 notes the native difference on faults.

## Alternatives
- **A runtime defer stack of closures**: one copy of the code, but
  closures capture by value at registration, so deferred code would not
  see later assignments (core 3.5 reads values when it runs) without
  boxing every variable it reads.
- **setjmp/longjmp to run deferred code on faults**: the values live in
  the skipped frames are not reachable from the landing point, so the
  deferred code could not read them.
