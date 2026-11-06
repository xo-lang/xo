# 0526: `xo lsp` checks through the module check cache

Status: Proposed (xo-lang-4, 2026-11-05; decision 0283 stage 1b, item A
of the lead's queue: "make xo lsp share serve's warm state")

## Context

Decision 0283 stage 1 (e849730f5) lets a running `xo serve` answer the
CLI lookups (`xo def`, `refs`, `type`, `symbols`, `doc`) from its
response cache. The editor's server, `xo lsp` (decisions 0114, 0219,
0284), shares none of that: each open directory is checked in a worker
task (0284) from source, dependencies included, on every edit.

Measured on the Mac (darwin/arm64, main 9b1973a4a, lspbench driver,
compiler/ssa/build.xo open, 2026-11-05): first diagnostics 468 to 544
ms, diagnostics after a one-line edit about 710 ms, hover and definition
1 ms, references 622 to 733 ms (142 to 148 ms since b7edfe4cf), RSS
1.64 GB.

Two ways to share warm state were on the table:

1. `xo lsp` asks a running `xo serve`. serve holds no checked program:
   it caches each command's response, stamped by the directories the
   command read, and runs a child `xo` on a miss (server RSS 8 to 16 MB).
   Every edit changes a stamped directory, so every edit is a miss and a
   cold child check (`xo check compiler/ssa`: 1.13 s), slower than the
   lsp's own worker, and the child cannot see unsaved text.
2. `xo lsp` checks through the module check cache that `xo check`
   already uses (decision 0351's machinery; compiler/cli/core/check.xo
   load_walked). An entry is keyed on the module's file texts, its
   import closure's texts and std's, and the check's flags
   (modcache.xo mod_cache_load), so a module with no edited file keeps
   its key and is replayed (diagnostics, scans, the eager bodies) instead
   of having its bodies checked. `xo check compiler/ssa` on the Mac: 1.13
   s with the cache off, 0.42 s with it (3 runs each, 2026-11-05).
   `xo check` leaves the cache out when there is an overlay
   (`overlay.is_empty()`), and `xo lsp` never sets it.

## Decision

Option 2. `xo lsp`'s per-directory check (check_view_walk) sets the
module check cache as `xo check` does, with the same directory and
flags (`xo check` plus the toolchain id and std), so the editor and the
CLI read and write one cache:

- a module none of whose files is open in the editor (in the overlay)
  may be skipped when its entry is present: its diagnostics come from the
  entry, its declarations from the load, its bodies are not checked;
- a module with an open file is always checked in full (its bodies give
  hover, definition, references and completion their records, and its
  text may be unsaved); its entry is not stored while a file of it
  differs from the disk, so the cache never holds an unsaved state;
- references and rename, which search every user module's bodies (the
  ALL key), keep checking without the cache.

## Consequences

- Diagnostics after an edit check the edited module and replay its
  dependencies. Expected: the CLI's 1.13 to 0.42 s ratio; the after rows
  (lspbench, Mac and host 1) go in the item B results file.
- A dependency's warning diagnostics come from its entry, as `xo check`
  shows them.
- The skip_unsafe fallback (feaeba094 fixed the dependency case) costs a
  second check when it fires; the lsp logs it like `xo check`.

## Alternatives

- Option 1 (lsp asks serve): rejected above; serve stays the CLI's.
- A resident checked program shared by lsp and the CLI (decision 0283
  stage 2): larger; item C measures whether its ceiling pays for it.
