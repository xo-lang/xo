# A sliced prune stalls when listing its shards outruns its budget

Status: Observed (2026-10-21, xo-fa)

## Symptom

After the back-end cache's prune became sliced (decision 0223 addendum,
6a8b47c8), the live cache grew from 15.2 to 32.9 GB in 11 minutes
under a 16 GiB cap. Its `prune-log` showed every run walking a single
shard and evicting nothing:

    slice 1/142425 shards from "chk/6e", walk 4180 ms, 152909 bytes before, 152909 after, 0 removed

## Cause

The old pack layout, `pack/<slot>/` (one directory per build target,
138,000 of them in one directory), was no longer read. It was left in
place on the reasoning that it would age out, but every slot was still a
shard. Each run listed the shards with read_dir, an lstat per entry,
which took 4 to 5 s. That alone used up the 300 ms walk budget, so a run
walked one shard. The ledger's total covered only the shards measured
so far, so it stayed under the cap and nothing was evicted.

## Fix

10f0be9d: the walk budget counts the shards' walk, not their listing.
The detached prune renames a layout the cache no longer reads to
`trash-<name>-<ms>` (prune.retire) and removes `trash-*` directories
(prune_trash), and the shard listing leaves `trash-*` out. A one-off
move and removal of the old pack/ freed 20.23 GB.

## How to avoid

When a cache's layout changes, retire the old one explicitly; don't
leave it to age out. Before shipping a prune that lists its shards each
run, count the top-level children (`find DIR -mindepth 2 -maxdepth 2
-type d | wc -l`) and time the listing against the run's budget.
