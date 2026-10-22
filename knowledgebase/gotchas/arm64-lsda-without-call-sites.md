# arm64: a function with a landing pad named an LSDA it never wrote

Status: Observed (2026-10-22, xo's self-build with every optional pass off)

Symptom: `xo: link: .../prog-darwin.79.o: a difference with an import`:
the eh_frame FDE of `arm64.G.str_data` subtracted from
`GCC_except_table_..._27`, a label no object defined.

Cause: emit's cfi_start writes `.cfi_personality` and `.cfi_lsda` when
any instruction has a landing pad, but the table itself was written
only when a call site with a pad was emitted. With the optional passes
off, str_data kept an instruction with a pad and no such call site.

Fix: the arm64 emitter writes the table whenever cfi_start named
it, with an empty call site table if need be; the personality
(std/native/llrt unwind.xo landing) continues the unwind for a pc the
table does not list, as for any call without a pad.

The x86 emitter (x86emit.xo) has the same condition; its fix waits for
the Docker amd64 golden sample (patch kept in the Xo Lang 4 notes).
