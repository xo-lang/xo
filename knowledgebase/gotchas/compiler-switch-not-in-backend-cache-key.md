# A temporary compiler switch is not in the backend cache's key

Status: Observed (2026-10-17, xo-4)

Symptom: an experiment that changes arm64 code generation through an
environment variable read in selfhost/xo main.xo (a temporary field of
ssa.Module) builds programs whose functions mix the modes: two modes
gave byte for byte the same text, and a third built a TLS client that
failed at run time (its call sites assumed registers a helper of the
other mode clobbered).

Cause: the per-function backend cache (selfhost/arm64 fcache.xo,
decision 0220's addendum) keys a function's canonical text by its SSA,
what it names, the target, and the compiler's identity; the module
fields it knows (regs, debug) are in the key, a new temporary one is
not. The helpers are generated outside the cache, from the current
mode.

Fix: build every experiment's programs with XO_BACKEND_CACHE=off (or
add the field to fcache.xo's key line when it is to stay).
