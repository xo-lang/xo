# A read or write of more than 2 GB in one system call fails

Status: Observed (2026-10-17, decision 0252 stage 4's fixed point; fixed)

Symptom: writing a file of more than INT_MAX (2,147,483,647) bytes failed
on macOS with `Io(path: ..., msg: "invalid argument")`. The self build
with DWARF types for every variable wrote xo's own assembly (over 2 GB)
and stopped at stage s2.

Cause: the native runtime passed the whole remaining length to each
read(2) and write(2) (std/native/llrt/darwin.xo and linux.xo read,
write, read_at, write_at; fsops.xo write_at's loop). macOS refuses a count
above INT_MAX with EINVAL; Linux moves at most 0x7ffff000 bytes a call
(a short count, which the loops handled). Windows already chunked
(ReadFile and WriteFile take a 32 bit length).

Fix: std/native/llrt/raw.xo io_len caps every one call at a chunk (1
GiB) and the callers loop on short counts, as they already did for pipes
and signals. A test sets the chunk small (std/internal rt_probe
"io_chunk", static word 47): internal/llvm TestRuntimeIOChunk
(testdata/rt/iochunk) moves files, handles, standard output, and a
child's input in 7 byte chunks, and a write to a child that stops reading
fails partway; XO_IO_CHUNK_BIG=1 adds one real file of 2^31 + 16 bytes.
