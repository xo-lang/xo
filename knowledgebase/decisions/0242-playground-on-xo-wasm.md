# 0242: The browser playground on Xo's own wasm backend

Status: Accepted (owner, 2026-10-16; xo-4b). The new playground replaces
the old one at /play/ (website, 2026-10-16).

## Context

The playground (website /play/, static/play) was frozen at
go-compiler-final on 2026-10-13: it ships Xo's Go-era checker plus Go's
compiler and linker to the tab, 174.0 MB (23.5 MB brotli), and runs a
program in about 2.7 s after the first load. The owner asked for one on
selfhost/wasm: the Xo compiler itself, built for wasm by its own wasm
backend, checking, compiling and running programs in the tab with no Go
toolchain.

## Decision

- **The compiler in the tab.** selfhost/wasm gains a playground mode,
  `--play check DIR | fmt FILE | build DIR OUT` (selfhost/wasm/play.xo),
  which prints one JSON object (diagnostics as the editor wants them,
  formatted source, or the built module's size). `check` is the plain
  check of `xo check`; `build` checks in native mode with lazy runtime
  bodies (decision 0247) and writes a WASI module. The website's
  build-playground-next.py builds selfhost/wasm for wasm with the
  checkout's xo and bundles std's sources (the .xo files and xo.mod,
  not tests); both are brotli files named by hash, listed in
  manifest.json.
- **The page.** /play/ keeps the old page's editor, examples, sharing,
  runner worker and safety limits; the toolchain worker is new
  (assets/js/play/toolchain.js): it runs the compiler under
  browser_wasi_shim with an in-memory /w (std, the source, an output
  directory) and hands the module to the runner. The website's
  scripts/build-playground.py builds it.
- **Only the new playground is on the site (owner, 2026-10-16).** The old
  toolchain's assets (xoplay, compile, link, pkg, gobind: 23.5 MB) and its
  manifest are removed; the website's git history keeps them, with the
  old build-playground.py, which built them from a go-compiler-final
  checkout (the Go compiler and linker built for wasm, the precompiled rt
  and Go std archives, the `use go` bindings).
- **Dropped, by the owner (2026-10-16): the generated Go tab and `use
  go`.** A program with `use go` gets a clear message that `use go` is
  not supported in the playground, instead of a binding error. Either can
  come back later:
  - `use go` through an on-demand "Go backend" mode that downloads the
    old toolchain (rebuilt from go-compiler-final as above) only when a
    program uses `use go`;
  - a code tab that shows the generated WAT instead of Go.
- **Tests stay a goal**: the playground refuses `test` for now, with a
  message.
- **A program that imports std/http** is refused right after its check
  (play_build): the wasm backend cannot run it (WASI preview1 has no
  sockets), and checking the HTTP runtime's bodies on the way there
  overflowed the worker's stack.
- **What made it fit a browser** (bench/results/playground-wasm-2026-10-16.md):
  - std's 138-term string chain became one literal, and the checker walks
    left-nested operator chains in a loop (it recursed once per term);
  - lazy runtime bodies (decision 0247) check only what a build reaches;
  - the SSA builder's RC pass runs sequentially in a wasm build
    (selfhost/ssa rc.xo, decided by the build as decision 0246 allows):
    with no task or channel left, nothing in the module waits, so no
    function needs the backend's suspension frames and every function
    gets structured control flow (514 used the dispatch loop). The
    module went from 51.7 to 28.7 MB, and a build from needing 1 to
    1.5 MB of stack to 0.6 MB, within a Chrome worker's;
  - the SSA builder's read_var walks a chain of single-predecessor
    blocks in a loop (it recursed once per block; the same code on all
    180 goldens), so a file system program builds in 0.6 MB too.
  - `--annotate` notes why a function uses the dispatch loop.

## Consequences

- First load 2.13 MB brotli (the compiler 1.46, std 0.67) instead of
  23.5 MB; a run 0.45 s after the first (0.66 s the first) instead of
  about 2.7 s, in headless Chrome on the development Mac.
- The stack is still the limit for nesting: an `else if` chain and a
  left-nested chain of operators are walked in loops (the parser, the
  checker, the flow passes and the SSA builder; a 200-branch chain builds
  in 0.6 MB, golden else_if_chain), but deeply nested blocks or calls
  still recurse once per level in each of those stages. The next step
  for them is smaller frames in the wasm backend (the Checker passed as
  45 leaves).
- The playground runs what the wasm backend runs: no network (WASI
  preview1 has no sockets), no working directory (`--wd` is given;
  a program's cwd_path reports Io), no `use go`, no tests yet. Crypto
  and TLS code works (decision 0231's WASI entropy addendum).
- A change that makes some function of selfhost/ssa, selfhost/check or
  selfhost/wasm wait (a task, a channel, a mutex) brings back suspension
  frames for it and its callers, and the stack with them; the RC pass's
  parallel path is the model for keeping such code out of a wasm build.
- The website's test-playground.mjs covers the new playground: the tour's
  examples run with their output (a server and `use go` are refused),
  the mistakes report their codes, the safety limits hold, tests are
  refused for now, and it prints the run times.

## Alternatives

- **Keep the old playground beside the new one** (as /play/v1/, an
  option the owner first chose and then dropped): 23.5 MB of assets to
  host for `use go` alone.
- **Keep the Go toolchain.** 23.5 MB and 2.7 s a run, frozen at
  go-compiler-final, and a second compiler to keep in step.
- **A server compiling for the tab.** The playground's promise is that
  code never leaves the machine.
- **A bigger stack.** A page cannot set a worker's stack size or V8's
  flags; frames had to shrink.
- **Splitting the checker's large functions by hand** (resolve_type_obj
  was 113,169 lines of wat). Removing the suspension frames shrank every
  function at once.
