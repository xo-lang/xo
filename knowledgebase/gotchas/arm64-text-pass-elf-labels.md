# arm64 text passes did not see ELF labels

Status: Observed (2026-10-17, linux/arm64)

Symptom: on Linux, the arm64 backend's final text passes
(compiler/arm64/lsr.xo pair_mem and reload_reuse) did much less than on
macOS. bench/perf/xo copy_words was 18 instructions per 32 bytes with no
ldp or stp, while `xo asm` of the same program (no debug info) paired
them; `branch` and `copyset` ran 3x LLVM's instructions on linux/arm64.

Cause: label_def recognized only the Mach-O local label form `L...:`.
ELF local labels are `.L...:` (gen.xo local), so on Linux every label
line ended a pair_mem search and cleared reload_reuse's registers (as a
"symbol" line: safe, never wrong, just no reuse). Default builds put a
debug line row label (`_dl<n>`, decision 0239) before nearly every
source line and variable location labels (`_dv<n>`, decision 0252)
between instructions, so on Linux almost nothing paired across lines.
On macOS the `_dv` labels, which the debug sections refer to, also
ended pair_mem's search (it checked "referred to" before "debug row").

Fix: label_def takes `.L...:` too, named without the dot (as
label_refs names references); debug row and variable labels are passed
by pair_mem whatever refers to them and do not clear reload_reuse's
registers (no branch targets them). Unit tests in lsr_test.xo cover both
label forms. Linux callgrind at f5d4f61c: copyset 62.1M to 40.7M,
branch 2092M to 1117M, push 78.6M to 71.7M (with the address fold of the
same change).

How to avoid: a text pass that matches labels must take both forms; test
it with `.L` labels as well as `L`.
