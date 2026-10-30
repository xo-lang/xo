# A host bundle's xo reads its own std while cmd/xo tests pass the source std

Observed 2026-10-30 (Xo Lang 4). Status: Observed.

**Symptom.** On the linux/arm64 offload host, a cmd/xo gate run from a
bundle of `xo/bin/xo`, `xo/std` and the source tree `src/` failed
TestXoTestStdModuleNames ("src/std/native/base64.xo is not a
directory" instead of naming `--std-module`), TestVetStd (vet of
`src/std/native/llrt`: check_errors 8425) and TestAgentRuntimeRefs
(`xo def park_wait` found poll.xo only). None of them fail on the Mac.

**Cause.** xo finds std at `bin/../std`, so the bundled xo read
`xo/std`. cmd/xo's `xoStd()` helper takes XO_STD, else
`bin/../lib/xo/std`, else the repository's std, so the tests passed
paths in `src/std`: a second copy of std that xo does not treat as its
own (modules seen twice, a file in std not recognized as std's).

**Fix.** Set `XO_STD` to the source tree's std (`XO_STD=$R/src/std`)
with `XO_CLI` in any gate run from such a bundle, so the tests and xo
read one std. A bundle that also runs `use go` tests ships
`xo-gobind` beside xo (`GOOS=linux GOARCH=arm64 go build -o
xo/bin/xo-gobind ./cmd/xo-gobind`); without it every TestUseGo* fails
with XO1101 "xo-gobind not found".

Related: heavy.sh writes the command's output to
`~/xo-lead/agents/NAME-cmd.log`, not to the caller's redirect, so a
script waiting for a heavy.sh job waits on its product file (a
`.tgz`), never on the redirected output.
