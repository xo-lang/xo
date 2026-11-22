# A "cold" xo build with an empty back-end cache still hits the run cache

Status: Observed (2026-11-22, task #687, host 1).

**Symptom**: bench/perf/twins/build.sh's cold build of the CRUD server
took 100 ms and 23 MB on its second run on a host, against 3030 ms and
748 MB on the first, with `XO_BACKEND_CACHE_DIR` an empty directory both
times. The one function edit then took 2790 ms, longer than the "cold"
build.

**Cause**: xo keeps a run cache of whole linked binaries (decision 0360,
`XO_BIN_CACHE`, on by default, in the user cache) apart from the back-end
object cache. An empty `XO_BACKEND_CACHE_DIR` does not touch it, so the
first run's binary answered the second run's cold build. The edit missed
the run cache and found the back-end cache empty, so it built every part.

**Fix**: a timing run gives every xo cache its own empty directory:
`XO_BACKEND_CACHE_DIR=DIR/backend XO_BIN_CACHE=DIR/bin`, as
scripts/release-vs-go.sh does (`XO_BIN_CACHE=off` to measure without it).
twins/lib.sh's `xobuild` does both, and lib_test.sh checks it. A cold row
under about a second for a program that took seconds before is the sign.
