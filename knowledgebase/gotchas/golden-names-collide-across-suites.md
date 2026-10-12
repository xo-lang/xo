# Golden names collide across the two arm64 suites, and asmdump paths leak into faults

Status: Observed (2026-10-12, decision 0182 stage 3)

Symptom: a scratch harness that dumped each golden's assembly (`xo
asmdump --os=linux DIR`) into `NAME.s` and ran the linked binaries got 51
of 159 wrong: every faulting golden printed `at
internal/arm64/testdata/run/x/main.xo:7:9` where want.txt says `at
main.xo:7:9`, and `byref_enum` and `nested_update` printed another
program's output.

Cause: two things. `xo asmdump` writes source locations relative to its
working directory, so run from the repository root they carry the whole
path; `xo build` and xo1 write them relative to the program's directory.
And `internal/llvm/testdata/golden` and `internal/arm64/testdata/run` both
have `byref_enum` and `nested_update`, so files named by the directory's
base name overwrite each other.

Fix: run `xo asmdump` from the golden's directory (`cd DIR && xo
asmdump --os=linux .`), and name outputs by the whole relative path with
`/` as `_`, as `scripts/selfhost-goldens.py` and
`scripts/elf-goldens.py` do.
