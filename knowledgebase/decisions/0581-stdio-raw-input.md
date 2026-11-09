# 0581: Stdio.raw_input, raw terminal input

Status: Proposed (Xo Lang 2, 2026-11-09; the owner chose option A the same day, through the lead).

## Context

The REPL's line editing (decision 0572 stage 2) needs keys one at a time from a terminal; std has only echo off
(`Stdio.read_secret`). To be filled in.

## Decision

`Stdio.raw_input(on: Bool) -> Bool` beside llrt's echo primitive. To be filled in.

## Consequences

To be filled in.

## Alternatives

A raw mode private to the REPL; no line editing. To be filled in.
