# A native build of several targets used one backend and one file name

Status: Observed (2026-10-22, the owner's 0.25.2 field report)

Symptom: `xo build --targets=all` with no `--backend` failed with "the
arm64 backend builds for darwin/arm64 and linux/arm64, not linux/amd64",
although each target built alone, and the README advertises the command.
`--targets=darwin/arm64,linux/arm64` succeeded but printed the same path
for both (`.../arm64/bin/p`): the second binary overwrote the first.
Separately, `-o app` for windows/amd64 wrote `app`, which Windows does
not run.

Cause: compiler/xo main.xo picked one backend for the whole build
(native_backend: the last target's architecture), and build_native
rejects a target its backend does not build. Its link steps named each
binary with `bin_path(..., many=false)`, the one-target name, whatever
the target count (the Go backend passed the count). bin_path returned
`-o`'s path as given.

Fix: with no `--backend`, backend_groups splits the targets by the
native backend that builds them (target_backend: arm64, amd64, wasm) and
build_by_backend runs build_native once per backend, then prints one
report (or one JSON document) of every binary; `all` leaves out
windows/arm64, which none builds, with a note (owner, 2026-10-22), and
naming it is an error suggesting `--backend=go`, after the rest.
build_native passes `o.many_targets` to bin_path (`NAME-OS-ARCH`), the
wasm branch names its module the same way, and bin_path adds `.exe` to
an extensionless `-o` for windows. cmd/xo TestTargetsAllNative builds
`--targets=all --json -o DIR` and checks each file's name and format.
