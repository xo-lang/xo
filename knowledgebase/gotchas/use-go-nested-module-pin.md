# `use go` of a package in a nested Go module took the enclosing module's pin

Status: Observed (2026-10-29, the Go workflows suite, bench/usego-workflows/aws)

## Symptom

With `require go github.com/aws/aws-sdk-go-v2 v1.47.5` in xo.mod,
`use go "github.com/aws/aws-sdk-go-v2/service/s3"` failed to bind:
`go: module github.com/aws/aws-sdk-go-v2@v1.47.5 found, but does not
contain package github.com/aws/aws-sdk-go-v2/service/s3`, and the same for
s3's own import of `aws/protocol/eventstream`.

## Cause

`service/s3` and `aws/protocol/eventstream` are Go modules of their own
inside the root module's path. The pin lookup (gobind PinFor, the
checker's go_pin_for, tidy's go_pin) takes the longest pinned module
prefix and assumed it was the package's module, so Bind ran
`go get github.com/aws/aws-sdk-go-v2/service/s3@v1.47.5`; the check called
a binding stale when the root's version was not the binding's; tidy saw
the root's line and added no pin for s3.

## Fix

The lookups return the pinned module with its version. Bind gets the
pinned module at its version, then the package (when the workspace does
not have it yet) at the version Go picks beside it; the stale and lock
checks compare only when the pinned module is the binding's; check
reports XO1319 for the nested module, and tidy pins it. Tests:
internal/gobind TestNestedModulePin, cmd/xo TestUseGoNestedModulePin
(synthetic example.com/nest and example.com/nest/sub in testdata/goproxy).
