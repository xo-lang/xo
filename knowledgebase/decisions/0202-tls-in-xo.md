# 0202: TLS written in Xo replaces Mbed TLS

Status: Accepted (owner, 2026-10-13); not started. Last in decision
0195's order.

## Context

Decision 0195 aims for no C that Xo compiles. The zero-C audit
(bench/results/zero-c-audit-2026-10-13.md, "Owner decisions found", A)
found Mbed TLS (third_party/mbedtls, 196,689 raw lines, much of it
configured out) and fiat-crypto (4,226 lines) are the largest C left,
all of it blocked on constant time code (prerequisite 5). It offered
three end states: (a) an Xo TLS stack after Go's design; (b) Mbed TLS as
the one permanent C exception; (c) the platform's TLS (Security.framework,
SChannel), which leaves Linux needing OpenSSL as a system library.

## Decision

(a): TLS and the crypto under it are written in Xo, following Go's
crypto/tls and crypto design, and replace Mbed TLS and fiat-crypto. It
comes last in decision 0195's order, after the constant time
prerequisite (5) exists. The constant time support stays internal to
std (decision 0074) unless the owner decides to expose a user marker.

## Consequences

- No C remains in the TLS path; one implementation on every target,
  including Linux without a system TLS library.
- New, security critical protocol code in Xo: it needs test vectors
  (Wycheproof, RFC vectors), interoperability tests against major
  servers and clients, differential tests against the current Mbed TLS
  path, and fuzzing, before the C path is removed (decision 0193's
  mechanism: the C path stays selectable until then).
- Constant time code must survive the optimizer on every backend: the
  prerequisite (5) design must make branches and memory accesses on
  secret values impossible to introduce, and be checked (for example by
  timing tests and inspecting the emitted code).

## Alternatives

- (b) Keep Mbed TLS as the one C exception: rejected by the owner; it
  contradicts decision 0195.
- (c) Platform TLS: rejected; Linux has no platform TLS and would need
  OpenSSL as a system dependency, and behavior would differ per OS.
