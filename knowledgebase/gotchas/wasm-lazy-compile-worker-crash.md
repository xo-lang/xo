# Node's worker crashes compiling a large wasm module lazily

Status: Observed (2026-10-12), Node 22.22.1 on darwin/arm64,
selfhost/wasm stage 2 (decision 0179).

**Symptom.** The self hosted checker built by the wasm backend (a
40 MB module) died with SIGSEGV (status 139, nothing on stderr) about
2 s into its run under `selfhost/wasm/run.mjs`, whatever the worker's
stack size (8 MB to 1 GB) and with `--liftoff-only`,
`--no-wasm-tier-up`, or `--wasm-lazy-validation`. The same module run
on Node's main thread (a script without the worker, `--stack-size=7500`)
printed the right output, and so did the worker with
`--no-wasm-lazy-compilation`.

**Cause.** Not established; V8 compiles a wasm function lazily, at its
first call, and that compile, on the worker's thread deep in a wasm
stack of large frames (functions of 10,000 locals), crashed. Small
modules (the goldens) never showed it.

**Fix.** run.mjs sets `--no-wasm-lazy-compilation`: every function is
compiled at instantiation (the checker: about 8 s and 5 GB on the
loaded machine). A second crash then showed at the end of long runs,
the incremental marking check of gotchas/wasm-busy-loop-crashes-node.md;
run.mjs sets `--no-incremental-marking` too. Retest both when Node is
upgraded, and in the browsers the playground targets.
