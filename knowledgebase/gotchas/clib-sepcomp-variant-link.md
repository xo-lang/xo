# A C library build linked against std's program variant

Status: Observed (2026-11-08, decision 0574 stage 2 branch).

## Symptom

`xo build --lib=shared` (or `--lib=static`) of a package that built a
minute before failed at the link with dozens of undefined symbols, all
functions only a library reaches: `_xo_f_entry_2einit`,
`_xo_f_heapos_2ethread__heap`, `_xo_f_clibh_2eget`, referenced from
`cabi_c`'s `xo_init`, `xo_heap_attach` and `xo_release`. It then failed
every time with the shared cache and passed with `XO_BACKEND_CACHE=off`
or a fresh `HOME`. Earlier the same day it showed as a one-off
`TestCLibThreadExit` failure in a parallel test run.

## Cause

Separate compilation is the default (decision 0388): a native build
links against std's prebuilt variant from the std cache when one exists,
and starts the variant's build in the background when it does not.
`sepcomp_default` skips a cc link, but `--lib` sets its cc linker later,
in build_native, so a library build used std's variant whenever one was
cached. That variant is a program's: the runtime's library entries are
not roots there, so their callees were never compiled into it. A first
library build missed the cache, built whole (and worked), and started
the variant's background build; once that landed, the next library build
linked against it. `go test` sets `XO_SEPCOMP_BG=0`, so the tests alone
never made the variant, but a `--lib` build from a script did.

The front end cache's key also lacked the library mode (`fe_settings`),
so a stale entry of such a build, and a static and a shared build of one
package, shared keys.

## Fix

`sepcomp_default` clears every `XO_SEPCOMP*` variable for `--lib`
(`XO_SEPCOMP=1` included): a library is always built whole. The front end
key has `clib=` (library, static). TestCLibNoInit builds with
`XO_SEPCOMP=1` and a missing facts file.

To find this class of failure: a build that fails with the shared cache
and passes with `XO_BACKEND_CACHE=off` or a fresh `HOME` is a cache key
gap; `XO_CACHE_STATS=1` names the caches a build reads (here `cache stdx
record loaded`), and the per-cache switches narrow it.
