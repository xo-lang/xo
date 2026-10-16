# Wine under Docker Desktop's Rosetta emulation cannot dispatch exceptions

Status: Observed (2026-10-16, xo-4).

**Symptom.** A windows/amd64 program that raises a structured exception
(a stack overflow, for internal/llvm/testdata/windows/stack_overflow)
ends at once under Wine in the emulated linux/amd64 container
(scripts/docker-wine.sh) with `rosetta error: invalid gdt selector index
5` and exit 133, before any vectored handler runs. Programs that raise
nothing (hello, the http, net, fs goldens) run normally.

**Cause.** Docker Desktop on Apple silicon runs amd64 containers with
Rosetta, which does not emulate the x86 segment selectors Wine's
exception dispatch loads. Debian's arm64 Wine is no way around it: it
runs no ARM64 PE program (a hello hangs).

**Fix.** Run the amd64 Wine under Box64 in a native linux/arm64 container
instead (`scripts/docker-wine.sh box64`: Debian's box64 and
wine64:amd64 through multiarch): the stack overflow golden passes there
(2026-10-16). Turning Docker Desktop's Rosetta off would also do, but
it is the owner's setting and changes every session's amd64 containers.
