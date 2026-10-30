# A function replaced by a shell in the module needs its body facts

Status: Observed (2026-10-30, xo-lang-6)

Symptom: after the emitter's parts took their functions' bodies out of the module (decision 0424),
byref_var and other goldens failed: callees passed by reference had no parameters, since
`Module.param_types(fi)` returned [] for every shell.

Cause: the module's readers of a function's signature and names (ssa/bodyfacts.xo param_types,
block_names, value_names, fault_facts, value_count) take `m.facts[fi]` when present and otherwise
read the body (the entry block's parameters, the values' names). A `Func.shell()` has no blocks,
so without facts the fallback silently returns nothing. The lazy functions never hit this because
rccache.xo lazy_shell records the facts decoded from the entry beside the shell.

Fix: record `ssa.body_facts(f, m.types)` into `m.facts[fi]` before replacing a function with its
shell (generate_cached does); any new place that hollows a function must do the same.
