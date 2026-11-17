# xo test --since and cached --std-module runs on 5 std pushes (2026-11-17)

Decision 0701, task #250. The Mac (darwin/arm64), xo built from
~/xo-wt/l2-since; each push's tree checked out at the push, XO_STD its
std, the test binary warm (one run first). Wall seconds of one run;
"since" is `xo test --std-module=M --since=PUSH^ --count=1`, "full" the
same without --since, "cached" a second plain run (the stored result).

| push | module | full | since (tests) | cached |
|---|---|---|---|---|
| 6cee17f78 toml strict decode | toml | 0.10 s, 20 | 1.07 s, 20 | 0.05 s |
| 3010fba63 websocket join | websocket | 0.06 s, 7 | 0.19 s, 7 | 0.05 s |
| 0d9784ea9 protoparse rejects | protoparse | 0.11 s, 27 | 0.90 s, 24 | 0.05 s |
| 0076bbe94 amqp null key | amqp | 0.07 s, 26 (9 skipped) | 1.35 s, 18 | 0.06 s |
| 945fff5e6 amqptest credit | amqptest | 0.06 s, 5 (31 skipped) | 0.28 s, 4 | 0.05 s |

Findings:

- Each push changed its own module and its tests, so --since kept most
  tests (24 of 27, 18 of 26, 4 of 5, all of the others), and its git
  calls and parsing of the module's files (0.2 to 1.3 s) cost more than
  these modules' tests (0.1 s with a warm binary). Within a module whose
  tests take a tenth of a second, selection does not pay.
- What pays is not running a module at all: --since on a module the push
  did not touch selects nothing (0 tests, one git diff), and an unchanged
  module's --std-module run replays its stored result in 0.05 s where it
  ran every test before (every --std-module run was uncached: 0439 stage
  1). The build of the test binary, not measured here (warm), is what a
  cold gate pays per module; the replay skips the run, not the build
  check.
- Next: a gate step that lists the std modules a push reaches (the push's
  files against each module's files and its importers) and runs
  `--std-module=M --since` only for those, the rest replaying; that is
  where minutes are, not within a module.

## For xw-1's #251 (a gate step over the reached std modules)

From gate.sh, with BASE the push's merge base (`git merge-base HEAD
origin/main`) and XO the gate's built xo:

```
for m in $(MODULES_THE_PUSH_REACHES); do
  "$XO" test --std-module="$m" --since="$BASE" || exit 1
done
```

- `--since` exits 0 with `ok: 0 passed` when the push changed none of
  M's files and no source outside them; it runs every test of M when a
  source outside M's files changed (a shared module, prelude, compiler/),
  so it is safe to call for every module, but each call costs a git diff
  and, when M's files changed, a parse of them (0.2 to 1.3 s).
- Without `--since` (or for modules it decides to run in full), a plain
  `--std-module=M` run replays its stored result when nothing the build
  read changed (decision 0701), so modules a push does not reach cost
  about 0.05 s each after their first run on the host; keep the run
  cache (XO_BIN_CACHE, the build dir) across gates on a host to get it.
- Exit 2 means a usage problem (not a git checkout, an unknown REV).
