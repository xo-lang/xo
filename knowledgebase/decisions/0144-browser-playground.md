# 0144: A browser playground that compiles and runs on the visitor's machine

Status: Accepted (2026-10-10). Requested by the owner ("do the playground
and ensure it's safe"); publishing it is the owner's call because it ships
compiled forms of the private compiler and runtime.

## Context
A playground is the fastest way to try Xo. A server that compiles and runs
visitors' code is the usual design (go.dev/play), but it is a standing
security liability (arbitrary code execution, abuse, cost) and xolang.dev is
static (GitHub Pages, decision 0139). The source is not public yet, so
whatever the site ships must not include it.

## Decision
- **Everything runs in the visitor's browser, in WebAssembly; there is no
  server.** The page sends nothing anywhere. Shared links carry the code in
  the URL fragment (`#code=`, deflate and base64url), which browsers never
  send to a server.
- **Pipeline**: `cmd/xoplay` (this repo, GOOS=wasip1) runs `check`, `fmt`,
  and `gen` (check, then `build.Generate`, the code generation step of
  `xo build` without the Go toolchain). The Go compiler and linker
  (`cmd/compile`, `cmd/link`, also built for wasip1) compile the generated
  packages and link them against precompiled archives of package rt and the
  Go standard library for wasip1/wasm. The result runs in a separate worker.
- **No runtime source ships.** `std/embed_impl.go` (tag `!xo_playground`)
  holds the rt, native, and llrt embeds; `xoplay` is built with
  `-tags xo_playground`, which leaves them empty. The rt archive carries
  compiled code and export data (which includes inlinable function bodies),
  not source.
- **Tests**: `xoplay gen --test` builds the test binary; one that does not
  reach std/http gets `xo_nohttp` too (`gen.ReachesStd`), so the same
  archive serves programs and tests.
- **One rt variant**: tag `xo_nohttp`. A program that imports std/http can be
  checked but not run (a tab cannot open a server).
- **`use go`**: a fixed list of Go standard packages is bound at build time
  (`xo bind`), the adapters compiled into the rt archive, and the bindings
  shipped read only with `XO_GOBIND_DIR`. Other packages get an XO1101 that
  names the supported list.
- **Sandbox of a run**: the program's only imports are WASI preview1
  (checked before instantiating), over an empty in-memory file system and no
  sockets; memory is capped at 1 GiB by rewriting the module's memory
  section; output at 1 MB; wall time at 10 s (the page terminates the
  worker; a stop button does the same). The toolchain worker is restarted
  after 120 s.
- **Page**: Content Security Policy `default-src 'none'`, scripts only from
  the site plus `wasm-unsafe-eval`, `connect-src 'self'`; all code and
  output are shown with `textContent`. A link's source is inflated to at
  most 64 KB and only checked on load; it runs only when the visitor
  presses run.
- **Third party code**: `@bjorn3/browser_wasi_shim` 0.4.2 (MIT/Apache-2.0)
  and Google's JS brotli decoder (`js/decode.js` of google/brotli v1.1.0,
  MIT), vendored after review (no network, storage, or eval use). Browsers'
  DecompressionStream has no brotli; xz was smaller still (19 MB) but the
  only browser decoder found ships its WebAssembly as an unreviewable blob.

## Consequences
- About 23 MB of toolchain (brotli; 33 MB with gzip) on the first run (the page asks first: 5 MB to check, 18 MB more to run), cached in Cache
  Storage under content hashed names. Checking alone loads 7 MB.
- A build and run of a tour program takes about 2 s in Chrome.
- The site repo carries the binaries; rebuild them with
  `scripts/build-playground.py` after compiler, std/rt, or Go changes. The
  compiler, linker, and archives must come from the same Go release.
- Shipping `xoplay.wasm` publishes the compiler in compiled form before the
  source is public: it can be decompiled. The owner decides when to publish.
- Programs see real time but no files, network, or environment.

## Alternatives
- A compile server (sandboxed containers): rejected for the risk, the
  running cost, and the static host.
- Check only, no run: much smaller, but the owner asked for a playground.
- Compiling rt in the browser for arbitrary `use go`: needs the rt source
  in the page.
- Building `cmd/xo` for wasip1 as is: it compiles, but embeds the runtime
  and native sources.
