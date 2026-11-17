# TestDevGoldensELF failed a `use c` golden only when the short sample drew it

Status: Observed (2026-11-17, Xo Lang 3)

Symptom: a gate's nohttp step failed `TestDevGoldensELF/native_sqlite_file`
with `xo: --ld=macho links no C sources (.../cffi/xo_c_xo_sqlite_045d638a.c):
link with cc`, on a change unrelated to sqlite; other gates of the same day
passed.

Cause: the ELF run builds with `--ld=macho`, Xo's linker, which compiles no
C, so a golden with a C binding cannot link there. xo dev's classify only
calls a cross built `use c` program unsupported when the C source is in the
program's own directory; std/sqlite's C comes from std/native. Under -short
the run takes every 25th program (`--sample=25`), so the gate reached the
golden only when new goldens shifted the index onto it (latent since
760e5ba3b, 2026-11-10). Also: gate.sh's nohttp step runs `-run
'Go|Replay|Holes'`, and `Go` matches every TestDevGoldens* name, so the
step runs the Arm64, LLVM and ELF goldens as well as the Go backend's.

Fix: TestDevGoldensELF skips a failing program whose `xo build
--list-inputs` names a C binding (a file in a cbind directory: `.xo/cbind`,
or the cache's `test-cbind` under go test), so a new C golden is covered
too (internal/devgoldens usesC, TestUsesC).
