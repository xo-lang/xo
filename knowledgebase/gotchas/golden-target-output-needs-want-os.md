# A golden whose output depends on the OS needs want_<os>.txt

Status: Observed (2026-10-13, sys_files_clock on windows/amd64)

Symptom: the Windows batch reported `golden sys_files_clock: fail
(output differs: exit 0)` (152 of 153) on every commit since decision
0206 added the golden, on the Xo path and on the C path alike.

Cause: the program prints `long: Io` on Windows (a `match target.os`
arm: Windows words the error otherwise) but the golden had only
want.txt, which holds the Linux and macOS line with the message.

Fix: a program that prints something per `target.os` gets a
want_<os>.txt for each other output (TestGoldenWant reads
want_<host os>.txt first); sys_files_clock has want_windows.txt.
