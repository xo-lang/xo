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

cloudflared runs on the OCI host as the systemd unit cloudflared
(`--config /etc/cloudflared/config.yml`). Since the restart on 2026-11-15
the tunnel follows its remote configuration in the Cloudflare dashboard
(the journal logged "Updated to new configuration" at version 2), and the
remote configuration wins over the local file. The local file is kept in
step with scripts/modservices/cloudflared-config.yml anyway, in case the
tunnel goes back to local config. So an ingress change goes into the
dashboard (Zero Trust, Networks, Tunnels), into the repository's file,
and into the host's copy. cloudflared does not have to be restarted for
a remote change. Check the local file with
`sudo cloudflared tunnel --config /etc/cloudflared/config.yml ingress validate`
and one rule with
`sudo cloudflared tunnel --config /etc/cloudflared/config.yml ingress rule https://index.xolang.org/`.
`--config` goes before `ingress`: `cloudflared tunnel ingress validate
--config FILE` prints the usage and still exits 0, so it checks nothing.
The host keeps the file from before that day as
`/etc/cloudflared/config.yml.bak-2026-11-15`.

## Checks

```
curl -s -o /dev/null -w '%{http_code}\n' https://vuln.xolang.org/index/db.json   # 200
curl -s -o /dev/null -w '%{http_code}\n' https://proxy.xolang.org/               # 200
curl -s -o /dev/null -w '%{http_code}\n' https://index.xolang.org/               # 200
xo vuln .                                                                        # exit 0 clean, 3 with findings, 1 (XO1323) when unreachable
```

State, 2026-11-15 (Xo Lang 2): vuln.xolang.org `/`, `/index.txt`,
`/index/db.json` and `/index/modules.json` answer 200 (std 64 entries,
toolchain 2); `xo vuln` against a byte copy of it reports XO1321 for
std/csv's XO-2026-0061 and both toolchain entries at a 0.40.0 toolchain
(exit 3), XO-2026-0061 at 0.51.1 (fixed in the next release; exit 3), and
nothing for a program with no std import (exit 0). proxy.xolang.org/ is
200. index.xolang.org `/` and `/index`, and sum.xolang.org `/`, are
200 (the lead fixed the `/` route on the host, with the owner's ok).
On the development Mac the resolver returned only
the AAAA records for vuln.xolang.org that day and IPv6 to it had no route
(curl -6 fails too), so `xo vuln` there failed with XO1323; other
networks resolve both (gotchas/darwin-resolver-aaaa-only.md). Last verified: 2026-11-15
