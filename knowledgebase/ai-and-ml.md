# Xo for AI and ML: what it would take, and where Xo fits first

The owner's question (2026-11-05): what would it take for Xo to be the
choice for developing AI and ML instead of Python? This is exploration, not
a plan: nothing here is decided or scheduled. A step that becomes work gets
its own ADR.

## Why Python holds the field

Python does not win ML on speed. It wins on the ecosystem (PyTorch, NumPy,
JAX, Hugging Face), on cheap exploration (notebooks, a REPL), and because
the heavy arithmetic runs in C++ and CUDA underneath: Python is the glue.
Replacing it outright would take a GPU story plus a decade of libraries.

## What Xo would need, roughly in order

1. **An n-dimensional array type**: slicing, broadcasting, and views that do
   not copy. This meets value semantics (decision 0003: assignment copies),
   so arrays need copy-on-write or borrowed views to stay cheap. Under it:
   BLAS bindings (`use c`) and SIMD, which are among the speed primitives
   the owner parked on 2026-10-13.
2. **GPU access**: at least calling CUDA, Metal or ROCm through `use c`;
   ideally compiling Xo functions to GPU kernels, as Triton does for
   Python. The largest piece by far, years for any language.
3. **Automatic differentiation**, where Xo has a structural edge. JAX's
   `jit`, `grad` and `vmap` are only correct on pure functions, and in
   Python purity is a convention nobody checks. Xo's effect system already
   proves a function has no capability effects (spec 6.1: no `uses` clause
   means pure), so compiler-level autodiff and tracing could be guaranteed
   safe rather than hoped safe.
4. **Interoperate rather than compete**: load models trained elsewhere
   (safetensors, ONNX) and run them; possibly call Python libraries. No one
   rewrites the ecosystem.
5. **An interactive loop**: a REPL or notebook kernel with near-instant
   recompiles (see "A REPL" below).

## Where Xo could win first

Not research, where Python is entrenched, but the steps around it.

### Inference serving

The service that loads a trained model and answers requests. Today teams
write it in Python (slow, memory hungry, the GIL) or move to Go, Rust or
C++ for production. It is what Xo is built for: HTTP/1.1 and HTTP/2
servers in std, static binaries, structured concurrency for batching and
cancellation (a request that times out cancels its model work), and
predictable memory: native builds free at last use by reference counting
(decision 0046), with no GC pauses in a latency budget. Against Go, the
0.38.1 release's run rows all peaked lower than Go's (0.353 to 0.864 of
Go's, bench/results/release-vs-go-v0.38.1.md:46). Request arenas
(decision 0066, being brought to the native backends) fit the
per-request scratch of tokenization and pre-processing.

What it needs: a tensor runtime to call (llama.cpp, ONNX Runtime or
GGML through `use c`, then native kernels for the hot paths), safetensors
and tokenizer libraries in std or a module, and streaming responses for
token-by-token output (std/http streams response bodies today, the
http_stream_body_h2 golden among them).

### Backends that show what touches the network or disk

Every Xo function's signature says what it may do: `uses net.dial`,
`uses fs.read`, or nothing (pure). The capabilities themselves are values
passed down from `main`, so a library can only open a socket if it was
handed one. For AI systems this matters in three ways:

- **Agent tool sandboxing.** An agent's tools are functions; their
  signatures say which can reach the network, the disk, processes or the
  clock. A "summarize this file" tool that declares only `fs.read` cannot
  exfiltrate data over the network: the compiler rejects it. Today that
  guarantee needs containers or a review.
- **Supply chain.** `xo audit` reports, per dependency, the effects its
  API may reach and its escape hatches (`use c`, `use go`); `xo.lock`
  records each dependency's effects, and a new version that reaches new
  ones is flagged until `xo mod accept-effects` (decision 0156). A model
  helper library that suddenly starts calling home shows up at upgrade
  time, not in production.
- **Data pipelines.** `xo query effects module.fn` gives the call chain to
  every effect, so "which steps of this pipeline write to disk" is a query,
  not an audit. Tests pass fake network, disk, clock and environment
  values (the agents rate this the best part of Xo, survey 2026-11-05).

### Code that AI writes

ML code is increasingly written by models, and Xo's founding thesis is a
language for agents to write, check and debug. Python finds most mistakes
at runtime, sometimes hours into a training run. Xo moves them to compile
time: an unhandled error is a compile error, there is no nil, `match` is
exhaustive, data races are excluded by value semantics, and effects are
checked. Diagnostics are JSON with stable codes and machine fixes
(`xo check --json`, `xo fix`), so an agent repairs its own code in a loop
without a human reading errors. The measured form of this is the planted
bug benchmarks: Xo prevented or caught 23 to 29 of 30 planted bugs against
9 for plain Go (bench/results/planted-xo-2026-10-07.md). For ML, the
same checks would catch a shape mismatch only if array shapes were in the
type system, which is a design question for item 1 above.

## A REPL

The owner finds the REPL interesting, especially if it also speeds up
developing the language itself (2026-11-05). What it could be: `xo repl`
keeps a checked program in memory (as `xo serve` does for check and query),
compiles each entered line or definition against it, and runs it at once.
For language work it would let an agent try a construct, a std call or a
diagnostic in seconds instead of writing a program and building it. It
depends on a fast build of small changes, which the fast edit loop work
(Xo Lang 4, 2026-11-05: rebuild measurement, warm builder caches,
checker-only mode) is the start of. Open questions for an ADR: native
code loaded into a running process (a JIT, which wasm-and-jit.md argued
against for general use) or a small interpreter over Xo's SSA; how state
carries between entries under value semantics; and what effects a REPL
grants.
