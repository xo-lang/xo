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

## Correction, same day: the walk could not cross into an implementation body

The counts above under-count, and this section says how. `loadDecls` read
std/*.xo plus std/native/prelude.xo, and every other module's implementation
lives in std/native/<module>.xo. A front declaration has no body edges of
its own, so the walk stopped at it: a declaration called only from a body
read as unreached, and a front declaration reached from a root never
credited anything its body calls.

The first half of the fix links a front declaration to its body
(`implAliases`: same module, symbol and kind, positions read from
std/native/*.xo and described there). A second half was needed for the same
reason one level down: a declaration that exists only in the body file, with
no front twin, is in no list the walk visits, so an edge into it marked
nothing and its own calls were dropped again. std/native/sql.xo's `Pool`,
with `take`, `give`, `ping`, is the case that exposed it: `Driver.connect`,
`DriverConn.ping`, `close` and `is_broken` are called only from those
methods. `loadImplDecls` makes every body declaration a node of the walk
(unlisted and uncounted: the reported set stays the API surface), and reads
std/native/llrt/*.xo too, because a body calls the runtime helpers there and
they call back into declarations (`Int.wrapping_shl` from the allocator,
`prelude.fault`); an llrt file's module is its base name, which can equal a
front file's (std/trace.xo against std/native/llrt/trace.xo), so llrt
declarations never join an alias pair.

Measured on the four dumps still on disk (the sql, prelude, http and
prelude-stream programs, dumped the same day), uncovered 1357 to 1257 in
three steps: the alias 1357 to 1316, 44 declarations, the body-only nodes
1316 to 1265, 51 more (52 lines changed, one is an `[impl]` mark and not a
new reach), and the llrt nodes 1265 to 1257, 8 more, all prelude
(`Int.mul_hi_unsigned`, `Int.shr_logical`, `Int.wrapping_shl`, `List.insert`,
`prelude.fault` and the four lowered `int_*`/`fault` helpers that
std/native/llrt/alloc.xo and asn1.xo reach). 100 declarations are newly
reached in all, among them
`sql.Driver.connect`, every `sql.DriverConn` method,
`sql.Row.items`, `prelude.read_or`, `read_text_or`, `Str.trim`,
`starts_with`, `contains`, `is_empty`, `replace`, `List.is_empty`, `pop`,
`Mutex.new`, `with`, `with_read`, `Chan.send`, `recv`, `Atomic.set`,
`Fs.read`, `read_text`, `float_parse`, `task.is_canceled` and the fs_*
helpers. The same mechanism explains the two `prelude.read_or` entries: it
is declared twice, front and body, keyed by path:line, so one was reached
and its twin printed as unreached.

The fix (internal/stdcoverage implAliases and loadImplDecls, with
TestUncoveredResolvedCrossesImplementationBodies covering the chains front ->
body-only helper -> declaration and front -> llrt helper -> declaration, and
the llrt same name collision) landed as 4ff76b0e2 and its follow-up. The numbers in the
sections above stand as the pre-fix measurement of that sweep; recomputing
them needs the edges again, and the sweep deleted each shard's .tsv after
comparing it, so the next audit is a full second sweep with the fixed tool.
Keep the dumps until the numbers are superseded.

## What the same-day corpora already closed, and what the cold paths really are

Landed the same day (main 83f5ae058): a prelude test for `Stream.read`,
`write` and `close` through a dialed and an accepted connection; a new http
corpus that calls `status`/`display` through a value typed
`http.StatusError`; a new metrics corpus (the module had none); and a sql
corpus that writes a Driver and drives it through `open_driver`, so
`Driver.connect` and every `DriverConn` method run (the pool calls
`is_broken` on every connection it is given back and `close` whenever it
keeps none).

With the fix, three of the remaining cold paths stop being cold without any
new program: `prelude.read_or` and `read_text_or` (their twins, and the
corpora that call them), and `prelude.texts`, which `llrt_env_args`,
`llrt_env_get` and `llrt_env_vars` call and programs do reach through
os.env (`corpus/cli`, `golden/closure_env_frame`, `golden/fs_native`).
`llrt_concat_int` stays: the compiler emits its call, so no site names it and
it is allow-listed already, as `texts` is. `internal.SharedLock.unlock` is
called only by `compiler/xo/gobackend.xo`, and the sweep's roots are the
corpora, the goldens, bench/perf and scripts, never the toolchain: it is a
documented over entry with that reason, not a corpus gap, and the sweep's
definition stays fixed rather than grow a toolchain root (lead,
2026-10-26). `go.Error.message` had no caller anywhere; golden/go_url now
calls it (below).

## Correction, same day: `xo dev edges` lost every `use go` program silently

The sweep ran `xo dev edges` once per program directory and took each
directory's quiet output as that program's references. A program with `use go`
whose binding is not in the directory's `.xo/gobind` stops the check before
names resolve; `xo dev edges` then wrote only the standard library's
references and exited 0, with nothing on standard error. Every `use go` golden
was therefore measured as a program with no references of its own
(`internal/gen/testdata/golden/go_url`, `go_regexp`, `go_strconv`,
`go_sha256`, `go_hex`, `go_uuid`, `go_strings`, `test_go_interop`, and the
replay programs; those that commit their `.xo/gobind` were measured, which is
why the loss was invisible).

So the earlier reading of `go.Error.go_type` here ("its edge resolves to
something other than std/go.xo:19") was wrong. With the binding present the
edge is exactly `internal/gen/testdata/golden/go_url/main.xo:24 ->
std/go.xo:19 Error.go_type`, measured on 2026-10-26 with
`xo bind --dir=<copy> net/url` and one `xo dev edges` run; `go.Error.display`
and `go_type` are reachable and only `message` lacked a caller.

`xo dev edges` now exits 1 with the reason when its program does not check
(compiler/cli/servers/agent.xo, Session.stopped), held by
TestDevEdgesMissingBinding: before the change the test read
"17989 lines, all std, and exited 0". golden/go_url also prints
`e.message()` now, so `go.Error.message` has a caller and its want.txt carries
the line (`message: parse "http://[::1": missing ']' in host`, the same text
as `display`, which is what std/go.xo says it is).

The second sweep therefore has to bind first: `xo bind --dir=DIR <imports>`
for each `use go` program before its `xo dev edges` run, and the binding
removed after, or the sweep reports those programs as failures instead.

## Second sweep, 2026-10-27: the whole surface with the fixed walk

The full recompute the corrections above called for. Every program
`scripts/stdcoverage -programs` lists on the tree got one `xo dev edges`
run, seven dump shards and seven comparison shards (`-edges` with an empty
dirs file, so the uncovered sets intersect), 234,335,620 edge rows, 1.6 GB
kept as .tsv.gz until the numbers were written and then deleted.

- Tree `fe1a63473`, the xo built in pi-deepseek-33 at 2026-10-26 19:38
  (`xo 0.32.0 darwin/arm64`), so the walk has 4ff76b0e2's front/body and
  body-only nodes and the llrt files.
- Bind first: `xo check DIR` for the 17 programs whose `use go`/`use c`
  binding was not committed; it created 15 `.xo/gobind` trees, no failures
  (`/tmp/cov3/bindings.txt`, log `ds2-bind2-cmd.log`). The binding trees
  were removed after the sweep. They also landed in the program list,
  which was regenerated after binding: the sweep ran 926 entries, the 907
  programs of that tree plus 19 generated `.xo/gobind` directories. The
  binding dumps reach none of the 155 declarations below (checked by
  taking the target name column of all 19 dumps, 9,235 distinct names,
  against the 155), so the counts are the program-only counts.
- 1463 declarations.
- Name scan over the same 926 dirs: 129 uncovered, 128 of them in
  allow.txt. The one outside it is `prelude.merge_end_past`, the sort
  merge's fault helper for a caller that broke its `hi <= len` clamp, which
  no program reaches because every caller in std clamps first: this tree
  left TestStdAPICovered red in `-short` and a1477fdf0 added the allow
  line, so a tree with that commit has all 129 allowed.
- Resolved edges: 155 uncovered, 1308 reached (89.4%).
- The scan credits, no edge reaches: 26 (13 marked `[impl]`).
- The scan misses, an edge reaches: 0.

Against the 187 uncovered and 61 over of 2026-10-26 (890 dirs, before the
walk fix), the uncovered set fell by 32 and the over set by 35. The 155 by
module: prelude 120 (112 `llrt_*` helpers, the compiler's emitted targets,
plus `int_range_list`, `list_sort_str`, `merge_end_past`, `slice_check`,
`slice_fail`, `str_eq_lower`, `str_prefix7`, `texts`), testing 10,
internal 7, debug 5, time 3, toml 2, and one each big, crypto, go, http,
netip, tls, websocket, xml. Fifteen modules that had uncovered
declarations on 10-26 now have none: sql (9 then), and base32, base64,
cli, compress, csv, mail, metrics, mime, multipart, regexp, smtp,
template, url, uuid (one each then), which is that day's corpora and the
driver and metrics programs showing up in the walk.

The 26 the scan credits and no edge reaches keep the reasons the section
above gives, now much shorter: 13 interface-named `display`/`debug`
methods (big.BigInt.debug, and display on crypto.KeyErr, go.Error,
http.TlsErr, netip.Prefix, time.Date, time.Zone, time.ZoneErr, tls.TlsErr,
toml.DecodeErr, toml.TomlErr, websocket.WsErr, xml.XmlErr), the 10
`testing` fakes (FakeEnv.home/program/vars, FakeStdio.eprintln/lines/
print/read_all/write, MemFs.cwd/cwd_path), `internal.SharedLock.unlock`
(a toolchain caller), `prelude.texts` (called from the lowered `llrt_env_*`
helpers) and `prelude.llrt_concat_int` (emitted by the compiler). Every
other `.display` the 10-26 list named as over is reached now: the corpora
call them through an interface-typed value.

Three limits, stated so the numbers are not read as fresher than they
are. (1) The tree is `fe1a63473`; main was `a5a0bf790` when this section
was written, and its scan reports 1465 declarations and 131 uncovered,
all 131 allowed, so the edge numbers are two declarations behind and the
list may miss a declaration added after 19:38. (2) One comparison job
started as six shards in one process group was stopped by the memory
reaper at 16.4 GB (`ds2-cmp2to7-cmd.log`); one shard per job stays far
under it, which is how the rest ran. (3) The dumps are deleted; the next
audit is another full sweep, and it should regenerate the program list
after binding so the 19 generated directories do not appear in it.

## Closing the 26, 2026-10-27

Ten test only commits close 13 of the 26: exactly the sweep's 13
interface-named `display`/`debug` entries, `big.BigInt.debug` among them.
They came in two shapes.

- A corpus call on the concrete value: `netip.Prefix.display`
  (67e01a32b, compared with Go's `Prefix.String()`), `big.BigInt.debug`
  (75127fe2c, the decimal text Go's `z.Text(10)` gives), `crypto.KeyErr`
  (afc79717c), `http.TlsErr` (5c15be995) and `time.Date`, `time.Zone`,
  `time.ZoneErr` (5607dd065; the ZoneErr message is std.md 2891's, and the
  negative-year Date, which `date()` cannot produce because it refuses
  years outside 1..9999, is compared with Go's
  `time.Date(-44, 3, 15).Format("2006-01-02")`), all compared with the
  message the error carries, and `xml.XmlErr` (99f282ee8, Go's
  `XML syntax error on line N: msg`).
- A golden that already printed a concrete error through `${e}`, rewritten
  as `${e.display()}`: toml `DecodeErr`/`TomlErr` (01403e8f7), websocket
  `WsErr` (5fb1d3227), tls `TlsErr` (73263cba2) and go `Error`
  (8c6cf2f89). `display` prints exactly what the interpolation printed, so
  want.txt did not change; the go_url golden first failed on the stack's
  `main.xo:44`, because three comment lines above the failing `?` moved it
  to 46, and the fix keeps origin/main's line count.

Each commit carries the same proof: one `xo dev edges` run over the program
that changed, merged with `scripts/stdcoverage -edges`, with the module's
declarations absent from both the over and the under list. A `use go`
golden needs `xo check DIR` first, which writes `.xo/gobind`; the generated
tree was deleted after the proof and is not committed.

The 13 that remain are the ones no program can reach, and the reasons are
now measured rather than assumed.

- the 10 `testing` fakes (FakeEnv.home/program/vars, FakeStdio.eprintln/
  lines/print/read_all/write, MemFs.cwd/cwd_path): nothing in std calls
  them; `cmd/xo`'s own tests do, through the test runner, and a corpus
  program is not that caller.
- `internal.SharedLock.unlock`: a toolchain caller, and it is in
  `internal`, not in the std API surface a program imports.
- `prelude.texts` and `prelude.llrt_concat_int`: compiler-emitted. On
  2026-10-27 a probe program that calls `os.env.get` with a non-constant
  name, `os.env.vars` and `os.env.args`, concatenates an `Int` into a
  string and prints through `os.stdio` was dumped with `xo dev edges`
  (`/tmp/ds2-prelprobe2.tsv`, 254,525 rows) and merged the same way. It
  reaches 37 declarations, among them `prelude.env_get`, `prelude.env_vars`,
  `prelude.env_admits` and the lowered `prelude.int_*` helpers, and it
  reaches neither `texts` nor `llrt_concat_int`: the call from the Env
  implementation to `llrt_env_get`, which is what calls `texts`, is
  synthesized by the compiler and exists in no source call graph, so no
  site edge can enter it. The same reason covers the 112 `llrt_*` helpers
  the name scan already allows.

So the next audit should read 26 over to 13, with nothing newly uncovered.
The 13 are a walk limit, not a test gap, and adding them to `allow.txt`
would state the opposite of what they are: reachable at run time, invisible
to a source level walk.

`scripts/stdcoverage -check` passes on main `8c6cf2f89` (2026-10-27): 1465
declarations, 131 uncovered, all 131 already in `allow.txt`. That run is the
baseline the next drift is measured from.
