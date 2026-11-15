# 0643: a std error struct gains Go's error types as a kind enum, text unchanged

Status: Accepted (aws-43, 2026-11-14, on the lead's order for x509 piece 7:
additive only, no change to any error text)

## Context

Go's crypto/x509 returns typed errors (CertificateInvalidError with a
Reason, HostnameError, UnknownAuthorityError, InsecureAlgorithmError,
ConstraintViolationError, UnhandledCriticalExtension) that callers tell
apart with errors.As. std/x509's `CertErr` held only Go's text, so a caller
could only match on strings. Other std modules port Go packages with typed
errors the same way (one error struct with Go's text), so the shape chosen
here is one they can copy.

## Decision

- The error struct keeps its fields and text and gains
  `kind: <Mod>ErrKind = <Mod>ErrKind.Other`: an enum with one variant per
  Go error type, carrying that type's exported fields that are plain data
  (a reason enum in Go's order, a detail, a host, an algorithm's Go name).
  Fields that point at a certificate (Go's Cert, Certificate) are left
  out: the caller has the certificate.
- `Other` is the default, so every existing literal (`CertErr{msg: m}`)
  still builds and means an error Go returns untyped (errors.New, a parse
  error).
- The Go backend sets the kind from Go's own error with errors.As at every
  site, so an untyped Go error is Other there too. The native backends
  set it where they make the error, from the outcome they already compute
  (never by parsing text they did not make; a fixed Go text such as
  ConstraintViolationError's may be matched exactly).
- A corpus row shows each error as Go's text plus its type
  (" {Kind detail}"), from Go's errors.As, so a kind that differs from Go
  on any backend fails the same test as a text that does.

## Consequences

Callers match `e.kind` instead of `e.msg`. A module adopting this adds the
enum and the default field; no error text and no caller changes. SystemRootsError is not
a kind in std/x509: its verify takes roots explicitly, so neither backend
returns it.
