# arm64: a var method on a struct of 18 fields failed with "no free register for a spilled operand"

Status: Observed 2026-11-07 (std/native/htmltok.xo, `Tokenizer.read_raw_or_rcdata`; Xo Lang 3).

Symptom: `xo test` or `xo build` stops with `arm64 backend: internal: no free register for a spilled operand in
htmltok.Tokenizer.read_raw_or_rcdata: one instruction with 10 registers and more spilled operands than free registers`.

Cause: a `var` method returns the updated struct, and the SSA rebuilds it with one `struct v1, ..., v18` instruction over
all 18 fields (one of them a Map literal built beside it); with the loop's live values the arm64 allocator has more
spilled operands in that one instruction than free registers. The same fields split into fewer top level members
(the four pending attribute spans moved into a nested struct) compile.

Fix: keep a struct that a hot `var` method updates under about 15 fields, grouping related fields in a nested struct.
Compiler side: the struct instruction should build field by field when its operands do not fit (not done).
