# 0002: P1 prototype transpiles to Go

Status: Accepted (2026-10-07)

## Context
The language design (types, effects, diagnostics) must be validated quickly.
Writing a backend, GC, and scheduler first would delay that by months.

## Decision
The P1 compiler is written in Go and emits Go source, then invokes `go build`.
Generated code carries `//line` directives back to `.xo` sources.

## Consequences
- Free: GC, goroutine scheduler, cross compilation to every Go target, Delve, pprof.
- Semantics Go does not have must be implemented in the emitted code or the
  runtime package: checked arithmetic, sum types, persistent collections,
  insertion ordered maps, error frames, scopes.
- Deterministic scheduling and full record/replay wait for P2 (own runtime).

## Alternatives
- LLVM / Cranelift / QBE first: slower to first working program.
- Interpreter first: does not exercise the cross compilation story.
