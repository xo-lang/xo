# 0568: hosting the public module proxy and checksum log

Status: Accepted (owner, 2026-11-08, on the lead's recommendation; owner, 2026-11-09: the OCI host is for hosting only, never a test or pass/fail host; conditional on the owner confirming the OCI console shows the checksum log VM as Always Free, met 2026-11-10: VM.Standard.A1.Flex at 2 OCPUs and 12 GB, on-demand, the tenancy's only instance, and Cost Analysis shows $0.00 for every service from 2026-10-01 to 2026-11-10; the owner confirmed the account type; key rotation: yes to a standby key built into xo, two offline copies kept by the owner). Proposed (Xo Lang 2, 2026-11-08; the owner decides). The owner
approved a public proxy plan on 2026-11-08 and chose its direction the
same day, through the lead:

- R2 serves the module files through Cloudflare's CDN.
- The owner's OCI host runs the fill service and the log.
- The services live under xolang.org (proxy.xolang.org, sum.xolang.org,
  index.xolang.org), as Go uses golang.org beside go.dev; xolang.dev is
  unchanged.

This ADR records that choice, its costs and the alternatives, and the
work that comes before a launch. No cloud account, DNS record or host
has been touched for it, and none will be until the owner accepts it.

## Context

Decisions 0124 (checksum log) and 0126 (module proxy) built the client
side, and since c4f9aad40 (2026-11-08) both servers exist in Xo:

- `xo mod proxy serve --fetch` (0126);
- `xo mod sumdb keygen` and `xo mod sumdb serve` (0124's addendum).

cmd/xo TestChecksumLog runs them end to end on loopback. No public
instance runs, so XO_PROXY defaults to `direct` and XO_SUMDB to `off`.
maturity-gaps.md item 2 names the missing public proxy and log as a
top gap.

What a public service needs beyond the two commands:

- a fill path that fetches a version from git once, records it in the
  log, and stores its `.zip`, `.mod` and `.lock` where the CDN serves
  them;
- a durable, append-only record store and an always-on signer for the
  log;
- `@v/list` answers that track new tags;
- an index feed.

## Prices (retrieved 2026-11-08 from each provider's own pages)

| Item | Price | Source |
|---|---|---|
| R2 storage | $0.015 / GB-month; 10 GB-month free | developers.cloudflare.com/r2/pricing |
| R2 Class A (writes) | $4.50 / million; 1 million free | same |
| R2 Class B (reads) | $0.36 / million; 10 million free | same |
| R2 egress | free | same |
| Cloudflare plans | Free $0; Pro $20 (annual) or $25; Business $200 (annual) or $250 | cloudflare.com/plans/network-cdn |
| Cloudflare cacheable file size | 512 MB on Free, Pro and Business | developers.cloudflare.com/cache/concepts/default-cache-behavior |
| Workers Free | 100,000 requests a day, 10 ms CPU each | developers.cloudflare.com/workers/platform/pricing |
| Workers Paid | $5 a month with 10 million requests, then $0.30 per million; counted before the cache | same |
| Cloudflare for SaaS (Free) | 100 custom hostnames, then $0.10 each | developers.cloudflare.com/cloudflare-for-platforms/cloudflare-for-saas/plans |
| Cloudflare Tunnel | every plan; outbound connections only | blog.cloudflare.com/tunnel-for-everyone, developers.cloudflare.com/cloudflare-one/connections/connect-networks |
| S3 Standard | about $0.023 / GB-month for the first 50 TB; GET $0.0004 per 1,000; the PUT price did not load (an older AWS page says $0.005 per 1,000); 100 GB a month to the internet free; S3 to CloudFront free | aws.amazon.com/s3/pricing (the table did not load in full) |
| CloudFront pay as you go | 1 TB and 10 million requests a month free; then $0.085/GB for US, Mexico and Canada to 10 TB, $0.080 for the next 40 TB; HTTPS $0.0100 per 10,000 | aws.amazon.com/cloudfront/pricing/pay-as-you-go |
| CloudFront flat rate | Free $0 (1M requests, 100 GB), Pro $15 (10M, 50 TB), Business $200 (125M, 50 TB), Premium $1,000 | aws.amazon.com/cloudfront/pricing |
| Hetzner CAX11 (arm64) | €6.49 a month excl. VAT: 2 vCPU, 4 GB, 40 GB NVMe, 20 TB traffic (EU); the page marked every CAX plan "currently unavailable" that day | hetzner.com/cloud/cost-optimized |
| Backblaze B2 | $6.95 / TB-month; egress free to 3x storage and free through partner CDNs (Cloudflare among them); API calls free | backblaze.com/cloud-storage/pricing |
| OCI Always Free | A1.Flex: 1,500 OCPU hours and 9,000 GB hours a month (2 OCPUs and 12 GB for an Always Free tenancy); 10 TB a month outbound; 200 GB of block volume | docs.oracle.com, Always Free Resources |

## Cost at three sizes

Assumptions:

- launch: 1,000 modules, 10 GB stored, 100 GB a month served;
- growth: 10x that;
- popular: 100x that.

Requests: about 2 million a month at the edge at launch (files plus
`list`, `.mod` and `.lock`, at a 200 KB average), 90% served from the
CDN cache, so 10% reach R2 as Class B reads. Writes: four objects per
new version, about 2,000 versions a month at launch. Both scale with
the size. These are estimates; the private beta measures the real
ratios.

| Monthly | Launch | Growth (10x) | Popular (100x) |
|---|---|---|---|
| A. R2 + Cloudflare Free CDN (files) | $0 (inside the free tier) | about $1.35 (90 GB of storage over the free 10) | about $18.45 ($14.85 storage, $3.60 for 20M Class B) |
| A + Workers for fill on a miss (phase 3) | $0 (66,000 requests a day, under the free 100,000) | about $8 ($5 + 10M extra × $0.30) | about $62 ($5 + 190M × $0.30) |
| B. S3 + CloudFront, pay as you go | about $0.35 | about $13.50 (mostly the 10M requests over the free tier) | about $1,000 (9 TB at $0.085, 190M requests, storage) |
| C. A VM serving the files itself (Hetzner CAX11 + B2) | €6.49 + under $1 | €6.49 + under $1, but 100 GB exceeds the 40 GB disk, so files come from B2 | €6.49 + $6.95, 10 TB inside the 20 TB, one server with no CDN |
| Fill service and log on the owner's OCI host | $0 if the host is Always Free (see below) | $0 | $0, unless the load needs a larger host |
| The same on Hetzner (the fallback) | €6.49 (CAX11) | €6.49 | €10.99 (CAX21) if git fetches need more memory |

The domain costs nothing new: the owner already holds xolang.org.

A costs nothing until the growth size and about $18 a month at 100x.
Every provider's price for the files rounds to zero at launch. The
difference is the popular size, where AWS's per-GB egress costs about
fifty times as much.

## Decision (proposed)

### Names and DNS

- `proxy.xolang.org`: module files, an R2 custom domain behind
  Cloudflare's CDN.
- `sum.xolang.org`: the checksum log. It is also the log's name: XO_SUMDB
  without a URL means `https://<name>` (decision 0124), so the default
  needs no URL.
- `index.xolang.org`: the index feed.
- xolang.org's nameservers move from Namecheap to Cloudflare (Free plan).
  The owner makes the change in Namecheap; the registrar stays Namecheap.
  - An R2 custom domain requires the domain to be a zone in the same
    Cloudflare account (r2/buckets/public-buckets).
  - A subdomain-only zone is Enterprise only (dns/zone-setups/subdomain-setup).
  - A partial (CNAME) setup is Business or Enterprise only, $200 to $250
    a month (dns/zone-setups/partial-setup).
  - So a whole zone on the Free plan is the only $0 route. xolang.org
    makes that harmless, with one exception below.
- Check before the move: xolang.org has Namecheap's email forwarding MX
  records (eforward1 to 5.registrar-servers.com, checked with dig on
  2026-11-08), not only the parking A record. Namecheap documents its
  free forwarding only for domains on its own DNS, so it stops when the
  nameservers move. Settled (owner, 2026-11-08): xolang.org is unused,
  mail included, so the forwarding MX records (eforward1 to 5) and the
  SPF record are dropped when the zone moves, with no Email Routing
  replacement.
- If DNSSEC is on at Namecheap, turn it off before the nameserver change
  and turn it back on with Cloudflare's DS record afterwards.
- xolang.dev stays on Namecheap DNS, unchanged.

### Files: R2 behind the CDN

- One R2 bucket. Objects sit at the protocol's own paths
  (`<escaped path>/@v/<version>.zip`, `.mod`, `.lock`), so the R2 custom
  domain proxy.xolang.org serves the proxy protocol as static files.
- Cache rules (Cloudflare cache rules plus the objects' Cache-Control):
  - `.zip`, `.mod` and `.lock`: `public, max-age=31536000, immutable`.
    A version's content never changes; a moved tag is the checksum
    log's business.
  - `@v/list`: `public, max-age=60`.
  - A 404: not cached (`no-store`), so a version filled a moment later
    is served at once.
- Every file is cacheable. The proxy's zip cap is 500 MB (fetch.xo
  MAX_ZIP), under the 512 MB cacheable size of the Free plan.
- How the fill service writes to R2: its S3-compatible API at
  `https://<account id>.r2.cloudflarestorage.com`, with an R2 API token
  scoped to that one bucket at "Object Read & Write" (r2/api/tokens).
  - The token's Access Key ID and Secret Access Key live only in the
    fill service's systemd credential on the host. They are never in
    the repository, the knowledgebase or CI.
  - The client is S3 SigV4 over std/http. The Xo AWS SDK's S3 client
    pointed at the R2 endpoint is the first candidate.

### The fill service and the log on the owner's OCI host

Hosting only (owner, 2026-11-09): the OCI host runs the fill service,
the log and cloudflared, and nothing else of the project. It is never a
test, gate, benchmark or pass/fail host: no offload env, no heavyd lane,
no gates, goldens, soaks or measurements run there, and its Always Free
state is confirmed in the OCI console (the three checks below), not by a
run on the host. Testing of the services stays on the Mac, the offload
hosts and containers (phase 1 below).

The host was read only checked by the lead on 2026-11-08:

- linux/arm64, 2 cores, 10 GB of memory (about 7 GB available, since
  it already runs something), 155 GB of free disk;
- Oracle Linux 9.8, 86 days up.

Its address is in no tracked file; this ADR calls it the owner's OCI
host.

- Always Free terms (docs.oracle.com, Always Free Resources):
  - A1.Flex gives 2 OCPUs and 12 GB on an Always Free tenancy, with
    10 TB a month of outbound data.
  - Idle Always Free instances "may be reclaimed by Oracle". An
    instance is idle when, over 7 days, CPU at the 95th percentile,
    network, and (A1 shapes only) memory are all under 20%.
  - The host's memory use (about 3 of 10 GB) is above 20%, so the rule
    does not apply today. The new services add to it.
  - Oracle's page does not say whether a Pay As You Go account's
    Always Free instances can be reclaimed; community reports say an
    upgraded account keeps them.
  - The owner checks three things in the OCI console before launch:
    1. the instance's shape is VM.Standard.A1.Flex;
    2. it carries the "Always Free-eligible" label;
    3. the tenancy is Free or Pay As You Go.
- Exposure: Cloudflare Tunnel (`cloudflared`, outbound only).
  - sum.xolang.org and index.xolang.org are published routes of one
    tunnel.
  - The services listen on 127.0.0.1 only, and the host opens no
    inbound port.
  - Large files never go through the tunnel. Cloudflare's terms for the
    Free, Pro and Business plans reserve video and other large files
    for its paid services, which R2 is. So zips go only through R2.
- Isolation from what already runs on the host:
  - a system user `xomod` with no login shell, owning `/srv/xomod`
    (log data, the git cache, the key);
  - three systemd units: `xo-sumdb.service`, `xo-fill.service` and
    `cloudflared.service`;
  - limits on the two xo units:
    - xo-sumdb: MemoryMax=1G;
    - xo-fill: MemoryMax=3G, since git and hashing are the large ones;
    - CPUQuota=100%, so one core stays for the existing work;
    - ProtectSystem=strict, ReadWritePaths=/srv/xomod;
    - NoNewPrivileges, PrivateTmp, ProtectHome;
  - xo built as a static linux/arm64 binary, installed under
    /srv/xomod/bin. No package installs beyond git and cloudflared.
- The fill service: a new command, `xo mod proxy fill`, built on
  proxyserve.xo and fetch.xo, with the S3 upload added. For a version
  it:
  1. fetches it from git with direct-only settings;
  2. asks the local log for its record;
  3. checks the record against the fetched tree's hashes;
  4. uploads `.mod`, `.lock` and `.zip` to R2, in that order;
  5. refreshes `@v/list` from `git ls-remote`.

  It is driven three ways:
  - by the log: every lookup of a version the log adds queues a fill,
    so the first `direct` fetch anywhere fills the proxy for the next
    user. The version is written to `<data dir>/fill-pending` before
    its upload starts and removed once the files are written, so an
    upload an R2 error or a restart cut off is retried when the log
    starts and on every `--refresh` tick (2026-11-08, cmd/xo
    TestChecksumLogFillRetry); before, a failed one was only logged and
    the version stayed out of the bucket until someone ran `xo mod
    proxy fill`;
  - by the log's `--refresh=30m` (2026-11-08): every module the log
    holds has its `@v/list` rewritten from its repository's tags on
    that interval, and written only when it changed. Without it a tag
    nobody looked up never reached the list, and a proxy client's
    `@latest` stayed on the older version, since a client falls back
    to git only on a 404. The cost is one git fetch of tags per module
    per interval (48 a day each; at the launch size about 33 a minute,
    inside the unit's one core) and an R2 write only per new tag. A
    module whose repository cannot be read waits 2^n ticks (at most 64)
    before its next try, so dead repositories do not hold up the
    sequential loop (2026-11-08, cmd/xo TestChecksumLogRefreshBackoff);
    `GET /healthz` (no-store) shows the record count, the uploads still
    pending, the loop's ticks and the failing modules with their last
    error, for the owner's monitor through the tunnel;
  - by `GET sum.xolang.org/refresh/<module path>` (2026-11-08): the
    same rewrite for one module at once, through the tunnel, for a
    module the log holds (404 otherwise), counted in the client's
    `--rate` since each one runs git. It is also what a phase 3 Worker
    would call.
- The log: `xo mod sumdb serve --fetch /srv/xomod/log`, whose records
  file is appended and synced before a tree head holding it is signed
  (0124's addendum).
  - Durability: the records file is copied to R2 (a separate, versioned
    bucket) after every append, from the fill service's queue, plus an
    OCI block volume backup daily (five are free).
  - A lost tail is the one failure clients see as a rewrite (XO1313),
    so the copy is required before launch, not optional.
- Proxy caching for the log's paths:
  - `/latest` and `/lookup/...`: `no-store`, since they carry the
    current head;
  - `/proof/inclusion/<id>/<n>` and `/proof/consistency/<m>/<n>`:
    `public, max-age=31536000, immutable`, since a proof for fixed
    sizes never changes.
- Rate limits:
  - the log's lookups that start a git fetch: 60 a minute per client
    address (the CF-Connecting-IP header), in the log server, not yet
    built;
  - a Cloudflare rate limiting rule on `sum.xolang.org/lookup/` and
    `/refresh/`;
  - git fetches: a stall limit, and the existing 500 MB cap per module
    version. Every git xo runs carries `http.lowSpeedLimit=1000` and
    `http.lowSpeedTime=60` (2026-11-08, cmd/xo TestGitStalledRemote): a
    transfer under 1 KB/s for a minute is abandoned, where a remote that
    stopped sending held a lookup, and the `--refresh` loop behind it,
    for good. A remote that drips just over 1 KB/s is stopped by a
    deadline per git command, `XO_GIT_TIMEOUT` (default 30m): xo kills
    the git it spawned (cmd/xo TestGitDeadline).

### Ed25519 key custody

- The owner runs `xo mod sumdb keygen sum.xolang.org
  /srv/xomod/log.key` on the host, as the `xomod` user. The file is
  mode 0600, and keygen never overwrites one.
- The verifier key it prints is the one compiled into xo as the default
  XO_SUMDB.
- The signer key must stay on the host, because the log signs every
  answer. The owner keeps one offline copy (a password manager entry or
  an encrypted file), which a move to another host needs. No other
  copy: not in the repository, not in R2, and not on the Mac.
- Compromise: there is no key rotation yet (decision 0124). A new key
  means a new log name (for example `sum2.xolang.org`), a release that
  changes the default, and a security note. A rotation ADR is a
  follow-up before the default flip.

### Defaults at the flip

- `XO_PROXY=https://proxy.xolang.org,direct`: a 404 falls back to git,
  as Go's GOPROXY default does.
- `XO_SUMDB=sum.xolang.org+<key id>+<public key>`, the key from the
  owner's keygen.
- XO_NOPROXY and XO_NOSUMDB stay empty by default. A follow-up adds
  `XO_PRIVATE` to set both at once, like Go's GOPRIVATE.
- Before the flip, these defaults are opt in through the environment.

### Policies

- Deletion and takedown:
  - Versions are kept indefinitely, so builds keep working when an
    origin disappears; that is the proxy's purpose.
  - A zip is removed from R2 only for a legal request or confirmed
    malware. The proxy then answers 404 and clients fall back to git.
  - The log's record is never removed: it is append only, and removing
    it would be a rewrite.
  - The contact address is the owner's choice.
- Size and rate: the 500 MB zip cap matches Go's and fits the CDN cache.
  The rate limits are listed above.
- Privacy note (published with the service):
  - Cloudflare sees client addresses and the module paths requested;
    the host sees only Cloudflare's addresses, through the tunnel.
  - The log and the proxy keep no request logs of their own beyond
    error counts.
  - Private modules belong in XO_NOPROXY and XO_NOSUMDB, which keep
    their paths from ever reaching either service.
- Index feed: `index.xolang.org/index?since=<RFC 3339 time>` lists the
  versions in log order as JSON lines `{"Path", "Version",
  "Timestamp"}`, as Go's index.golang.org does.
  - The log's records file gains a timestamp per record (a sidecar
    file, so the records stay the hashed bytes).
  - Answers are served through the tunnel with `public, max-age=60`.
  - Built in phase 1 (2026-11-08; `GET /index` of `xo mod sumdb serve`).

### Phases

1. Build (no cloud). Progress: `xo mod proxy fill` (2026-11-08), and
   the log's record copies, restore, times, index feed, rate limit and
   cache headers, and the log writing each version it adds to the proxy
   bucket (`--fill-to`), all 2026-11-08; the list refresh (`--refresh`
   and `/refresh/<path>`, cmd/xo TestChecksumLogRefresh) the
   retried uploads (`fill-pending`), the refresh backoff and
   `/healthz`, 2026-11-08; and `scripts/modservices/`
   (the `xo-sumdb` unit, cloudflared's ingress, `install.sh` and the
   owner's steps in its README), 2026-11-08. Phase 1 is complete. The
   owner's OCI host runs Oracle Linux 9 with SELinux enforcing:
   `install.sh` labels the release `bin_t` (systemd may not execute the
   `var_lib_t` default under `/srv`; it runs `bin_t` as
   `unconfined_service_t`) and the data `var_lib_t`, and the README lists
   the checks (`ausearch -m avc`). Tested in Debian 12 and Oracle Linux
   9.8 arm64 containers under systemd with a cross-built xo, a real
   module fetched with git and a PUT sink for R2; SELinux was checked
   against the targeted policy offline, not enforced, so the owner's
   first install is its first enforcing run.

   The items:
   - `xo mod proxy fill` with the R2 (S3) upload;
   - the records copy;
   - the log's rate limit;
   - the timestamp sidecar and the index endpoint;
   - systemd unit files and an install script, all tested against a
     local S3-compatible server and loopback.
2. Private beta, after the owner accepts this ADR:
   - xolang.org's nameservers move to Cloudflare (its mail records
     dropped, owner 2026-11-08);
   - the R2 bucket, the scoped token and the tunnel are created;
   - the owner generates the key on the host and the services are
     installed;
   - testers set XO_PROXY and XO_SUMDB by hand;
   - measure cache hit rates, requests per day and fill latency against
     this ADR's assumptions.
3. On-demand fill if the beta shows that falling back to git on a miss
   hurts: a Worker on proxy.xolang.org that serves R2 and, on a miss,
   calls the fill service through the tunnel. It is priced above; every
   request counts, since a Worker runs before the cache.
4. Default flip in a release: the defaults above, a CHANGELOG entry, the
   privacy note on xolang.dev, and the key rotation ADR landed first.

### Move-out plan

The state that matters is small:

- the records file and its timestamp sidecar;
- the signer key;
- R2, which does not move.

To move to Hetzner (CAX11 or CAX21) or another host:

1. Install the same units on the new host.
2. Stop the log on the old host and copy `/srv/xomod/log` and the key.
3. Start the log on the new host.
4. Move the tunnel's connector, the same tunnel with a new connector,
   so no DNS change.
5. Delete the key from the old host.

Clients see the same tree under the same key, so nothing breaks; the
log is unavailable only for the minutes of the copy.

Triggers for a move:

- Oracle reclaims or changes the instance or the tenancy;
- the existing workload needs the memory;
- the measured load outgrows 2 cores.

## Consequences

- The files cost nothing until the growth size and about $18 a month at
  100x. The fill service and the log cost nothing while the OCI host
  stays Always Free.
- xolang.org's DNS moves to Cloudflare, so Cloudflare holds the zone of
  the module services; xolang.dev and the website do not depend on it.
- The services depend on one host. Its failure stops new fills and log
  lookups:
  - versions already filled stay available from R2;
  - with the log down, first fetches fail closed with XO1314, by
    design;
  - the move-out plan above is the recovery.
- New code before the beta: the fill command, the records copy, rate
  limits and the index.

## Alternatives

- AWS (S3 + CloudFront, with Lambda or an instance for the fill): about
  the same at launch, about $13.50 at growth, about $1,000 a month at
  the popular size, almost all of it CloudFront egress. S3 to CloudFront
  is free, but CloudFront's egress is not. No reason to pay it when R2
  has no egress fee.
- A VM serving everything (Hetzner CAX11, or the OCI host itself with
  its 10 TB of free outbound data):
  - cheapest in money;
  - one server with no CDN, so a cold cache and DDoS exposure are its
    own;
  - the OCI host's 155 GB of disk covers the growth size but not the
    popular one;
  - kept as the fallback host for the fill service and the log, not for
    the files.
- B2 instead of R2 behind Cloudflare: free egress to Cloudflare and
  $6.95/TB, so cheaper storage at the popular size (about $7 against
  $15). It needs a public B2 bucket and Cloudflare in front for the
  domain, with two vendors to manage instead of one. Worth revisiting
  past the popular size.
- Only a subdomain on Cloudflare: Enterprise (subdomain zone) or
  Business ($200 to $250 a month, partial setup). The other route is
  Cloudflare for SaaS custom hostnames (100 free), which keeps the
  domain on Namecheap. It still needs another zone on Cloudflare and a
  Worker or an origin rule to reach R2. Moot now that the services have
  xolang.org to themselves.
- R2's r2.dev URL: rate limited and documented as non-production, with
  no cache rules. Rejected.
