# New syntax cannot be used in compiler/ or std/native until a seed has it

Status: Anticipated (2026-10-21, decision 0321).

## Symptom

`x ?? return v` (0321) parses with this tree's xo, but `go run
./scripts/xobuild` builds compiler/xo with an earlier xo (a seed or the
previous bin/xo), which reports `XO0160` on the first use in compiler/ or
the std/native files the compiler imports.

## Cause

The compiler builds itself in stages: the xo that compiles compiler/xo
is the one before the change. Syntax the change adds is unknown to it.

## Fix

Do not use new syntax in compiler/ or in std/native files the compiler
imports in the change that adds it. Use it after the next seed cut (core
toolchain.md "Cutting a seed") contains it; xo vet XO0906 then lists the
four line matches to rewrite. Programs, tests, and std files outside the
compiler's imports may use it at once (their xo is the new one).
