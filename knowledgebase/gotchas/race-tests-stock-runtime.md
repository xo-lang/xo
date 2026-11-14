# Race tests and evidence silently linked clang's stock ThreadSanitizer runtime

Status: Fixed (2026-11-14)

## Symptom

http's race evidence on host 2 measured 1.30 to 1.41 times Go's race memory
per task after scripts/race-runtime.sh switched xo's runtime to 128 KB trace
parts (e3ae167f2), and the internal/llvm race tests and TestGoldenRace kept
passing on hosts and in Mac worktrees without ever running the patched
runtime.

## Cause

`xo build --race` links xo's own runtime from `lib/xo/race/GOOS-ARCH/`
beside std, and when a checkout has none it falls back to clang's
(`-fsanitize=thread`'s own runtime, or clang's dylib on darwin) with no
message. `lib/xo/race` is gitignored and only releases and seeds carry it,
so every offload host checkout and every Mac worktree tested clang's stock
runtime (256 KB trace parts), not the one xo ships.

## Fix

- Hosts: scripts/lead/race-rt.sh builds xo's linux runtime once per
  race-runtime.sh hash (Docker on the Mac) and copies it once per host;
  race-rt-host.sh installs it in every offload-gate, tier and stdproven
  checkout and fails the job without it.
- Mac: gate.sh runs `race-rt.sh --local` after xobuild, which builds the
  darwin runtime once per hash and installs it in the worktree (a tree's
  own kit runtime is kept); a failed build stops the gate.
- stdproven race refuses without lib/xo/race (`-stock-runtime` allows
  clang's) and race.md's `race runtime:` line names the runtime.

## Rule

A test or measurement of race behavior names the runtime it ran. A change
to scripts/race-runtime.sh is tested with the runtime it builds, never
with whatever the checkout falls back to.
