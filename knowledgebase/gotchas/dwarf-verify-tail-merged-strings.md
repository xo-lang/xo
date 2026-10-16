# llvm-dwarfdump --verify rejects tail merged .debug_str entries

Status: Observed (2026-10-16, LLVM backend line tables, decision 0239).

Symptom: `llvm-dwarfdump --verify` on an LLVM backend binary: `invalid
string offset *0x8 == 0x5, is neither zero nor immediately following a
null character`, while lookups and symbolization work.

Cause: the assembler merges .debug_str with tail sharing: the producer
"xo" was the tail of "main.xo", so its offset points into another
string, which is valid DWARF but not what the verifier expects.

Fix: none needed. The LLVM compile unit's producer is "xo compiler",
which no file name ends with, but function names still share tails
(the TLS client: 2 entries); treat this verifier message alone as
harmless, and check with lookups or llvm-symbolizer instead.
