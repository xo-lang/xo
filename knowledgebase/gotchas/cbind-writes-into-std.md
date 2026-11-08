# A `use c` binding of a std module is written into std

Status: Observed (2026-11-08, tier 60).

Symptom: internal/arm64 TestSepcompDefault failed in the full tier with
"the default build did not ask the std cache: std variant: miss, the
whole program is built" and passed alone. Its `xo dev stdcache --conc=0`
had built over std's tree at 340 files; `--conc=1` and the default build
a minute later saw 341, the newest
`std/native/.xo/cbind/c_xo_sqlite_045d638a.xo`.

Cause: a binding's cache is `<module root>/.xo/cbind` unless
`XO_CBIND_DIR` is set (compiler/cli/bind cbind.xo cache_dir), and std's
module root is std itself. A parallel test that built std/sqlite (a std
variant build binds it too) wrote its binding there, and std's tree key
(stdcache.xo std_tree_lines) counts every `.xo` file under std, so the
key changed between the test's variant build and its own build. Users
meet the same once: a release's std ships no binding, so a first
std/sqlite build writes into the installed std and the next default build
rebuilds std's variants.

Fix (tests): internal/xotool's init sets `XO_CBIND_DIR` to
`<user cache>/xo/test-cbind` when it is unset (bindings are named by their
content, so processes share it), and TestStdRecordBindRetry, which set it
empty, uses a directory of its own. A test that sets `XO_CBIND_DIR=""` on
purpose (the default location's tests) uses a copied std or its own
module. Users: the release's std to ship the std/sqlite binding prebuilt
(lead, 2026-11-08, after 0.44.0).
