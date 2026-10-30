# 0427: native Windows DNS lookups use Xo's own client

Status: Proposed (Xo lang 5, 2026-10-30; the lead's queue item 2 after decision 0426). Partly supersedes decision 0417's stage 2 plan (binding DnsQuery_W).

## Context

Decision 0417 gave native Windows `Err(Io("dns lookups are not supported on windows yet"))` for lookup_mx, lookup_txt, lookup_srv and lookup_addr, with a stage 2 that binds DnsQuery_W. Std/native/llrt/dns.xo already speaks DNS over UDP and TCP on Linux and macOS, and its sockets layer (llrt sock.xo) already runs on Winsock.

## Decision

To be filled.

## Consequences

To be filled.

## Alternatives

To be filled.
