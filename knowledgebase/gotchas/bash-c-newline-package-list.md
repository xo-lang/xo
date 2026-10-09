# A package list with newlines breaks `bash -c`

Status: Observed (2026-10-09)

Symptom: the first EC2 CI run (`infra/ci-aws`) reported `short` and `full`
with exit 127 after 7 s and 115 s; the logs held one `bash: line N:
github.com/xo-lang/xo/internal/...: No such file or directory` per package.

Cause: `PKGS="$(go list -e ./... | grep -v /runs/)"` keeps the newlines;
spliced into a command string run by `bash -c "go test $PKGS"`, each
newline ends the command, so only the first package (cmd/xo) was tested
and every other line ran as a command. Plain `go test $PKGS` in the same
shell is fine (word splitting turns newlines into separators).

Fix: `tr '\n' ' '` the list before splicing it into a command string
(`infra/ci-aws/remote-jobs.sh`). Check a harness by the pass counts in its
logs, not only its exit codes.
