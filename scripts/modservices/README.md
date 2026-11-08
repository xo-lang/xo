# Module services (decision 0568)

The checksum log behind sum.xolang.org and index.xolang.org, which also
fills the public proxy bucket behind proxy.xolang.org, run as one
systemd service on the owner's OCI host (linux/arm64) and reached only
through a Cloudflare Tunnel. Nothing here creates a cloud resource: the
owner runs every step below by hand.

- `install.sh`: as root, from an xo release archive for linux/arm64:
  the `xomod` user, `/srv/xomod` (the release, `log/`, `modcache/`), a
  template `xomod.env` (mode 0600), the `xo-sumdb` unit and
  `/etc/cloudflared/config.yml` (kept when one exists). It starts
  nothing and makes no key.
- `xo-sumdb.service`: `xo mod sumdb serve` on 127.0.0.1:7071 with
  `--fetch`, `--rate=60`, `--copy-to` (the log bucket), `--fill-to`
  (the proxy bucket) and `--refresh=30m` (every module's `@v/list`
  rewritten from its tags, written only when it changed), confined to
  `/srv/xomod` (3 GB, one CPU). An upload to the proxy bucket that
  fails waits in `/srv/xomod/log/fill-pending` (one `path@version` per
  line) and is retried on each refresh and at the next start; a line
  that stays there names a version R2 keeps refusing (its error is in
  `journalctl -u xo-sumdb`). `https://sum.xolang.org/refresh/<module
  path>` rewrites one module's list at once, for a release that should
  not wait the 30 minutes.
- `cloudflared-config.yml`: the tunnel's ingress: sum.xolang.org to the
  log, index.xolang.org's `/index` to the same port, anything else 404.

## Before install.sh (Oracle Linux 9)

    dnf install -y git policycoreutils-python-utils

## SELinux

Oracle Linux 9 enforces SELinux. A file under `/srv` is `var_lib_t` by
default, which systemd (`init_t`) may not execute, so an unlabeled
release fails to start with status 203/EXEC. When SELinux is on,
`install.sh` adds three file context rules and runs `restorecon -R
/srv/xomod`:

| Path | Type | Why |
|---|---|---|
| `/srv/xomod(/.*)?` | `var_lib_t` | the log, the module cache, `xomod.env`, the key |
| `/srv/xomod/xo(/.*)?` | `usr_t` | the release's std, read only |
| `/srv/xomod/xo/bin(/.*)?` | `bin_t` | xo: `init_t` executing `bin_t` runs it as `unconfined_service_t` |

`unconfined_service_t` may bind 127.0.0.1:7071, run git and connect
out, so no policy module is needed. cloudflared from Cloudflare's
package is `/usr/bin/cloudflared` (`bin_t`, the same transition) with its
unit as `systemd_unit_file_t` and `/etc/cloudflared` as `etc_t`; keep its
credentials file in `/etc/cloudflared` (step 3), not under `/root`
(`admin_home_t`).

Checks after the first start:

    getenforce                                   # Enforcing
    ls -Z /srv/xomod/xo/bin/xo                   # ...:bin_t:...
    ps -eZ | grep -E 'xo|cloudflared'            # ...:unconfined_service_t:...
    ausearch -m avc -ts recent                   # <no matches>
    systemctl status xo-sumdb cloudflared

A denial shows in `ausearch -m avc`; `restorecon -Rv /srv/xomod` fixes a
file moved in with the wrong label (`mv` keeps the old one).

## After install.sh

1. The key (as xomod; the private key never leaves the host, the
   verifier key printed goes in xo's default `XO_SUMDB` at the flip):

       sudo -u xomod /srv/xomod/xo/bin/xo mod sumdb keygen sum.xolang.org /srv/xomod/log.key

   Keep an offline copy of `log.key` (decision 0568, key custody).
2. In the Cloudflare dashboard: two R2 buckets (proxy, log), an R2 API
   token with Object Read and Write on those two buckets only, and the
   proxy bucket's custom domain proxy.xolang.org with the cache rule of
   0568. Fill `/srv/xomod/xomod.env` with the token's access key ID and
   secret, the account's R2 endpoint and the two bucket names.
3. The tunnel (cloudflared installed from Cloudflare's package
   repository):

       cloudflared tunnel login
       cloudflared tunnel create xomod
       cloudflared tunnel route dns xomod sum.xolang.org
       cloudflared tunnel route dns xomod index.xolang.org

   Move the credentials file it wrote (`/root/.cloudflared/<id>.json`)
   to `/etc/cloudflared/` and `restorecon -v /etc/cloudflared/<id>.json`,
   put the tunnel ID in `/etc/cloudflared/config.yml` (TUNNEL_ID,
   twice), then `cloudflared service install`.
4. Start and check:

       systemctl enable --now xo-sumdb
       curl -s https://sum.xolang.org/latest
       XO_PROXY=https://proxy.xolang.org XO_SUMDB='<verifier key>' xo get <module path>

## Upgrading xo

Run `install.sh` with the new archive, then `systemctl restart xo-sumdb`.
It replaces `/srv/xomod/xo` only; the key, the log, `xomod.env` and the
tunnel config stay.

## Restore and moving out

The log bucket holds a copy of every record with its time, written
before the record is added. On a new host (another OCI region, or the
Hetzner fallback of 0568) run `install.sh`, copy `log.key` and
`xomod.env` over, then as xomod:

    set -a; . /srv/xomod/xomod.env; set +a
    /srv/xomod/xo/bin/xo mod sumdb restore --from="$XO_R2_ENDPOINT/$XO_LOG_BUCKET" /srv/xomod/log

It rebuilds the same records, times and signed head. Point the tunnel at
the new host (`cloudflared tunnel run` there with the same credentials,
or a new tunnel and `route dns` again) and stop the old service. The
proxy bucket and its domain do not move.

## Tested

2026-11-08, in linux/arm64 containers with systemd as PID 1 (Debian 12
and Oracle Linux 9.8), with an xo cross-built for linux/arm64 and a PUT
sink standing in for R2: install.sh (twice: an upgrade keeps
`xomod.env`), keygen as xomod, the unit started and stayed active, a
real lookup (github.com/xo-lang/aws@v0.1.0 fetched with git) copied
its record and filled `.zip`, `.lock`, `.mod` and `list` in that
order, and restore rebuilt identical records and times.
`systemd-analyze verify` passes and `systemd-analyze security` rates
the unit 1.7 (OK). The containers run with SELinux off: the file
context rules and the policy rules above were checked offline against
Oracle Linux 9's targeted policy (`matchpathcon`, `sesearch`), not
enforced; the first enforcing run is the owner's install (the checks
above). Last verified: 2026-11-08.
