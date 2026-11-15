# riscv64 tools on host 2: clang.cfg's -mcpu and a transitional qemu package

Status: Observed (Xo Lang 1, 2026-11-15, decision 0645 stage 1)

## Symptom
- `clang -c --target=riscv64-linux-gnu ...` on host 2 fails with
  `unsupported argument 'generic' to option '-mcpu='`.
- `apt-get download qemu-user-static`, unpacked, runs nothing: `timeout: failed to run
  command '.../usr/bin/qemu-riscv64-static': No such file or directory`.

## Cause
- /opt/llvm/bin/clang.cfg adds `-mcpu=generic` (meant for arm64). riscv64 has no CPU of
  that name.
- On Debian 13 (trixie), qemu-user-static (77 kB) is a transitional package. Its
  `qemu-riscv64-static` is a link to `qemu-riscv64`, which ships in qemu-user (64 MB).

## Fix
- Run clang with `--no-default-config` for riscv64 (and llvm-mc needs no config).
- Download qemu-user (with libc6-riscv64-cross for the dynamic loader) and run
  `qemu-riscv64`, `-L .../usr/riscv64-linux-gnu` for a glibc program
  (infrastructure/offload.md, riscv64 tools on host 2).
