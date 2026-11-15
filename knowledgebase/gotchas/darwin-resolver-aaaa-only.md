# The Mac's system resolver returned only AAAA records, so dials failed with XO1323

Status: Observed (2026-11-15, Xo Lang 2, on the development Mac)

## Symptom

`xo vuln .` failed with XO1323 (the database unreachable), and so did
`curl https://vuln.xolang.org/index/db.json` (curl exit 6, could not
resolve). Other machines reached the host.

## Cause

On macOS, dial resolves names with the system getaddrinfo
(std/native/llrt/cabi_net.xo resolve_libc; the pure Xo resolver in
dns.xo serves Linux and std/dns record lookups only). That day,
mDNSResponder's answer for the name held only the two AAAA addresses
(`dscacheutil -q host -a name vuln.xolang.org` listed ipv6_address
lines only), and this network had no IPv6 route. Asked directly with
`dig A` and `dig AAAA`, the router (192.168.0.1) answered both families.
xo's dial already races every resolved address in both families
(netconn.xo dial_race, Go's dialParallel, RFC 6555), but there was no
A address to fall back to.

## Fix / workaround

- Check the system resolver's answer first:
  `dscacheutil -q host -a name HOST`, then compare it with `dig +short A HOST`.
- Pin an IPv4 address for one check:
  `curl -4 --resolve HOST:443:IP https://HOST/...`. For `xo vuln`, use a
  byte copy of the database with `--db=file://DIR`.
- Open with Lang 3 (std/net owner): whether darwin's dial should also ask
  the pure Xo resolver for A records when getaddrinfo returns only one
  family and every connect to that family fails. Go on darwin uses
  libSystem's resolver too and would fail the same way.

Last verified: 2026-11-15
