# Playground (xolang.dev/play)

Decision 0144. Everything runs in the visitor's browser; there is no server.
Last verified: 2026-10-10 (Go 1.26.1, Chrome headless).

## Pieces

| Piece | Where | What |
|---|---|---|
| `cmd/xoplay` | this repo | `check DIR`, `fmt FILE`, `gen [--test] DIR OUT`, one JSON object each; built for wasip1 with `-tags xo_playground` |
| `build.Generate` | `internal/build/build.go` | check plus code generation of `xo build`, no build directory, no Go toolchain |
| `std/embed_impl.go` / `embed_playground.go` | this repo | the rt, native, and llrt embeds; empty under `xo_playground` |
| `scripts/build-playground.py` | site repo | builds xoplay.wasm, compile.wasm, link.wasm, the rt plus Go std archive bundle (tag `xo_nohttp`, with `use go` adapters), and the bindings bundle into `static/play/assets/` (gzipped, content hashed) and `static/play/manifest.json` |
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
every tour example and mistake, the tour's tests, a failing test, the
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

Sizes (gzipped): xoplay 7 MB, compile 9 MB, link 3 MB, archives 14 MB,
bindings under 1 MB. Bound Go packages: see `GOBIND` in the script.
