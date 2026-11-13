# 0626: a parsed certificate pool for std/x509

Status: Proposed (Xo Lang 3; stub, filled in after the x509 review's sign-off)

## Context

x509.verify takes roots and intermediates as lists of Certificate and
parses each one again from its DER on every call: with the 150 macOS
system roots that is about half of a verification's time (the decision
0616 x509 review, std/evidence/x509/review.md section 6). std/tls passes
parsed roots and is not affected. Go keeps a CertPool, parsed once and
indexed by subject and key identifier.

## Decision

To be written: a CertPool type (Go's shape, Xo's API), verify's current
signature kept, measured with 150 roots before and after against Go.

## Consequences

To be written.

## Alternatives

To be written.
