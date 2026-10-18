# 0268: Fault reports with the faulting frame's locals

Status: Proposed (xo-4, 2026-10-18; stub). To be filled in.

## Context

A fault report on the native backends shows the frames, files, and
lines (dwline.xo); the values of the faulting frame's variables need a
debugger today.

## Decision

To be written: with `XO_TRACEBACK=locals` (off by default) the report
lists each frame's variables, read from the DWARF of decision 0252 with
the reader rules of decision 0253 (llrt dwvars.xo beside dwline.xo).
