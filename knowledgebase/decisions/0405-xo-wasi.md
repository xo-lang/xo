# 0405: xo built for wasip1/wasm: check, fmt, vet and build for every native target

Status: Proposed (owner request via the lead, 2026-10-29: ship a full `xo` built for wasip1/wasm with each release, target 0.35; Xo Lang 3). Notes with the measurements: ~/xo-lead/notes/lang3/wasi-xo.md.

## Context

`xo build --targets=wasip1/wasm compiler/xo` stops at the wasm backend's first refusal ("does not support processes (`proc.run`)"). The complete list, found by making the generator collect every reason: 29 findings in four groups: processes (proc.run, proc.exec, proc.spawn), network (net_listen, net_dial, net_status, net_msg, listener_accept, listener_close, listener_addr, conn_read, conn_write, conn_close), rt.task_stopping (watch, exec_binary, http.Server.serve), and native scheduler and allocator code (stack_switch, sched_hook, fiber_start_addr, heap_big, heap_grow, heap_big_release) reached only through those. With the wasm backend's `--lax` mode each becomes a runtime fault and the build links (184 MB; native xo is 113 MB).

The module then fails to load: a function may have at most 1000 parameters and 1000 results (the WebAssembly limit, enforced by V8, wasmtime and wasmer), and ssa.Builder flattens to 1039 wasm values. Lang 1's wasm work (wide results through a caller block, wide drops field by field, check_arity) removes it.

## Decision

1. **Scope.** The WASI build of xo runs `check`, `fmt`, `vet`, `explain`, `asm` and `build` for every native target (arm64, amd64, with Xo's own assemblers and linkers: no clang, ld or dsymutil). It targets WASI preview1 only (files, clocks, environment, arguments, random; no sockets, processes, threads or file locks). Tasks stay cooperative on the wasm runtime.
2. **Unavailable commands.** `run`, `test`, `bench`, `watch`, `serve`, `lsp`, `mcp`, `replay`, `dev goldens` and `bisect`, `use go` and `xo bind`, module fetch over git, and `--backend=llvm` or `go` (clang, the Go toolchain) print `xo: COMMAND is not available in the WASI build` and exit 2, with the new diagnostic `XO1513` (error, build; spec section 10, `xo explain XO1513`). A `build` that reaches a process (a system linker with `--ld=system`, dsymutil) reports the same code at that point, naming the program it needed.
3. **Backend policy.** The wasm backend lowers proc.*, net_* and rt.task_stopping in compiler/xo as runtime faults with the same stable text (a generator option for this build, not the generic `--lax`, which stays a debugging aid); rt.task_stopping is false on the cooperative runtime. The driver's checks come first, so a user sees XO1513 and not a fault.
4. **Degradations.** The build cache's file lock becomes no lock when the runtime has none (one process per module); the working directory comes from `PWD` or `--wd` (preview1 has no getcwd); the atomic rename is path_rename. Paths are reached through preopened directories (`.` and `/`).
5. **Release.** Each release ships `xo.wasm` with std and a run script for Node (`compiler/wasm/run.mjs`: 1 GiB stack worker, eager compilation) and the command line for wasmtime; its smoke (hello built for linux/arm64 and darwin/arm64, the darwin result run natively) is part of the release checks.

## Consequences

- The wasm backend's size: a 184 MB module (about 1.6 times the native binary), start time dominated by compilation (seconds under Node, once under wasmtime with `wasmtime compile`).
- Every new use of a process or the network in compiler/ must be reachable only through a command that is gated, or the build's generator refuses it.
- `XO1513` is new; the spec lists it and `xo explain` describes it.

## Alternatives

- A WASI preview2 / threads build: wasmtime-only, sockets and threads not in the Node runtime the playground uses; not needed for the scope above.
- Making unavailable features no-ops: a build that silently skips a linker or test run is wrong; refusing with a stable code is honest.
- Splitting compiler/xo into a WASI-only front: duplicates the driver; the generator policy and the gate keep one source.
