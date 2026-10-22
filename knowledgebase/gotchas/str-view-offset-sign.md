# A Str view 2 GiB or more into its string reads 4 GiB early

Status: Observed (2026-10-22)

Symptom: a windows/amd64 build of internal/arm64's huge_frame (about
9,000 variables in one frame) failed in the assembler with "an
instruction outside __text" at line 75,169,964. The build's assembly
(`--asm=FILE`, 3.2 GB) held 1.08 GB of NUL bytes starting 91,847 bytes
past 2^31, each line's bytes zero but its newline intact. A small
program reading `s[at..at + 8]` of a 2.25 GB string at `at` past 2^31
faulted with SIGSEGV 2 GiB below the string (cabi.xo
obj_rs_append_bytes, from an interpolation).

Cause: a Str or Bytes length word is `len | (off << 32)` (decision
0330), so a view whose offset is 2^31 or more has the sign bit set. The
generated code reads the offset with a logical shift (arm64 `lsr #32`,
LLVM `lshr`), but the runtime's Xo code used `>>`, which is arithmetic
on Int: std/native/llrt cabi.xo view_data and obj_rs_append_bytes made
the bytes start 4 GiB early, and obj.xo's "is this a view" tests
(`an <= LEN_MASK`, `an > LEN_MASK`) are signed, so such a view of a
string held only by the view counted as a unique plain string and an
append wrote in place over the string's start. xo's text writer joins
lines that are views of a larger text, hence the zeroed lines with
their newlines.

Fix: the offset is `x.shr_logical(32)`, and "is a view" is `an >> 32 !=
0` (true for either sign). internal/arm64 TestStrViewPast2GiB generates
a 2 GiB + 1 MiB string and reads, compares, concatenates and appends to
views past 2^31 (about 2.2 GB of memory). Any new runtime code that
takes a view's offset apart must use shr_logical, and compare the word
for "is a view" by its high half, never by a signed bound.
