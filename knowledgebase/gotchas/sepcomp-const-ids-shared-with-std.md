# A separately compiled program's constants took std's slots

Status: Observed (2026-11-11, Xo Lang 2, from mime's `--std-module` test)

## Symptom

With std from its store (separate compilation, the default for a native
build or run since v0.34.0), a program with a few constants of collection
types that called `mime.detect_content_type` faulted:

    fatal error: SIGTRAP: trace trap (code 0)
      mime.sig_hit (mime.xo:683) [inlined]

with 5 constants on 0.48.0 and 150 on 0.44.0 and 0.46.0 (where std's
numbers fell elsewhere). Reading std's constants first and the program's
after gave SIGSEGV at address 0. `xo test --std-module=mime` failed
"go detect_content_type" with no fault: 125 inputs got "" or another
input's hex string. `XO_SEPCOMP=0` (and LLVM) gave the right output.

## Cause

A constant of a collection type, or one computed by a call (decision
0332), lives in one table of the process, read by a number the SSA builder
gives in order of first use (ssa show.xo const_value). std's store and the
program's side were each numbered from 0, and std's code, and its bodies
the program inlines from std's facts, read std's constants by the store's
numbers. A program constant and a std constant with the same number shared
a slot: whichever was built first was the other's value.

## Fix

The program's side of a separately compiled build numbers its constants
from ssa.CONST_PROGRAM_BASE (2048); the driver fails a std object of 2048
constants or more. internal/arm64 TestSepcompConstIds builds std's
variants and runs a program of five constants that calls mime.

## How to avoid

Any number one compilation unit hands out and another reads at run time
(constant slots, spawn sites, type ids) needs a range per unit once units
are compiled apart, or a link-time name instead of a number.
