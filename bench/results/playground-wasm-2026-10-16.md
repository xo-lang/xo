# The playground on Xo's own wasm backend (decision 0242), 2026-10-16

xo-4b, the development Mac (M4 Pro), headless Chrome driven over the
DevTools protocol by the website's scripts/test-playground.mjs and a
scratch copy of its harness that times one sample program. The old
playground is /play/ as it was until 2026-10-16 (Go's compiler and linker
in the tab, frozen at go-compiler-final); the new one replaces it at
/play/, built by the website's scripts/build-playground.py from this
branch (manifest "xo": "d915f102").

## First-load download (brotli)

| Playground | Files | Compressed |
|---|---|---|
| old | xoplay 5.0, compile 6.2, link 2.1, pkg 10.1, gobind 0.04 MB | 23.5 MB (174.0 MB raw) |
| new | xoc (the compiler) 1,458,595 bytes, std 672,614 bytes | 2.13 MB (32.0 MB raw: 28,833,639 and 3,165,778 bytes) |

## Time to run a program in the tab

The sample: `fn main` sorting `[3, 1, 2]` and printing two values. Times
are from the click on run to the output, after the page loaded and
checked once.

| Playground | First run | Later runs |
|---|---|---|
| old | 3.79 s | 2.68 s, 2.79 s |
| new | 0.66 s | 0.45 s, 0.45 s |

test-playground.mjs over the tour's examples on the new playground:
0.5 to 0.8 s per run (11 examples).

## What got it there

Node worker stack each build needed (a Chrome dedicated worker allows
about 0.6 MB of a Node worker's, gotchas/wasm-browser-stack-locals.md):

| Step | Module | Stack needed | Build in Node |
|---|---|---|---|
| selfhost/wasm built for wasm, before | 50.7 MB | check 16 MB (4 MB failed) | 8.5 s |
| tzhost's 138-term chain as one literal, infer_left, lazy runtime bodies (decision 0247) | 51.7 MB | check 0.8 MB, build 1 to 1.5 MB | not measured |
| the RC pass sequential on wasm: nothing waits, so no function needs suspension frames, and every function gets structured control flow (514 had used the dispatch loop) | 28.7 MB | check 0.4 MB, build of the sample 0.6 MB, of a file system program 1 MB | 1.53 s |
| read_var walks single-predecessor chains in a loop (the same code on all 180 goldens); a program importing std/http is refused before the HTTP runtime is checked | 28.8 MB | the sample and the file system program 0.6 MB | |
