# `xo check` with the module check cache and lazy runtime bodies (2026-10-23)

The b08 measurement (check-support-cost-2026-10-23.md) found a plain
`xo check` 5 to 8 times slower than a warm build's check phase. Two
causes, both fixed by decision 0351's 2026-10-23 addendum: `xo check`
never used the module check cache, and it checked every body of the
native runtime modules, which a build checks only when the SSA builder
needs them (decision 0247).

Development Mac (Apple M4 Pro, 24 GB), xo from this commit's tree,
`xo bench --ab --rounds=10`, A = XO_CHK_CACHE=off (the old behavior),
B = the default (warm cache), `xo check .`, as a bulk job.

## Step 1: the module check cache only

Loads 10.3 to 12.4 (small server) and 7.7 to 9.7 (compiler/xo).

| program | wall B/A (95%) | cpu B/A (95%) | peak RSS B/A |
|---|---|---|---|
| bench/perf/http/xo | 0.999 (0.956 to 1.009) noise | 0.981 noise | 0.999 |
| compiler/xo | 0.566 (0.542 to 0.581) better | 0.555 better | 0.788 better |

## Step 2: plus lazy runtime bodies (the change as pushed)

| program | A median | B median | wall B/A (95%) | cpu B/A (95%) | peak RSS B/A |
|---|---|---|---|---|---|
| bench/perf/http/xo | 591.5 ms | 255.3 ms | 0.432 (0.357 to 0.48) better | 0.412 better | 0.656 better |
| compiler/xo | 1,815 ms | 669.4 ms | 0.369 (0.365 to 0.373) better | 0.362 better | 0.669 better |

## Same output

`xo check .` with XO_CHK_CACHE=off, then twice with the cache (cold,
then warm), in a private cache directory: the output and exit status
are identical in all 76 directories tried (every test/errorcheck
program, every 8th golden of internal/llvm/testdata/golden,
compiler/xo, and std/, which keeps the eager check). cmd/xo
TestCheckCacheWarnings: a module's XO0540 warning prints on the
first and the cached check, after an edit of another module, and is
gone after its fix.

## `xo check --support` through the caches (b08 option C', same day)

`--support` kept the module check cache and the lazy runtime off and
built its SSA without the front end cache. It now keeps both and builds
through the front end cache (its own entries: the key is the support
check's, so a repeated `--support` check hits them). A = XO_CHK_CACHE=off
and XO_FE_CACHE=off (the old path), B = the default, warm, 10 rounds:

| program | A median | B median | wall B/A (95%) | cpu B/A | peak RSS B/A |
|---|---|---|---|---|---|
| bench/perf/http/xo | 926.9 ms | 355.6 ms | 0.384 (0.367 to 0.406) better | 0.365 better | 0.737 better |
| compiler/xo | 3,743 ms | 1,610 ms | 0.43 (0.416 to 0.449) better | 0.43 better | 0.848 better |

`xo check --support .` output and exit status: identical, A against B
cold and warm, in the same 76 directories.

Making the support check the default (option C) would add the
difference to every check: about +0.1 s to the small server's 0.26 s
and +0.94 s to compiler/xo's 0.67 s.
