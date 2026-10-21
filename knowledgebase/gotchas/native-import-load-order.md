# An implementation module's `use native/M` became runtime calls when M sorted later

Status: Observed (2026-10-21, Xo Lang 3; fixed the same day)

Symptom: a call from one std implementation module into another's
std-internal `pub` function (decision 0319), for example
`nmp.new_stream(...)` from std/native/http.xo into
std/native/multipart.xo, failed to build on the native backends with
`arm64 backend: the runtime call \`rt.new_stream\` returns a
\`multipart.Stream\``. The same pattern worked for std/native/websocket.xo
calling `nhttp.handoff_response`.

Cause: the checker loads implementation modules in sorted order of the
std modules (load_native). An import `native/M` seen while loading an
implementation module whose M was not loaded yet went to
load_native_rt, which loads intrinsics modules (native/rt, sys_c):
bodies are declarations there, and the SSA builder lowers every call
into a std `native/` module as `rt.<name>`. "websocket" sorts after
"http", so that import found the real module; "multipart" sorts after
"http" too, but http imports it, so http loaded first.

Fix: load.xo's load_imports loads the implementation module itself
(load_impl, with std/M) for `use native/M` from an implementation
module when std/M exists; load_native skips modules already loaded.
