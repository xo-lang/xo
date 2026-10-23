# x86 frameless blocks read stack parameters at the framed offset

Status: Observed (2026-10-23, fixed the same day)

## Symptom

A program built with the amd64 backend computes a wrong value in a
function with more argument words than the six registers: a parameter
passed on the stack reads the word after it (golden
x86_frameless_stack_args: `eight(...)` printed `0 442542985` instead of
`607 910`; text's zone_offset gave another checksum once the register
allocator's ranking changed). arm64 is not affected.

## Cause

Shrink wrapping (compiler/arm64 shrink.xo, decision 0224) lays out the
blocks that need no frame without it, so in them rsp is the entry's and
the first stack parameter is at 8(%rsp). The x86 emitter's x86_in_arg
gave the framed offset (frame size + 16) for every block but a leaf
function's, so an entry block left without the frame read 8 bytes off.
It stayed hidden while the allocation happened to need a callee saved
register at the entry (which puts the frame there).

## Fix

x86emit.xo x86_in_arg_here: the K_ENTRY parameter loads and the K_RET
result stores of a block outside the frame's region use 8 + 8k. When an
allocator or layout change makes a function frameless at its entry,
run a program with stack-passed parameters on amd64 (the golden above).

Not fixed yet: dwvars.xo describes a stack parameter's DWARF location
with x86_in_arg too, so XO_TRACEBACK=locals can show such a parameter
8 bytes off inside a frameless block.
