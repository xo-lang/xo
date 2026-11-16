# 0663: Embedding a package built for wasm (a std/ui App's client)

Status: Accepted (owner, 2026-11-15). The implementation also gives the wasm build a whole-output cache hit, so a warm server build stays a key lookup (Consequences).

## Context

Decision 0659 stage 3 (343b92236) lets the browser predict: an App's
client program (`fn main(os: Os) ! { ui.client_main(os.stdio, app())? }`)
is built with `--targets=wasip1/wasm`, and the server passes its bytes to
`ui.host(app, client: bytes)`, which serves them at `_xo/app.wasm`.
examples/ui-todo does this in two steps:

    xo build --targets=wasip1/wasm -o client.wasm ./client
    xo run ./server -- :8080 client.wasm

So a deployed app is two files, and the server and client can drift
apart: a client built from yesterday's `update` predicts states today's
server refuses. Prediction only works when both run the same `update`
(decision 0662 made it deterministic; nothing makes it the same build).

`embed("path")` (decision 0304) can't close the gap. Its path must
name a file that exists when the declaring module loads, inside the
module and outside nested modules; a build output is neither checked in
nor present on a clean checkout, so `xo check` would fail before anyone
ran the first step. Go has the same gap (`go:generate` plus a checked-in
or Makefile-built artifact).

What the checker does today (compiler/check/embed.xo): `expand_embeds`
replaces each `embed(...)` constant with a literal when its module
loads, and the embedded files' SHA-256s join that module's cache keys
(`embed_digest`).

## Decision

A second form of the built-in: `embed.build(package, target)` as a
constant's initializer, of type `Bytes`.

    const CLIENT: Bytes = embed.build("example.com/uitodo/client", "wasip1/wasm")

1. **The package** is a main package of the same module, named by its
   import path (no relative path, so the 0304 rule against `..` stays).
   **The target** is a string literal from the supported target list;
   this stage allows only `wasip1/wasm`, the one std/ui needs.
2. **The driver builds it.** The checker does not compile. When
   `expand_embeds` meets `embed.build`, it calls a hook the driver
   supplies (`build_embed(pkg, target) -> Bytes ! Str`). `xo build`,
   `xo run`, `xo test` and `xo check` pass a hook that runs a nested
   build of that package (its own cache, keyed as any build) and returns
   the output's bytes. The LSP and `xo check --fast` may pass a hook
   that returns a placeholder of the right type, so editing never waits
   on a wasm build.
3. **Keys:** the output's SHA-256 joins the declaring module's keys
   through `embed_digest`, as a file's does, so a change to the client's
   `update` rebuilds the server.
4. **No nesting:** a package built this way may not itself contain an
   `embed.build`, and a package may not build itself or a package that
   imports the declaring one (a new XO code). One level covers the use.
5. **Sizes** count against 0304's limits (64 MiB one, 256 MiB total).

examples/ui-todo's server then reads:

    const CLIENT: Bytes = embed.build("example.com/uitodo/client", "wasip1/wasm")
    ui.host(todo.app(), title: "Todos", css: todo.CSS, client: CLIENT).mount(r)

and `xo build ./server` (any target) gives one binary whose client is
always built from the same source.

## Consequences

- One build, one binary, client and server in step. Cross-compiling the
  server (linux/arm64, windows/amd64) still builds the client for wasm.
- A server build's cost includes the client's wasm build. Measured on
  the Mac Studio (2026-11-15, main at 6d42a57e3, examples/ui-todo, 3
  rounds, a fresh `XO_BACKEND_CACHE_DIR` each round, the front end caches
  shared): the client builds in 0.58 to 0.68 s cold and 0.23 to 0.24 s
  warm (0.18 to 0.19 s after an edit to the App's file), the server in
  1.31 to 1.38 s cold and 0.04 to 0.05 s warm; the module is 203,388
  bytes. So `embed.build` adds about half a cold server build, and a warm
  server build goes from 0.04 s to about 0.27 s unless the nested build's
  output is cached by its key: the wasm build has no whole-output cache
  hit today (its warm time is 5 times the native one), which the
  implementation adds (the client's key decides, as a file's SHA does).
- The checker gains a hook, not a build: compiler/check stays free of
  the driver. New XO codes for a bad package, an unsupported target and
  a nested `embed.build` (claimed with code-claim.sh when implemented).
- `TestCodegenInputsClassified` and the watch loops must learn the new
  input (the nested build's sources); `embedded_paths` lists them.

## As built (2026-11-15)

- The checker has no hook (a function field would carry the nested
  build's effects into every checker entry point). Instead
  `expand_embeds` replaces an `embed.build` constant with the output
  from `Checker.embed_builds` ("package\ntarget" to bytes) when the
  driver gave it, else records the build in `embed_build_want` and puts
  an empty Bytes there. The driver's `check_program_ov`, after a check
  without errors that wants builds, runs each (`xo build
  --targets=wasip1/wasm -o <user cache>/xo/embedbuild/HASH.wasm DIR` with
  `XO_EMBED_BUILD_NESTED=1`, where `embed.build` is XO1519), then checks
  again with the outputs. Editors and the LSP never fill the map, so the
  constant is empty there, as point 2 allowed.
- The run cache (decision 0360) keys a build on the files the process
  reads; the nested build reads in its own, so a warm server build kept
  the old client after an edit to the client alone. The driver now lists
  the nested build's inputs (`--list-inputs`) and reads them through its
  own recorded file system (TestEmbedBuild fails without it).
- The wasm path stored no run cache entry, so every warm wasm build ran
  the backend: 0.20 s against 0.01 s now (TestWholeBuildWasm; main's xo
  at 95b2f061 missed on both builds of an unchanged program).
- Measured (Mac Studio, examples/ui-todo, 3 rounds): a server build
  after an edit to the App's file is 0.83 to 1.05 s with `embed.build`
  and 0.42 to 0.44 s without; a warm build 0.02 s either way. The cost
  left is the second check, the nested build and its `--list-inputs`
  (a third process).
- Next: have the nested build write its input list itself (one process
  less), and skip the first check's wasted half when the previous run's
  builds are still current (a record of the last run's wants).

## Alternatives

- **Status quo: two builds, the server reads a file.** Works today; two
  artifacts that can drift, and every app needs a script. Kept as the
  fallback (`host(client:)` takes any bytes).
- **`embed("path")` on a checked-in client.wasm.** A binary in the repo
  regenerated by hand; drifts the same way and breaks review.
- **An `xo ui build` subcommand** that builds the client, then the server
  with the bytes stamped in (`--stamp`, decision 0165, only takes short
  strings). Ties a std module to the CLI, and `xo run` and `xo test` would
  still not do it.
- **The checker compiling the package itself.** Puts the whole pipeline
  under compiler/check and makes check time depend on a wasm build;
  the hook keeps that in the driver.
- **A JS backend instead of wasm** for the client. The measured wasm
  path is fast enough (p95 2.6 ms with a 6x slower CPU, 55 KB brotli) and
  byte-identical to the native build; a second backend would be a much
  larger decision.
