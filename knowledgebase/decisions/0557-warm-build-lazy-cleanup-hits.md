# 0557: A warm build keeps the cleanup cache's hits undecoded

Status: Proposed (Xo Lang 4, 2026-11-07). Extends decision 0382 (the chained cleanup keys, the
"cln" cache) and decision 0283 stage B (lazy reference counting and inline hits, BodyFacts).
Step 1 (an incremental uniqueness analysis) is a prerequisite found by measurement, below.

## Context

A profile of 20 warm whole builds of http_server_request on the linux/arm64 host (3eec30839,
2026-11-07, 9.16 s of CPU) has decode_func at 11.4% of the CPU. Two decodes make it up:

- fe_replay decodes every body from the front end cache. Decision 0382's correction stands: the
  replay commits each entry's events in order, so the module grows as a serial build grows it; that
  decode is a floor.
- cln_hits decodes every function the cleanup cache holds a second time, now in its post-cleanup
  form (ssa.cln_hits, 6.47% of the CPU). The reference counting and inline caches then leave
  their hits undecoded (decision 0283 stage B), so most of these bodies are read only by the
  passes between the cleanup and the reference counting.

Those passes, and what each reads of a function's body:

- specialize_args (decision 0368): each call's callee and each argument's constant (spec_key);
  of a callee, spec_callee and spec_uses; spec_clone copies a callee's whole body; a rewritten
  call changes its caller.
- rc_prepare_incr: immortal_returns; reuse_var_params (it rewrites functions with `var`
  parameters); borrow_hashes, init_borrows and borrow_callers (names and parameter types); the
  borrow rounds (the inferred functions' bodies); ret_views; apply_uniq (decision 0322).

Measured with a diagnostic (XO_CLN_LAZY_STATS=1 on the prototype branch: of the functions the
cleanup cache read, those a later reader needs decoded), a warm unchanged build on darwin/arm64:

| program | functions | cleanup hits | spec | var params | borrow | uniq | forced |
|---|---|---|---|---|---|---|---|
| json_decode_fields | 1543 | 1539 | 8 | 54 | 0 | (not counted) | 57 without uniq |
| http_server_request | 3301 | 3251 | 32 | 197 | 0 | 3146 | 3147 (97%) |

Without apply_uniq's readers, 93 to 96% of the hits need no body. apply_uniq is the blocker:
uniq_facts (compiler/ssa uniqvar.xo) is a whole-program analysis over bodies (each candidate's
callers' uses and dominators) and marks entry Reuses in place, and the candidates and their
callers are nearly every function.

## Decision (proposed)

1. An incremental uniqueness analysis, the shape of the incremental borrow inference: a record of
   each function's uniq facts (entry, out, ret) by a hash of its body id and the facts it reads of
   its callees and callers; a warm build recomputes only the functions whose hash is new and those
   the analysis reaches from them, and takes the rest from the record. XO_UNIQ_CHECK=1 runs the full
   analysis beside it and fails on a difference, as XO_BORROW_CHECK=1 does.
2. Unforced cleanup hits stay entries: a shell (lazy_shell's fields) in Module.funcs, the cleanup
   entry in Module.lazy, and BodyFacts beside the entry (stored with it, the cleanup cache's
   version bumped) extended with what the readers above need: the calls with their arguments'
   spec_keys, spec_callee and spec_uses, the parameter types, the return view. The readers take
   facts for a lazy function, as they do for std's externs (decision 0388 stage 3), and force a
   function before rewriting it: specialize_args' rewritten callers and clone sources, `var`
   parameter functions, the borrow inference's dirty functions, entry Reuse marks, and the
   reference counting's misses.
3. Each step lands with the forced-hit rate beside its s1ab result, warm builds equal to cold over
   the golden set, scripts/identity, TestBackendCacheEditIdentity and XO_BACKEND_CACHE=verify
   clean; verify forces every function.

## Consequences

- Ceiling: cln_hits' 6.5% of a warm build's CPU, less the facts' decode and the forced functions
  (about 7% of the hits on http). Step 1 alone gains apply_uniq's own CPU on a warm build.
- Two more records in the back-end cache (uniq facts, extended cleanup facts) and one more
  verification switch.
- A reader added between the cleanup and the reference counting must take facts or force.

## Alternatives

- A whole-module cache of apply_uniq's result: helps an unchanged build only; any edit changes
  its key and forces every hit again.
- Skip the second decode by keeping fe_replay's decoded body and redoing the cleanup: the cleanup
  is what the cache saves (decision 0382 stage 2 measured it a wash).
- Leave it: decode_func stays the largest single cost of a warm build.
