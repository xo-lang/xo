# A directory's listing taken during a build is not the next build's

Status: Observed (2026-10-28)

**Symptom.** After recording each listed directory's digest as the build
read it (recorder.xo Rec.list_sums), so the run cache's entry would not
list std again, an unchanged rebuild missed the run cache every time
(cmd/xo TestWholeBuildCache: "cache hit false").

**Cause.** The build lists the module's directory before it writes its
own output there (`xo build -o prog .`). The digest the build saw lacks
`prog`; the next build's check (run_fetch, listing_of) sees it, so the
entry never matches. run_store's own listing, taken after the build,
always had the name ("its listing keeps the name").

**Fix.** run_store uses a build-time digest only for a directory the
build wrote nothing into (Rec.written and the outputs: directly in it for
a listing, anywhere under it for a walk); any other is listed again when
the entry is stored. A miss here is safe but silent: a cache that always
misses still builds correctly, so check a hit on an unchanged rebuild.
