# offload-gate.sh --wait returns at once for a worktree with no gate yet

Status: Observed 2026-11-16 (Xo Lang 2)

## Symptom

A chain meant to run gates one after another,
`offload-gate.sh --wait A; offload-gate.sh B STEPS`, started B's gate
while A's was still queued. On 2026-11-16 a perf88c gate captured a HEAD
that a later commit then moved (the commit had to go to its own branch),
and a term gate chained on an unstarted gate was refused at heavyd's cap
of 2 leases per agent on one host (rc 5, "queue it when one ends").

## Cause

`--wait WT` waits for the gate that WT has queued or running. When WT's
own chain has not yet started its gate (it is itself waiting on an
earlier one), there is nothing to wait for and `--wait` returns 0 at
once.

## Fix

- Wait on every earlier gate in the order their chains start them:
  `for w in A B C; do offload-gate.sh --wait ~/xo-wt/$w; done; gate D`.
  A small window remains between one gate ending and the next chain
  registering its gate.
- Never commit in a worktree whose gate is chained but not started:
  the gate records HEAD when it starts, not when it is chained.
- An rc 5 refusal at the lease cap is not a test failure: rechain the
  gate behind the queue.
