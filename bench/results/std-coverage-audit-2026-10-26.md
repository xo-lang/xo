# The std coverage audit: the resolved edges against the name scan

2026-10-26, darwin/arm64. The sweep ran in worktree pi-deepseek-4 at
commit 7c4270d85 with the xo built there at 02:44 (xobuild key
76acbd0d2c7f5ab1), so it does NOT include e8fe6f28f (the corpora for
Int64.try, UInt32.try, Set.is_empty and log.error, picked at 10-26 from
02:58 to 03:30); the three programs that commit added or changed were
re-swept afterwards with the current xo and this file's counts are the
merged result. This is the periodic audit decision 0381 describes, run for
the first time over the whole sweep. The gate stays the name scan (seconds,
and the safe direction: it over-credits, never under-credits, measured
below); this audit is what checks the scan's credits.

## Method

`go run ./scripts/stdcoverage -programs` prints the 890 program
directories the sweep covers (the corpora, the goldens, bench/perf,
scripts). `xo dev edges` on each, one .tsv a program, is 3.5 hours and 22
GB of text (1.5 GB gzipped). The comparison ran as ten shards of about 90
programs, one gate job a shard, about two minutes each: decompress the
shard's parts, `scripts/stdcoverage -edges=SHARD -dirs=/tmp/cov/nodirs.txt
-list=DIR`, delete the parts.

The empty dirs file is what makes the shards merge: with it the name scan
covers nothing, so each shard's `-list` `uncovered.txt` is the set of
declarations that shard's edges do not reach. A declaration is reached by
the whole sweep exactly when one program's own edges reach it (the roots
are the non-std site paths and the walk only crosses edges out of a root
and then through std bodies, and two programs share only std nodes), so
the reached sets union and the uncovered sets intersect. `over` is then
the uncovered set minus the name scan's own uncovered set, which was read
from one `-dirs` run over all 890 dirs. The `[impl]` mark travels with
each entry in the lists, so it survives the merge.

## Result

- 1455 std declarations (1453 distinct module.symbol keys: `read_or` and
  `read_text_or` each name two declarations).
- Name scan over the same 890 program dirs: 126 uncovered, all 126 in
  `internal/stdcoverage/allow.txt`, `-check` exit 0.
- Resolved edges, 219 million rows plus the re-sweep below: 187 uncovered,
  1266 reached (87.1%).
- The scan credits, no edge reaches: **61**. The scan misses, an edge
  reaches: **0**. The scan's covered set is a strict superset of the
  edges', as it was on the 822-dir run of 2026-10-25 (129 over, 0 under).

187 uncovered by module: prelude 121, testing 10, sql 9, debug 6,
internal 5, http 4, time 4, big 3, go 3, toml 2, netip 2, and 16 modules
with one each (base32, base64, cli, compress, crypto, csv, mail, metrics,
mime, multipart, regexp, smtp, template, tls, url, uuid, websocket, xml).

## The six the sweep's base missed, and the re-sweep

The sweep's xo was built at 02:44 from pi-deepseek-4 at 7c4270d85, so it predates
e8fe6f28f: the corpora for `Int64.try`,
`UInt32.try`, `Set.is_empty`, `log.error` and the sql array and float
values were not in it. The three programs that commit added or changed
(`internal/stdtest/testdata/corpus/prelude`, `internal/stdtest/testdata/
corpus/sql`, `internal/llvm/testdata/golden/log_error`) were re-swept with
the current xo (built from this tree, xobuild key 99531926d00dcef4; three
`xo dev edges` runs, about 252k rows each, then one `-edges` run with
`-list`, log `~/xo-lead/agents/ds2-recheck-*-cmd.log`). Six declarations
are now reached that the sweep's base left uncovered: `prelude.Int64.try`,
`prelude.UInt32.try`, `prelude.Set.is_empty`, `log.error`, `sql.Row.ints`
and `sql.Row.strs`. Uncovered 193 to 187 and over 67 to 61; nothing became
newly uncovered, so the merge stayed sound.

## The 61 the scan credits and no edge reaches, with reasons

### Interface dispatch, 41 (marked `[impl]` in the audit's lists)

A call through an interface resolves to the interface's method, never to
the implementation, so an implementation looks unused:

- `display` on the error types, 30: base32.DecodeErr, base64.DecodeErr,
  big.BigErr, big.BigInt, cli.CliErr, compress.CompressErr, crypto.KeyErr,
  csv.CsvErr, go.Error, http.CookieErr, http.TlsErr, mail.MailErr,
  mime.MimeErr, multipart.MultipartErr, netip.ParseErr, netip.Prefix,
  regexp.RegexErr, smtp.SmtpErr, template.TemplateErr, time.Date,
  time.ParseErr, time.Zone, time.ZoneErr, tls.TlsErr, toml.DecodeErr,
  toml.TomlErr, url.EscapeErr, uuid.UuidErr, websocket.WsErr, xml.XmlErr
  (all `.display`).
- the testing fakes, 10: FakeEnv.home, FakeEnv.program, FakeEnv.vars,
  FakeStdio.eprintln, FakeStdio.lines, FakeStdio.print,
  FakeStdio.read_all, FakeStdio.write, MemFs.cwd, MemFs.cwd_path.
- `big.BigInt.debug`.

The reason `http.StatusError.display` is in the list below and not here:
its name matches no covered interface method.

### Corpus gaps, 9 (a program would reach them)

- `sql.Driver.connect`, `sql.DriverConn.begin`, `close`, `commit`, `exec`,
  `is_broken`, `ping`, `query`, `rollback`: the driver interface. The
  in-memory and Postgres drivers are std modules, so no program in the
  sweep implements a driver. A corpus program that implements one, over
  the in-memory driver's own tests, is the fix (largest of these).
- `sql.Row.ints` and `sql.Row.strs` were in this group before the
  re-sweep: the sql corpus of e8fe6f28f reaches both now.

### Cold paths no program here drives, 11

`go.Error.message`, `go.Error.go_type` (only a `use go` program's error
text), `http.StatusError.status` and `display`,
`internal.SharedLock.unlock` (the pair of the allowed `lock_shared`),
`metrics.latency_buckets` (a default parameter of `Registry.histogram`),
`prelude.Stream.close`, `prelude.llrt_concat_int`, `prelude.read_or`,
`prelude.read_text_or`, `prelude.texts`. The four that e8fe6f28f's corpora
reach (`Int64.try`, `UInt32.try`, `Set.is_empty`, `log.error`) left this
group in the re-sweep.

## What the numbers mean

- The gate stays the name scan: it is seconds against 3.5 hours, and its
  error direction is the safe one. Edges never credit a declaration the
  scan calls uncovered (0 of 126), so switching would only add work.
- The audit's product is this list, not new `allow.txt` lines: allow.txt
  means "uncovered and accepted", and 20 of the 61 are not accepted, they
  are dispatch or a corpus gap to close.
- The next audit is worth running after the sql driver program (a
  measurement, not a gate): it should show 61 down to about 52, with 0
  under.
