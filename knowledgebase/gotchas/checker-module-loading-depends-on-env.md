# The Go checker's module loading depends on the environment

Status: Observed (2026-10-10)

## Symptom

`xo typedump internal/mod/testdata/repos/github.com/acme/billing@v1.2.0/invoice`
took 0.45 s and reported XO1302 `cannot fetch github.com/acme/money v1.1.0:
git clone https://github.com/acme/money.git: fatal: could not read Username
...`: TestSelfhostChecker ran `git clone` against github.com on every run,
and the message (and whether there was one) depended on the network, the
user's module cache, and git's credential setup. Comparing it with the
self hosted checker was impossible: the Xo checker does not fetch.

A second dependency: the absolute paths inside module messages (`module
cache: /...`, `record it in /.../xo.lock`, `no xo.mod with a module line in
/...`) come from `filepath.Abs`, which uses `os.Getwd`, which returns
`$PWD` when it names the working directory. In a `t.TempDir()` on macOS
that is `/var/folders/...` or `/private/var/folders/...` depending on how
the directory was entered.

## Cause

internal/check calls `mod.Load` for every main module it finds (decision
0086), and `mod.Download` fetches a missing version from git unless
`XO_OFFLINE` is set; the cache is `XO_MODCACHE` or the user cache
directory; `xo.work` is looked for up to the file system root unless
`XO_WORK` is set.

## Fix

The differential tests pin the environment for both checkers
(`setModEnv` in internal/selfhost/check_test.go): `XO_MODCACHE` is the
fixture cache `internal/selfhost/testdata/modules/cache`, `XO_OFFLINE=1`
(a missing version is XO1303, deterministic), `XO_WORK=""`. The Xo
checker gets the Go side's `os.Getwd()` as `--wd=DIR` (default `$PWD`), so
both make the same absolute paths. Any test that checks a module with
`require` lines should set `XO_OFFLINE` and `XO_MODCACHE`.
