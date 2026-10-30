# An offload run's name can delete another job's /work directory

Status: Observed (2026-10-30, Xo lang 5, the linux/arm64 offload host).

Symptom: a run of a per-agent offload script (worktree at `/work/l5-NAME`, removed with `rm -rf` before and after the run) named `grpc` deleted `/work/l5-grpc`, the grpc-go interop setup another job had built (its clone and binaries; the read only Go module cache stopped the rm and left 114 MB). The run then failed with `fatal: '/work/l5-grpc' already exists`.

Cause: the script derived its scratch path from the run name alone and deleted whatever was there, so any other directory with the same prefix was fair game.

Fix: a script that `rm -rf`s a path built from a name owns that path by a marker (`$W.l5run`, touched after it creates the worktree, removed with it) and refuses to start when the path exists without one. Keep long lived setups (toolchains, clones) under a name no run uses (`/work/l5-grpcgo`), and build Go module caches with `-modcacherw` so they can be deleted later. The same holds on the Mac: a chain that creates a worktree must stop when `git worktree add` fails (an existing path), or the commands after it run in the current directory (a commit once landed in the main checkout that way).
