# A 1501-byte post-quantum ClientHello hung every R2 request from an MTU 9000 host

Status: Observed (2026-11-11)

## Symptom

On the owner's OCI host (linux/arm64, xo 0.45.0 under `xo mod sumdb
serve`), every lookup that added a version failed with
`cannot record ...: PUT records/0000000000: timeout` after 120 s, so the
checksum log never added a record and the index feed and the proxy bucket
stayed empty. From the same host, curl reached the R2 endpoint in 28 ms
and git fetched from GitHub. The same xo builds (0.45.0, 0.46.0, 0.48.0)
had no problem from the Mac, and against fake buckets on linux/arm64 with
TLS, HTTP/2 (empty 200s), an IPv6 blackhole and serve mode on one CPU.

## Cause

xo's TLS client offers X25519MLKEM768 first, as Go 1.26's crypto/tls
does (tls13.xo client_groups), so its ClientHello carries a 1,216-byte
key share and is 1,501 bytes on the wire (275 with `XO_TLS_MLKEM=0`).
The host's VNIC (enp0s6) has MTU 9000, so the ClientHello leaves as one
packet of about 1,550 bytes with its IP and TCP headers, and the path to the internet is 1,500. With path MTU
discovery broken (no ICMP type 3 code 4 back to the host), the packet is
dropped silently and the handshake waits until the client's timeout.
OpenSSL 3.0's curl and git send a ClientHello of about 500 bytes and fit.

Every xo program on such a path is affected: any HTTPS client of std/http
to a server that takes the hybrid (Cloudflare, Google), not only the
module services. Go programs on the same host fail the same way, since
Go sends the same hello; xo keeps Go's default (no behavior change).

## Fix

- The host's lasting fix: allow ICMP type 3 code 4 in the subnet's
  security list (OCI's default list has it), or `ip link set <vnic> mtu
  1500`.
- Until then, `XO_TLS_MLKEM=0` (Go's `GODEBUG=tlsmlkem=0`) in the
  program's environment: X25519 only. The xo-sumdb unit sets it
  (scripts/modservices/xo-sumdb.service, 6402e3002), and the modservices
  README's "R2 requests time out" gives the check: a dummy-key `xo mod
  proxy fill` to the endpoint that hangs without `XO_TLS_MLKEM=0` and
  answers a fast 4xx with it.
- When an xo HTTPS request times out on one host only while curl works
  there, compare the two with `XO_TLS_MLKEM=0` before suspecting the
  client.
