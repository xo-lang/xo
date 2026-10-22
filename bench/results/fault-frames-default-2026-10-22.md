# Fault frames by default: their cost (2026-10-22, Xo Lang 2)

For the owner's decision to print a fault's frames by default
(XO_TRACEBACK on unless 0). A native program (arm64, darwin, Apple
silicon) catches 200,000 faults with `task.isolate`, each an index fault
22 calls deep; `xo bench --ab`, 10 rounds, A without XO_TRACEBACK, B with
XO_TRACEBACK=1, built once (the variable is read at run time, so the
binary is the same: 2.0 MB):

| metric | A | B | B / A | 95% interval |
|---|---|---|---|---|
| wall ms | 134.9 | 245.1 | 1.816 | 1.774 to 1.853 |
| CPU ms | 120.0 | 230.0 | 1.917 | 1.769 to 1.917 |
| max RSS KB | 2,808 | 202,808 | 72.2 | (the leak, below) |

So about 0.55 us more a fault (0.67 us to 1.23 us), paid at each raise:
the trace is taken before the fault is known to be caught. Only programs
that fault in a loop see it (an HTTP handler's 500 is one fault a
request).

The RSS column found a leak: each trace block was released at the wrong
address. Fixed in 1f9ebbd77 (cmd/xo TestTracebackIsolatedNoLeak: 100,000
faults under 32 MB, 100 MB before).
