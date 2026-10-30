# 0422: `use go` binds a variadic of a function type (Go's functional options)

Status: Proposed (Lang 2, 2026-10-29; the lead asked for it after the Go workflows suite)

## Context

Stub: being written. internal/gobind skips a variadic last parameter of a function type
(`optFns ...func(*s3.Options)`), which removes AWS SDK v2's s3.NewFromConfig and every Client
operation, and the same functional options pattern across gRPC and cloud SDKs.

## Decision

To be written: bind it as `List[callback]`, each element converted as a callback parameter is.

## Consequences

To be written.

## Alternatives

To be written.
