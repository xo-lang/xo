# A new WASI import in the wasm runtime needs its component type entry

Status: Observed (2026-11-03, decision 0447 R2c)

## Symptom

Every wasip2/wasm command that uses the network fails to load, before
main runs:

    Error: failed to parse WebAssembly module
        instance 5 has no export named `[method]tcp-socket.remote-address`

The build succeeds and the core module validates.

## Cause

A wasip2 command is a component whose imports are typed instance by
instance in compiler/wasm/component.xo (ity_tcp, ity_sock_create, and
so on). The core runtime's `(import "wasi:sockets/tcp@0.2.0" ...)` lines
in rt9.xo (or rt5/rt7 for other interfaces) are matched by name against
those instance types when the component is built. An import added to the
runtime but not to its instance type makes an instance with no such
export, which wasmtime rejects when it parses the component.

## Fix

Add each new WASI function to both places in the same change: the core
import in the rt*.xo file and an `i.fun("[method]...", params, result)`
line in that interface's ity_* function in component.xo, with the WIT
signature. Run one network command golden (TestWasmCommandGoldens with
-run on http_server) after, since a wasip1 build never builds the
component.
