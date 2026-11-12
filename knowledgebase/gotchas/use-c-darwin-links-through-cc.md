# A `use c` program on darwin builds through cc even with std/sqlite's prebuilt object

Status: Observed (2026-11-11, Xo Lang 2, the 0.49.1 pre-release sweep)

Symptom: a check meant to prove a release links std/sqlite's prebuilt
object (decision 0534 stage 2) built a std/sqlite program with no C
compiler on PATH; it failed on darwin/arm64 and darwin/amd64 with "the
arm64 backend needs a C compiler to assemble and link", for 0.48.2's
archive as for main's. A hello program builds there with no cc.

Cause: a program with `use c` takes the assembly and system linker path
on darwin (cc assembles the program's .s files and links), while a plain
program is written and linked by Xo's own Mach-O writer. The prebuilt
object only saves compiling sqlite3.c, xo_sqlite.c and the adapter.

Fix: to check the prebuilt object is used, put a logging cc in front of
the system's and check that no call compiles sqlite3.c
(~/xo-lead/notes/xo-publish/release-0.49.x/sq-prebuilt.sh: 2 or 3 cc
calls, none of them sqlite3.c, when the key matches). Do not test with
an empty PATH on darwin.
