# xo1 with a relative --std could not build the TLS library

Status: Observed, fixed (2026-10-12, decision 0178 stage 7) Obsolete since 2026-10-15: Mbed TLS removed (decision 0202).

Symptom: on a clean Linux machine (no TLS archive in the cache), the
self hosted `xo` run from the repository root with its default
`--std=std` failed to build compiler/xo: `the TLS library sources:
NotFound(path: "./third_party/mbedtls/xo/third_party/mbedtls/xo/xo_crypto_config.h")`
and `xo1: /usr/bin/cc failed`. The development Mac never showed it: its
TLS archive was cached (built once by the Go driver, same key), and
switch point A passes an absolute `--std`.

Cause: the repository root is `dir_name("std")`, that is `.`, so the
directory walked was `./third_party/mbedtls/xo`. The walk joins paths as
Go's `filepath.Join` does, which cleans away the leading `./`, and
`files_under` (compiler/xo/tools.xo) cut the walked paths by the
uncleaned prefix, so nothing was cut and the relative names held the
whole path a second time.

Fix: `files_under` cuts by the directory as given or, when the walk
returned the cleaned form, by the directory without its leading `./`.
Found by scripts/bootstrap-xo.sh (empty caches, relative std) on EC2.
Lesson: a cold-cache run (fresh `XO_TLS_CACHE`, `XO_RTOBJ_CACHE`,
`XO_BUILD_DIR`) is the only way to exercise the build-it-yourself paths
of xo1; the bootstrap script does that.
