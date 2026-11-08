# 0124: Checksum log (XO_SUMDB)

Status: Accepted (2026-10-10), implemented. Builds the optional public
checksum log of decision 0086. Code (since decision 0215, 2026-10-13, in
Xo): `compiler/cli/modules/sumdb.xo` (verifier keys and signed tree
heads, Ed25519 from std/crypto; RFC 6962 Merkle proofs; the client) and
`compiler/cli/modules/fetch.xo` (the log record of a fetched tree,
`checkLog`, and `XO_SUMDB` and `XO_NOSUMDB` in the fetch settings).
Spec: core 4.3, codes XO1313 and XO1314. The Go implementation this
decision first cited (`internal/sumdb`, its log server, and
`internal/mod/fetch.go`) was deleted with the Go compiler; so were its
tests (`internal/sumdb/sumdb_test.go`, `internal/mod/log_test.go`,
TestChecksumLogCommand: its loopback log server was the Go one; decision
0215, "Tests not carried over"). No public Xo log runs yet, so
`XO_SUMDB` defaults to `off`. Gap closed 2026-11-08: the server is Xo
(`compiler/cli/modules/sumserve.xo`, `xo mod sumdb keygen` and `xo mod
sumdb serve`, see "Server" below), and cmd/xo TestChecksumLog runs the
client end to end against it and `xo mod proxy serve --fetch` on
loopback (maturity-gaps.md 2).

## Context

xo.lock (core 4.3) pins the content of every version a module has
already built with. It cannot help the first time: whoever adds a
dependency, or bumps it, trusts whatever the repository or proxy serves
at that moment. A moved tag, a compromised mirror, or a targeted proxy
that serves one user a different tree goes unnoticed, and the bad hash
is then locked in. Go closes this gap with sum.golang.org: a public,
append-only, verifiable log of every module version's hash.

## Decision

Mirror Go's checksum database, simplified where Xo can afford it.

Configuration:
- `XO_SUMDB` is `off` (the default: no public Xo log exists yet) or
  `<name>+<key id>+<base64 Ed25519 public key> [<URL>]`; without a URL
  the log is `https://<name>`. The key id is the first four bytes of
  SHA-256(`<name>\n` || key), so a mistyped key fails to parse.
- `XO_NOSUMDB` lists path patterns (comma separated globs over leading
  path elements, as XO_NOPROXY and Go's GONOSUMDB) never looked up:
  private modules the log can never see, and whose paths should not
  leak to it.
- The client keeps the latest verified tree head per log in the module
  cache (`<cache>/cache/sumdb/<name>/latest`).

When: on the first fetch of each module version, from any source
(direct or proxy), before the tree enters the module cache. Versions
already cached are trusted as before (xo.lock is checked on every load).
Go skips the log when go.sum has the line; Xo checks every first fetch,
which costs one lookup per version per machine and also covers a cache
filled for an xo.lock written elsewhere.

Records and tree: a record is the version's two xo.lock lines
(`<path> <version> h1:...` and `<path> <version>/xo.mod h1:...`),
newline terminated. The log is an RFC 6962 Merkle tree (leaf
SHA-256(0x00 || record), node SHA-256(0x01 || left || right)), the same
hashing as Go's tlog and Certificate Transparency.

Signed tree head: the text `xo checksum log\n<name>\n<size>\n<base64
root>\n`, a blank line, and `sig <name> <base64(key id || Ed25519
signature of the text)>`. The shape follows Go's signed notes; the
header and the `sig` keyword differ on purpose, so neither format is
mistaken for the other.

Protocol (read only HTTP(S), below the log URL; path and version
escaped as in the module cache):
- `GET /latest`: the signed tree head.
- `GET /lookup/<path>@<version>`: `<record id>\n<record>\n<signed tree
  head>`. A log may fetch and add a version it does not hold yet; 404
  or 410 when it cannot.
- `GET /proof/inclusion/<id>/<n>`: the audit path of record id in the
  tree of n records, base64 hashes one per line.
- `GET /proof/consistency/<m>/<n>`: the proof that the tree of m
  records is a prefix of the tree of n.

Proof endpoints replace Go's tiles: simpler to serve and to read, at the
cost of cacheability (see Alternatives).

Client check, in order: the lookup's tree head verifies under the
configured key; the record's lines equal the hashes of the fetched tree;
the record id is inside the signed tree and its inclusion proof leads
to the signed root; the signed tree and the stored one are consistent
(equal roots at equal sizes, a consistency proof from the smaller to the
larger otherwise, in both directions, so a log that shrank is caught);
the larger head is stored.

Failures:
- XO1313 (security): the log records other hashes, a signature or proof
  does not verify, or the log contradicts a tree seen before (a fork or
  a rewrite). Nothing is cached.
- XO1314: the log cannot be consulted (unreachable, no record, malformed
  answers) or the settings are malformed. The fetch fails closed: an
  unverified tree is never cached while XO_SUMDB is on.

Server: `sumdb.Server` keeps records in memory, signs on each request,
and adds unknown versions through a lookup function (`mod.LogRecord`
fetches from git into its own cache). It is what tests run and the
starting point for a team's log; persistence, key management commands,
and a public deployment are not built.

## Addendum (2026-11-08): the log server in Xo

The Go server went with the Go compiler (decision 0215), which left the
client with no end-to-end test. `compiler/cli/modules/sumserve.xo` ports
it (the protocol above, unchanged) as two commands rather than a test
only server: a team's log and any public one run the same code the tests
run, and the commands sit beside `xo mod proxy serve` (decision 0126).

- `xo mod sumdb keygen <name> <key file>`: a new Ed25519 key (std/crypto
  random_bytes), the signer key `PRIVATE+KEY+<name>+<key id>+<base64
  seed>` written to the file with mode 0600 and never over an existing
  file, the verifier key printed for XO_SUMDB.
- `xo mod sumdb serve --key=<key file> [--addr=127.0.0.1:7071] [--fetch]
  <data dir>`: the records persist in `<data dir>/records` (two lines
  per record, in log order), each appended and synced before any tree
  head holding it is signed, and read back at start (a torn last record
  stops the start rather than being dropped). An in-memory log that
  restarts would serve a smaller tree, which every client that saw the
  larger one reports as a rewrite (XO1313), so persistence is part of
  the server rather than a later feature. `--fetch` adds a version the
  log lacks with a direct git fetch into `<data dir>/cache` (never
  through XO_PROXY), as `xo mod proxy serve --fetch` fills its cache.
  The state is behind one Mutex; the git fetch runs outside it, and the
  add re-checks the index inside it.
- Tests: sumserve_test.xo (inclusion and consistency proofs for every
  tree of 1 to 17 records verify with the client's checks, and a wrong
  leaf or a rewritten prefix does not; signed heads verify with the
  verifier and not with another key; malformed signer keys refused).
  cmd/xo TestChecksumLog, through `xo mod proxy serve --fetch` and the
  log on loopback: fetch, verify and lock; a tampered tree from the
  proxy is XO1313, and with XO_NOSUMDB naming it XO1304 (xo.lock);
  XO_NOPROXY fetches from git with XO_PROXY=off; a restarted log on the
  same data serves a tree the stored head extends; a tag moved after the
  log recorded it is XO1313; a log under the same key that lost its
  records is XO1313 (its tree does not extend the stored one).
- Still not built: key rotation, tiles, gossip, a proxy passthrough for
  the log, and a public deployment (the owner's decision).
- Phase 1 of decision 0568 (2026-11-08) adds, for a public log:
  - `--copy-to=<bucket URL>`: each new record is PUT to
    `records/<10 digit id>` as `<unix time>\n<record>` before it is
    appended locally, so a host's loss loses nothing a head signed;
    `xo mod sumdb restore --from=<bucket URL> <data dir>` rebuilds the
    records and times files from the copies (into an empty directory);
  - `<data dir>/times`, the Unix time of each record (a log from before
    reads its records as time 0), and `GET /index?since=&limit=` (at
    most 2,000) as JSON lines `{"Path","Version","Timestamp"}`, cached
    60 s;
  - `--rate=N` (default 60): lookups that add a version, per client and
    minute, 429 past it; the client is CF-Connecting-IP when the peer is
    loopback (a local Cloudflare Tunnel), else the peer's address;
  - Cache-Control `no-store` on `/latest` and lookups, immutable on
    proofs;
  - `--fill-to=<bucket URL>`: each version the log adds is written to the
    proxy bucket from the tree already in the log's cache (fill_files,
    then the module's list), in a background task, so the first fetch
    anywhere fills the public proxy; a failure is logged and a later
    `xo mod proxy fill` repairs it.
  cmd/xo TestChecksumLogOps checks each against an in-test S3 fake.

## Threat model

Protects against:
- A repository whose tag moved (or was force pushed) after the log
  recorded the version: every later first fetch fails, xo.lock or not.
- A malicious or compromised proxy or mirror serving altered trees, to
  everyone or to a targeted user: the log's record differs.
- A log operator (or an attacker holding the log's key) showing
  different histories to different clients: each client's stored tree
  head forces consistency proofs, so a fork is detected by any client
  that saw both sides; gossip between clients (not built) would extend
  that to clients that saw only one side each.
- A log that drops or rewrites records: consistency with the stored
  head fails.
- Tampering on the network path to the log: tree heads are signed and
  proofs checked against them, so plain HTTP works for integrity.

Does not protect against:
- A version that was malicious when it was first logged: the log proves
  everyone gets the same content, not that the content is good.
  Effect ceilings (decision 0086 step 3) and review address that.
- The first contact of a fresh machine with a log that is already forked
  for it: there is no stored head yet (Go has the same gap; gossip or a
  pinned head would close it).
- Denial of service: an unreachable log stops new fetches (XO1314);
  XO_SUMDB=off or XO_NOSUMDB is the escape hatch, by choice of the user.
- Privacy: lookups reveal module paths to the log; XO_NOSUMDB keeps
  private paths out. A proxy passthrough for the log (as Go's
  `/sumdb/<name>/...` on proxies) would hide the client's address; not
  built.
- Compromise of the client's own module cache (it holds the stored head
  and the trees); the cache is the trust root of the machine, as in Go.

## Consequences

- A team running a log gets Go-level first fetch integrity; with
  XO_SUMDB off nothing changes.
- Every first fetch with the log on costs a lookup and one or two proof
  requests (a few hundred bytes each).
- Codes XO1313 and XO1314 are new; XO1304 keeps its meaning (xo.lock).

## Alternatives

- Go's tiles (`/tile/H/L/N`): immutable, cacheable by any HTTP cache,
  and the client computes proofs itself. Better for a public log under
  load; more code on both sides. The protocol can add tile endpoints
  later without changing records or signatures.
- Reusing Go's note and tlog formats byte for byte (and sum.golang.org
  conventions): would invite pointing XO_SUMDB at Go's log, whose
  records are Go hashes of Go zips, meaningless for Xo trees.
- Trusting the proxy's `.lock` answer (decision 0126): the proxy is
  exactly what the log must not have to trust.
- Signing module versions by their authors (sigstore style): proves
  who, not that everyone sees the same thing; complementary, not built.
