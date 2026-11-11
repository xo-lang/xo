# The playground's compiler overflowed every browser's stack, and every test passed

Status: Observed (2026-11-11, Xo Lang 2), fixed in 42160d539.

**Symptom.** Building the website's /play/ from main (for the repl tab,
decision 0583) failed every check and run in Chrome with "the compiler
stopped unexpectedly: Maximum call stack size exceeded", the tour's
first example included. The release 0.48.0 has it: its compiler, swapped
into the live page, fails the same way. Every xo test passed, and the
xo CLI and native builds were unaffected.

**Cause.** `Checker` grew to about 1,143 leaves, and V8's baseline
compiler sizes a wasm frame by its locals and its deepest operand stack
(gotchas/wasm-browser-stack-locals.md). Results of up to 1,000 leaves
came back as values and wide values were loaded whole onto the operand
stack, so 69-local functions had 9 KB frames, and a build needed more
than 700 KB of stack (0.35.0's compiler, the live page's, 350 KB).
Nothing in xo ran the playground's compiler as wasm in a browser-sized
stack: TestSelfhostWasm runs compiler/wasm built natively, and run.mjs
gives programs a 1 GiB worker stack. Only the website's
scripts/test-playground.mjs (headless Chrome) runs the built compiler,
and only when the playground is rebuilt.

**How it would have shipped.** The release page step syncs the spec and
std reference from the release tree but not the playground. Running
scripts/build-playground.py from v0.48.0 would have replaced the working
0.35.0 bundle with one that fails every build, and the site's deploy
gate (test-playground before every Pages deploy) is the only check
between them.

**Fix.** 42160d539: MAX_RESULTS 64 (a wider result goes through the
caller's block) and `mem_set` (a wide value read from memory goes a leaf
at a time into its locals); a build runs in a 200 KB Node stack.
`TestWasmPlaygroundStack` (internal/selfhost, merge tier) builds
compiler/wasm with `--backend=wasm` and runs `--play build` (writing
std's record, then with it) in a 0.6 MB Node worker, about a Chrome
worker's stack: 0.48.0's compiler traps there and 42160d539's passes.
Before rebuilding the playground for a release, run the website's
test-playground against the new bundle.
