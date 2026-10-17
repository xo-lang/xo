# Playground (xolang.dev/play)

Decision 0144. Everything runs in the visitor's browser; there is no server.
Last verified: 2026-10-10 (Go 1.26.1, Chrome headless).

**Since decision 0215 (2026-10-13) `cmd/xoplay` is not in this repository:**
it was the Go compiler (internal/check, format, gen, build) built for
wasip1, and the Go compiler is deleted. The site's
`scripts/build-playground.py` must build xoplay from the
`go-compiler-final` tag (87873aa5) until a self hosted replacement exists
(next step: an Xo program over compiler/check, compiler/cli/core/format,
and compiler/gen with the same three commands, built by `xo build
--backend=go --targets=wasip1/wasm`). The playground then compiles
programs with the language of that tag. The table below describes the
pieces as of that tag.

The tag itself is not wasip1 clean: internal/build/bincache.go uses
`syscall.ForkLock` (added in 15b3a46d), which wasip1 does not define, so
`cmd/xoplay` at 87873aa5 does not build for wasip1 as is. The site's
build-playground.py extracts the tag with `git archive` and drops those
three lock calls in the extracted copy only (xoplay never uses the binary
cache); deployed 2026-10-13 (site 454a6ca), test-playground.mjs passing.
The Xo xoplay replacement removes the workaround. Last verified:
2026-10-13.

## Pieces

| Piece | Where | What |
|---|---|---|
| `cmd/xoplay` | this repo | `check DIR`, `fmt FILE`, `gen [--test] DIR OUT`, one JSON object each; built for wasip1 with `-tags xo_playground` |
| `build.Generate` | `internal/build/build.go` | check plus code generation of `xo build`, no build directory, no Go toolchain |
| `std/embed_impl.go` / `embed_playground.go` | this repo | the rt, native, and llrt embeds; empty under `xo_playground` |
| `scripts/build-playground.py` | site repo | builds xoplay.wasm, compile.wasm, link.wasm, the rt plus Go std archive bundle (tag `xo_nohttp`, with `use go` adapters), and the bindings bundle into `static/play/assets/` (brotli, content hashed) and `static/play/manifest.json` |
| `assets/js/play/toolchain.js` | site repo | worker: downloads (Cache Storage), runs the tools over an in-memory WASI file system |
| `assets/js/play/runner.js` | site repo | one worker per run: caps memory, allows only WASI imports |
| `assets/js/play/play.js`, `layouts/play.html` | site repo | page, CSP, editor, share links |
| `static/play/vendor/browser_wasi_shim-0.4.2/` | site repo | vendored WASI shim (MIT/Apache-2.0) |

## Commands

```sh
cd ~/godev/claude-space/xolang/website
scripts/build-playground.py            # after compiler, std/rt, or Go changes (about 2 min)
.tools/bin/hugo                        # then test at /play/ (needs http://localhost or https)
```

`go test ./cmd/xoplay` covers the commands natively. The browser pipeline
is tested in the site repo by `node scripts/test-playground.mjs` (headless
Chrome over the DevTools protocol, no npm dependencies; about 1 minute):
every tour example and mistake, the tour's tests, a failing test, seed
and explore (a test that fails only under seed 2), underlines and their
tooltips (hover and caret), the generated Go tab and its links, the
phone layout, the
download notice, `#example=` and share links, highlighting, an infinite loop (stopped at 10 s), an output flood (cut at 1 MB,
HTML shown as text), unbounded allocation (Go "out of memory", exit 2),
`os.fs.read_text("/etc/passwd")` (fails: no files), `use go "os/exec"`
(XO1101 naming the supported packages), a 10 MB deflate bomb in a link
(refused at 64 KB), sleeps, and no console errors. Last verified:
2026-10-10, all passed.

## Tests (the test button)

`xoplay gen --test` generates the test binary (`gen.Test`). `xo test`
builds test binaries without `xo_nohttp`; the playground adds the tag when
the program does not reach std/http (`gen.ReachesStd`), so the same rt
archive serves both. The runner starts it with `play.test -v`.

## Editor

A textarea over a highlighted `<pre>` (`highlight.js`, a tokenizer for the
lexical grammar: the 28 keywords of core 1.4, strings with `${}`
interpolation, raw and byte strings, runes, numbers and durations, `???`).
It builds DOM nodes with `textContent`. Update its keyword list when
core 1.4 changes.

## Build steps in the browser

1. `xoplay gen /w/src /w/out` lists packages in dependency order, the Go tags
   (must equal the manifest's), and the rt files the program needs (must be
   adapters the archive already has).
2. `compile -p PATH -importcfg ... -pack FILES` per package, `-c=1`.
3. `link -importcfg ... -buildmode=exe -checklinkname=0 -s -w`.
4. The runner rewrites the memory section to a 1 GiB maximum and runs it.

Sizes (brotli `-q 11 -w 24`, `.brotli` files, decoded in the worker by
`static/play/vendor/brotli-decode-1.1.0`, about 250 ms for the archives):
xoplay 4.9 MB, compile 6.2 MB, link 2.1 MB, archives 9.3 MB, bindings
under 0.1 MB; 22.5 MB in all (33 MB with gzip -9, 19 MB with xz -9).
Last verified: 2026-10-10.

## CI

`.github/workflows/pages.yml` in the site repo runs
`node scripts/test-playground.mjs` with `CHROME=/usr/bin/google-chrome` and
`CHROME_FLAGS=--no-sandbox` (Ubuntu runners' AppArmor blocks Chrome's
sandbox) before deploying; a failure stops the deploy.

## Editor extras

Diagnostics of the last check are underlined (wavy, `xmark-error` and
`xmark-warning` in the overlay) and cleared on the next edit. Their
message shows in a tooltip when the pointer rests on one (the overlay
ignores the pointer, so the page hit tests the marked spans' boxes; each
records its source offset in `data-at`) or when the caret is inside one.
The generated Go tab is highlighted (`goTokens` in `highlight.js`) and
hides `//line` directives unless its checkbox is on; the page still reads
them, so each Go line they map to main.xo is a link to its Xo line. The
test script also checks the page at 390 px (`viewport`): nothing wider
than the screen, the buttons on one row, the test options floating over
the page. `SHOTS=dir` saves screenshots of it. The gear next
to test sets `--seed` and `--explore` (capped at 1000) for the test binary. Bound Go packages: see `GOBIND` in the script.
