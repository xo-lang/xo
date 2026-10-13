# Git for Windows checks goldens out with CRLF

Status: Observed (2026-10-12), fixed in scripts/remote-windows.sh.

Symptom: on the Windows EC2 instance every LLVM golden failed
(`0 pass, 127 fail`), each with output that looked identical to
want.txt.

Cause: Git for Windows installs with `core.autocrlf true`, so a clone
writes want.txt (and every text file) with `\r\n` line endings, while
the programs print `\n` (the runtime puts standard output in binary
mode, as Go does).

Fix: clone with `core.autocrlf false` (the script sets it globally on
the instance before cloning). A checkout that already has CRLF files:
copy the tree again from a Unix host (tar of `git ls-files`) or
renormalize. Tests read want.txt byte for byte, so do not strip `\r` in
the tests instead: a `\r` the runtime wrote by mistake would be hidden.
