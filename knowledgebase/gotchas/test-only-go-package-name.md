# A test only Go package cannot be an external test package

Status: Observed (2026-10-13, decision 0215).

## Symptom

After decision 0215 deleted the Go code of internal/llvm, internal/gen,
and internal/arm64 and kept their tests, `go test ./internal/llvm`
failed with "build constraints exclude all Go files in .../internal/llvm"
and, while a test still imported the package, "import cycle not allowed
in test".

## Cause

Their tests were external test packages (`package llvm_test`). An
external test package needs the package under test; a directory with
only `_test.go` files has none, so go finds no package to build.

## Fix

Name the tests' package as the directory's package (`package llvm`,
`package gen`, `package arm64`): a directory of internal test files
alone is a valid package that only `go test` builds, and `go build
./...` and `go vet ./...` accept it. cmd/xo's tests stay `package main`
the same way. The same holds for a new test only package
(internal/repotest).
