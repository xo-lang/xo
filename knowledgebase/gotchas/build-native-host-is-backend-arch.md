# build_native's `host` is the backend's arch, not the machine's

Status: Observed (Xo Lang 2, 2026-10-30)

Symptom: on the linux/arm64 offload host, TestCrossUseCFailsLoudly failed in
tiers 28 and 29: `xo build --json --targets=linux/amd64` of a `use c` program
printed about 40 assembler errors from the host's arm64 cc (`unknown token in
expression` on `movq %rbx, %rdi`), "xo: xo-cc failed", and no --json report,
where it should refuse the cross link ("linux/amd64 links with a C compiler
... build it on linux/amd64"). The Mac passed: from darwin the target is
never the host.

Cause: compiler/xo/main.xo build_native sets `host` to `${goos}/${march}` for
the native backends (the target a build with no --targets makes), so with
the amd64 backend on a linux/arm64 machine `host` is linux/amd64. The cross
link check (`tg != host`) and asm_only then took linux/amd64 for the
machine's own target and handed its assembly to the machine's cc.

Fix: the cc checks compare with `cc_host`, the machine's own target
(`${goos}/${host_arch()}`; darwin counts either arch, since Apple's cc builds
both). Any new "can this machine's cc do it" test uses cc_host; `host` stays
the default target. Linux only bugs like this show up only on the
linux/arm64 host: run a cross target test there, not only on the Mac.
