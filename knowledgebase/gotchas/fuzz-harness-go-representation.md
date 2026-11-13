# A fuzz harness compares Go's parsed form, not the field std keeps

Status: Observed (2026-11-13)

## Symptom

The decision 0616 x509 fuzz (1,000,000 cases) failed twice on one case
of 5,000, each time a URI subject alternative name:

- seed 2: std/x509 gave `spiffe://trust.example/n{/a`, the harness's
  Go side `spiffe://trust.example/n%7B/a`;
- seed 10 (after a first harness fix): std/x509 gave no URI, the Go
  side `http://[::1]:8080/`.

Neither was a parser difference.

## Cause

std/x509 keeps each URI as the SAN's text (`uris: List[Str]`); Go keeps
a parsed `*url.URL`, and the harness printed `u.String()`, which
re-escapes. The first fix read the raw SAN with encoding/asn1 and took
every context-specific [6], including a constructed one (`0xa6`, made by
a mutated tag byte), which Go's parseSANExtension skips (it switches on
`tag ^ 0x80`) and so does std/x509.

## Fix

Compare the representation std documents (spec/std.md 18: `uris` are
the text as written), read from the raw extension with Go's own rules:
primitive [6] only (internal/stdtest/x509fuzz_test.go xzRawURIs,
b4653851d and 82714542f). Before calling a fuzz difference a bug,
check which side changed the value: a Go type with its own String()
(url.URL, net.IP, big.Int, time.Time) is a representation, not the
input, and a hand-written raw reader must apply Go's tag rules.
