# A Windows variable set in Git Bash under another case is a second variable

Status: Observed (2026-10-28)

## Symptom

The 0.33.1 Windows release smoke's use-go-local-helper check printed the
program's output (27) but found no xo-gobind.exe under the user cache it
had pointed `LocalAppData` at: `FAIL use-go-local-helper (out '27', built '')`,
nothing on stderr.

## Cause

Git Bash (MSYS2) imports the Windows environment with upper case names,
so the runner's variable is `LOCALAPPDATA`. `LocalAppData=DIR cmd` adds a
second variable rather than replacing it, and both reach the child's
environment block. Xo looks names up ignoring case and takes the first
match (std/native/llrt/process.xo fold_case and env_real_at), as Windows
does, which was the runner's own `LOCALAPPDATA`: the helper was built into
the runner's real cache.

## Fix

Override a Windows variable from Git Bash by its imported name
(`LOCALAPPDATA=DIR`), or `unset` it first. scripts/ci/release-smoke.sh
does so since 2026-10-28.
