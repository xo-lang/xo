# An llrt state record entry.xo makes is 0 on wasm

Status: Observed (2026-11-02, Xo Lang 4, decision 0447 stage 3)

Symptom: on wasm (wasip1 under Node and wasip2 under wasmtime alike) the
first read of standard input worked and every later one failed with
`canceled` (std's stdin reader returned status 3), while native runs of
the same program were fine.

Cause: std/native/llrt/stop.xo's state record is made by `stop.init()`,
which std/native/llrt/entry.xo calls at a native program's start. The
wasm backend has its own start and never runs entry.xo, so the static
word that holds the record stays 0, and `stop.requested()` loaded
`R_REQUESTED` from address 16 of linear memory. That address was not
zero after the first read, so stdin.xo's `stopped_now()` reported a stop
request and the read returned canceled. No fault: a load from a low
address is valid wasm.

Fix: stop.xo's `requested`, `canceled` and `root` return false (0) when
the record is 0. Its writers (`signal`, `set_grace`, `stopping`,
`set_root`, `cancel`) stay unguarded: they are reached from native
signals, entry.xo and cabi.xo's exports, which wasm does not run; a wasm
path to one would write low memory. Any llrt module whose record entry.xo makes (`init()`
called from entry.xo) and that std can reach on wasm needs the same
guard, or a wasm start that makes the record: grep entry.xo's `init()`
calls and the module's `fn rec()` before relying on it in code wasm runs.
Test a std path that wasm runs more than once in a program (a second read
found this one; the first passed).
