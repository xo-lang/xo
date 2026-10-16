# arm64: a fault stub needs its own line row

Status: Observed (2026-10-16, fault tracebacks, decision 0239).

Symptom: the innermost frame of a traceback showed the function's last
statement's line (main.xo:7) instead of the faulting one (main.xo:5).

Cause: a checked instruction branches to a fault stub placed after the
function's body; the fault call returns into the stub, whose address
fell in the last row of the body.

Fix: emit.xo gives each stub a row label with the faulting
instruction's line; the stubs' rows go after the body's (a line
program's addresses must increase within a sequence).
