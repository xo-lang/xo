# The ThinLTO link of an LLVM-built xo1 needs about 19 GB

Status: Observed (2026-10-12, xo1-x86 agent, bench/results/xo1-x86-2026-10-12.md)

Symptom: building xo1 (selfhost/xo) with `--backend=llvm` split into 12
modules with ThinLTO (`XO_LLVM_SPLIT=12`, `XO_LLVM_LTO` unset, so
`thin`) spends most of its time in one linker process: on an EC2
m7i.4xlarge (linux/amd64, GNU ld with the LLVM gold plugin, clang
15.0.7) `/usr/bin/ld` reached 18.8 GB resident and the whole build took
1,918 s; on the 16 GB Intel Mac (ld64, Apple clang 17) the link
pushed the machine about 8.5 GB into swap and the build took 5,181 s.
The per module clang compiles are small next to it.

Cause: with `-flto=thin` each module's object is bitcode, so the
optimization and code generation of all of selfhost/xo (hundreds of MB
of IR) happen inside the link, in parallel, in the linker's address
space.

Fix: internal/selfdriver builds xo1 on an amd64 host split into 12
modules without LTO (`xo1Backend`: `XO_LLVM_LTO=none` unless the
environment names an LTO mode), and scripts/bootstrap-xo.sh and
scripts/xo1-host-check.sh use the same setting. Decision 0168 measured
the cost of dropping ThinLTO at 2 to 3 percent of run time. Set
`XO_LLVM_LTO=thin` to get the old behavior on a machine with the
memory.
