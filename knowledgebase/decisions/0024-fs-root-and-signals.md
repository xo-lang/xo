# 0024: os.fs root, lazy sub, process signals

Status: Accepted (2026-10-07)

## Context
A CLI taking user paths could not tell where `os.fs` is rooted; `sub` on a
missing directory made dry runs awkward; nothing said what stops a server in a
real process. (Paper test 2026-10-07 spec 0.2, [results](../../bench/results/paper-2026-10-07-v0.2.md))

## Decision
- `os.fs` is rooted at the filesystem root, accepts absolute paths, resolves
  relative paths against the working directory. `os.fs.cwd()` gives a sub
  capability. In a sub capability `/` and `.` mean its root.
- `Fs.sub` is lazy and infallible.
- First SIGINT/SIGTERM cancels `main`'s root scope; a second exits with 130.
