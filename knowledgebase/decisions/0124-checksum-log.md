# 0124: Checksum log (XO_SUMDB)

Status: Accepted (2026-10-10), implemented. Builds the optional public
checksum log of decision 0086. Code: `internal/sumdb` (tree arithmetic
`tlog.go`, keys and signed tree heads `note.go`, `client.go`,
`server.go`), `internal/mod/fetch.go` (`checkLog`, `LogRecord`,
`settingsFor`). Tests: `internal/sumdb/sumdb_test.go` (every proof of
trees up to 40 records, keys, forks, rollback),
`internal/mod/log_test.go` (TestChecksumLog,
TestChecksumLogThroughProxy), `cmd/xo/proxy_test.go`
(TestChecksumLogCommand). Spec: core 4.3, codes XO1313 and XO1314.

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
