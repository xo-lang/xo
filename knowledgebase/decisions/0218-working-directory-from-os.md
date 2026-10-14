# 0218: The working directory comes from the OS, not $PWD

Status: Proposed (stub, 2026-10-13); being implemented on a branch.

xo reads its working directory from the PWD environment variable in about
ten places (gotchas/xo-working-dir-is-pwd.md). A process started without
a shell inherits a stale PWD (VS Code from the Dock passes PWD=/), so
relative paths resolve wrongly. Decision to be filled in: a std working
directory call backed by the OS getcwd on every backend, with $PWD only
as a fallback.
