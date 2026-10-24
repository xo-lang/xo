# internal/llvm's goldens do not fit go test's default timeout

Status: Observed (2026-10-24)

## Symptom

`go test ./internal/llvm` (or with `XO_TEST_GO_PARITY=1`) runs for ten
minutes and then prints

```
FAIL	github.com/xo-lang/xo/internal/llvm	600.573s
FAIL
```

naming no failing test, so it reads like a broken golden or a hang. Agents
on 2026-10-24 lost two runs to it in one evening, 600.5 s and 603.1 s, and
the same work passed in 14 s once the run was narrowed.

## Cause

The package needs 46+ minutes from cold: every golden builds a program and
links it with clang, and the -O1 set alone is that large (the full tier runs
it; a per-push gate runs only the goldens a change touches). go test's
default `-timeout` is 10 minutes, so the panic lands mid-package with no
test named. A package that takes longer than the default must be given a
timeout or narrowed with `-run`.

## Fix

- One golden: `go test ./internal/llvm -run 'TestGolden/NAME$' -v`, NAME a
  directory under internal/llvm/testdata/golden (add
  `XO_TEST_GO_PARITY=1` for the Go backend comparison; TestGoldenGoParity is
  its own test, `-run 'TestGolden(GoParity)?/NAME$'` runs both).
- The whole package: `go test -timeout 90m ./internal/llvm`.
- `-short` skips enough of it to finish, but still runs TestTimeDiff and
  friends, so it is not a fast check either.
- Since 2026-10-24 the test binary stops such a run before it does any work
  and prints that advice (internal/llvm/golden_timeout_test.go, called from
  TestMain): a run with no `-run` filter, no `-timeout` of its own, and
  without `-short` exits 1 in a second with the two commands above.
