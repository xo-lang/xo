# `xo check` passes, the native build of the same Xo code refuses it

Status: Observed (2026-10-11, decision 0174).

**Symptom.** Code for a program built with a native backend (the self
hosted driver `compiler/xo`, built with `--backend=arm64`) checks clean
with `xo check`, then `xo build --backend=arm64` stops at once with
XO1201 "not available without a garbage collector (`--no-gc`) yet: the
method `Env.program`". In this repo the build sat in the heavy.sh queue
for over an hour before failing in one second.

**Cause.** Plain `xo check` checks for the Go backend, where the whole
prelude exists. Several prelude methods are Go backend only for now
(XO1201 on native builds): `Env.program`, `Fs.mode`, `Fs.glob`,
`Fs.read_link`, `Fs.open`, `Proc.cmd` and `Cmd`, among others (std.md
marks them "native builds report them as XO1201, planned").

**Fix.** Check native code with `xo check --no-gc <dir>` before queueing
a native build. xo1 works around the missing methods: `$_` for the
running binary's path, `stat(1)` for nanosecond modification times,
`sh -c 'cd DIR && exec env NAME=V... BIN'` for a child's environment.
