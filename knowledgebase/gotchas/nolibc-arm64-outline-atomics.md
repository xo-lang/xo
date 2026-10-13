# XO_NOLIBC=1 arm64 links: libgcc's outline atomics are missing

Status: Observed (2026-10-13, decision 0206 stage 2, linux/arm64).

Symptom: with `XO_NOLIBC=1`, every program built by the arm64 backend
failed to link on main once the reference counts moved to Xo (decision
0207 stage 2, std/native/llrt/rc.xo):

```
prog.s: in function `xo_f_rc_2einc': undefined reference to `__aarch64_ldadd8_relax'
```

(153 of 153 golden programs, bench/results/linux-syscalls-2026-10-13.md,
"Stage 2"). The LLVM backend linked.

Cause: on Linux the arm64 emitters lower the read modify write atomics
of decision 0200 (`slot_swap`, `slot_add`, `slot_cas`) as calls of
libgcc's outline helpers (`__aarch64_cas8_acq` and friends), as clang's
`-moutline-atomics` objects do. A static link with `-nostdlib` has no
libgcc. Stage 1 did not see it: no runtime module made such a call yet.

Fix: the driver writes the helpers itself for an XO_NOLIBC arm64 link
(selfhost/xo link.xo `nolibc_atomics`, from the ELF linker's
`macho.outline_atomic`, load and store exclusive loops, one section
each for --gc-sections). The C runtime is compiled with
`-mno-outline-atomics` there and the LLVM IR carries the same target
attribute, so neither calls them.
