# A large `const` List of Int costs every build that links it

Status: Observed (2026-10-15, B31 P-521)

Symptom: a generated `pub const G521: List[Int]` of 35,370 Ints (P-521's
fixed base table, 684 KB of source, like nistec_g256.xo's 7,680) made
every arm64 build of a program linking nistec.xo (every TLS program)
take 29.6 s instead of 3.0 s, every time (no cache across builds), and
the binary 5.0 MB instead of 2.5 MB.

Cause: a constant List compiles to code that builds the list (one store
per element), not to read only data, and std modules are compiled with
the program.

Fix: keep such tables small (nistec_g256.xo's 7,680 words are the
largest so far), or compute them at run time once per process (Go's
P-521 generator table is lazy) in a process wide slot: since 2026-10-16
P-384's and P-521's tables are made on first use (nistec.xo gtab,
static words 44 and 45 via nistec_gtab.xo, decision 0237's freed words;
bench/results/nistec-gtab-2026-10-16.md).
