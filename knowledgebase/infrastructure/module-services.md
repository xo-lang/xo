# Module services: sum, index, proxy and vuln (xolang.org)

The public services `xo` talks to for modules and vulnerabilities, where
each runs and how it is checked. Host addresses, account ids and tokens are
private (decision 0249): "the owner's OCI host", "the Cloudflare account".

| Host | What | Where it runs | Source |
|---|---|---|---|
| sum.xolang.org | checksum log (decision 0568) | `xo mod sumdb serve` as the systemd unit xo-sumdb on the owner's OCI host, loopback :7071, reached through a Cloudflare Tunnel | scripts/modservices/ |
| index.xolang.org | the module index (0568) | the same server; the tunnel sends `/` and `/index` there (path `^/(index)?$`) | scripts/modservices/cloudflared-config.yml |
| proxy.xolang.org | module proxy files (0568) | Cloudflare R2, its root rewritten to `/index.txt` by a URL rewrite rule | scripts/modservices/ |
| vuln.xolang.org | vulnerability database, OSV 1.6 in Go's vuln.go.dev layout (decision 0647) | Cloudflare R2 bucket xo-vulndb, custom domain; the root rewritten to `/index.txt` | github.com/xo-lang/vulndb (build.yml publishes on push) |

## Tunnel

cloudflared on the OCI host runs with a local config
(`/etc/cloudflared/config.yml`, installed from
scripts/modservices/cloudflared-config.yml), so the tunnel's remote
configuration in the Cloudflare dashboard is ignored: an ingress change is
made in the repository's file, copied to the host, and cloudflared
restarted (`sudo systemctl restart cloudflared`). Check one rule without
restarting: `sudo cloudflared tunnel ingress rule --config
/etc/cloudflared/config.yml https://index.xolang.org/`.

## Checks

```
curl -s -o /dev/null -w '%{http_code}\n' https://vuln.xolang.org/index/db.json   # 200
curl -s -o /dev/null -w '%{http_code}\n' https://proxy.xolang.org/               # 200
curl -s -o /dev/null -w '%{http_code}\n' https://index.xolang.org/               # 200 once the host's tunnel config has ^/(index)?$
xo vuln .                                                                        # exit 0 clean, 3 with findings, 1 (XO1323) when unreachable
```

State, 2026-11-15 (Xo Lang 2): vuln.xolang.org `/`, `/index.txt`,
`/index/db.json` and `/index/modules.json` answer 200 (std 64 entries,
toolchain 2); `xo vuln` against a byte copy of it reports XO1321 for
std/csv's XO-2026-0061 and both toolchain entries at a 0.40.0 toolchain
(exit 3), XO-2026-0061 at 0.51.1 (fixed in the next release; exit 3), and
nothing for a program with no std import (exit 0). proxy.xolang.org/ is
200. index.xolang.org/index is 200; `/` is 404 at the edge until the
host's tunnel config carries the repository's `^/(index)?$` (the origin
answers 200 for `/`). On the development Mac the resolver returned only
the AAAA records for vuln.xolang.org that day and IPv6 to it had no route
(curl -6 fails too), so `xo vuln` there failed with XO1323; other
networks resolve both. Last verified: 2026-11-15
