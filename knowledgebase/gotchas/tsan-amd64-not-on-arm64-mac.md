# amd64 ThreadSanitizer programs cannot run on the arm64 Mac

Status: Observed (2026-10-21)

## Symptom

A linux/amd64 --race program in Docker (scripts/docker-linux.sh amd64,
x86 emulation) aborts at start: first `ThreadSanitizer: CHECK failed:
... personality(old_personality | ADDR_NO_RANDOMIZE) != -1` (the default
seccomp profile refuses the personality call), and with `--security-opt
seccomp=unconfined` `FATAL: ThreadSanitizer: memory layout is
incompatible, even though ASLR is disabled`. A C program built with
clang -fsanitize=thread in the same container fails the same way, so it
is the emulator's address space, not Xo. A darwin/amd64 build gives "Bad
CPU type in executable": this Mac has no Rosetta.

A second trap when shipping such binaries to the Intel Mac: DYLD_* set in
a script does not reach a program started through /usr/bin/perl (or any
other SIP protected binary, such as /usr/bin/env), so dyld does not find
the TSan dylib. Point the binary's load command at
`@executable_path/...` (install_name_tool -change, then codesign -f -s -)
instead.

## Fix

Run amd64 --race checks on real x86: the owner's Intel Mac (darwin/amd64,
with the owner's approval for each run; memory intel-mac-runs) or an x86
Linux machine. Build and link on the arm64 Mac, which works for both
targets.
