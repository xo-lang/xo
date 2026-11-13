# 0626: a parsed certificate pool for std/x509

Status: Proposed (Xo Lang 3, 2026-11-13; adds public API, so the owner decides)

## Context

`x509.verify` takes its roots and intermediates as `List[Certificate]`.
The public Certificate is a value of plain fields (spec/std.md 18.x);
the checks verify needs (name constraints, policies, the raw subject and
key forms, the signature algorithm) live in the native backend's own
parsed form (std/native/llrt/x509cert.xo's Cert), which the public
struct does not carry. So the native verify parses every root and
intermediate again from its DER on every call (std/native/x509.xo
`lcert`). With 150 roots (about what `system_roots` returns on macOS)
that is half of a verification's time: an `XO_PROF=cpu` profile of
bench/perf/x509 `verify150` puts 210 of 420 ms in `lcert`. The decision
0616 x509 review (std/evidence/x509/review.md section 6, item 6) listed
it as a performance follow-up, not a security issue.

std/tls is not affected: `tls.roots` and `tls.roots_of` build a
`tls.Roots` handle once, and the TLS handshake verifies against its
parsed certificates. A program that calls `x509.verify` itself
(mTLS checks of its own, certificate tooling, a service validating
uploaded chains) pays the cost on every call.

Go keeps a `CertPool` (crypto/x509 cert_pool.go): certificates parsed
once and indexed by subject, `AddCert`, `AppendCertsFromPEM`,
`SystemCertPool`, and `VerifyOptions.Roots` and `Intermediates` are
pools.

## Decision

Add a pool type to std/x509 and let VerifyOptions take one, keeping
every current call working:

```
struct CertPool {}                                       // handle
fn cert_pool(certs: List[Certificate]) -> CertPool ! CertErr
fn (var p: CertPool) add(c: Certificate) -> () ! CertErr // Go's AddCert; the same DER is kept once
fn (p: CertPool) certificates() -> List[Certificate]    // in the order added
fn (p: CertPool) len() -> Int

struct VerifyOptions {
  roots: List[Certificate] = []                         // was required
  root_pool: CertPool? = None
  intermediates: List[Certificate] = []
  intermediate_pool: CertPool? = None
  ...                                                   // unchanged
}
```

- verify uses a pool's certificates and the list's together (the list
  parsed per call, as now), so existing code compiles and behaves as
  before; `roots` gains a default so a pool alone suffices.
- `cert_pool` and `add` parse once and return Err for a DER that does
  not parse (a Certificate built by hand rather than by `parse`); a
  certificate already in the pool (the same DER) is kept once, as Go's
  AddCert.
- Native: the pool holds the parsed llrt certificates, the public ones,
  and a map of DERs seen; verify reaches it with `rt.cast` from the
  public handle. Go backend: the pool wraps a `*x509.CertPool` and the
  public list, and verify passes it to Go's Verify as is.
- Measured on bench/perf/x509 (verify of Go's www.google.com chain at
  its test time, the real root among 149 generated Ed25519 roots, 2,000
  calls): see the table below.

| Workload | Xo before (lists) | Xo with a pool | Go (CertPool) |
|---|---|---|---|
| darwin/arm64 (Mac, wall, one run, setup included) | 0.46 s | 0.20 s | 0.26 s |
| linux/arm64 host 2 (stdproven perf, 7 rounds, medians) | 615.7 ms | 323.4 ms | 513.5 ms and 705.7 ms |

On host 2 Go's two rows run the same code (Go's VerifyOptions always
takes pools), and their medians differ by 37%, so the host's spread is
that large: the pool halves Xo's own time (0.525), and Xo against Go goes
from 1.199 to 0.458 (0.63 against Go's better row). Before the pool,
verify150 was the only x509 workload slower than Go (std/evidence/x509
perf run of 2026-11-13 at 103e3de1b, held in the lang3-certpool branch
with the code).

## Consequences

- A program verifying many chains against the same roots builds a pool
  once and keeps it; each call then costs about what a one root
  verify costs (the Mac's `verify`: 88 µs a call, `pool150`: about 95).
- `roots` becomes optional: a VerifyOptions with neither roots nor a
  pool trusts nothing and fails with Go's unknown authority text, as Go
  with an empty pool.
- spec/std.md 18.x documents the type, std/changes/x509.md has a
  feature entry, and the Go backend gains the matching std/rt type.
- Later, not in this decision: `system_pool(fs)` (Go's SystemCertPool)
  to keep the platform's roots parsed, and `tls.roots_of_pool` so a pool
  and a `tls.Roots` share one parse.

## Alternatives

- Keep the parsed form inside Certificate (a hidden field): the public
  struct is plain data that programs build, compare and copy; a hidden
  field would change its `==`, its size and its literals, and the llrt
  type cannot be named in std/x509.xo.
- A cache inside the native module keyed by the DER: a package level
  cache that grows without bound and needs a lock across tasks; the
  std porting gotcha (std-port-costly-go-idioms.md) rules out package
  level caches.
- A `verify_with(roots: CertPool, ...)` function beside verify: two ways
  to say the same options; a field on VerifyOptions keeps one verify, as
  Go's.
- Reuse `tls.Roots`: std/tls depends on std/x509, so x509 cannot take a
  tls type; the later `tls.roots_of_pool` covers the sharing.
