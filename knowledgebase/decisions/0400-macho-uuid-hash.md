# 0400: the Mach-O UUID from a cheaper hash than SHA-256

Status: Proposed

## Context

compiler/macho link.xo write takes the executable's LC_UUID from a SHA-256 of the whole
image (a 6.8 MB HTTP server: one full pass on top of the code signature's page hashes).
A UUID only has to be unique per content and deterministic.

## Decision

(to be filled: the hash, the measured share, what reads UUIDs)

## Consequences

## Alternatives
