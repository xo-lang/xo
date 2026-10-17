# arm64: "reads virtual register N before it is written" after a flag loop with break

Status: Observed (2026-10-13, decision 0193 step 6, std/native/llrt/http1.xo); fixed 2026-10-13

Symptom: `xo build --backend=arm64` failed with `arm64 backend:
internal: unescape reads virtual register 20 before it is written`, on
the Go driver (`XO_SELF_DRIVER=0`) and on xo1 alike (the same register:
the two emitters agree). The LLVM backend built the same program and it
ran correctly. Repro (user code, no runtime module needed):
internal/gen/testdata/golden/break_flag_join (also in
internal/llvm/testdata/golden for the native goldens); minimized:
internal/arm64/testdata/run/break_flag_join:

```
var i = 0
var plain = true
while i < hi {
  if Int(b[i]) == 37 {
    plain = false
    break
  }
  i += 1
}
if plain {
  return -1
}
// a second loop over b[i..]
```

Cause: instruction selection's memo (registers already holding a
constant, a list length, a data address, a load, a quotient) carried
into a block from the block that dominates it. Decision 0184 added a
third source: a block b entered through a join J that holds nothing
but a branch on its Bool parameter, where every predecessor of J but
one (p) passes the constant that leaves by J's other edge (`a && b`,
`a || b`, and here the flag: the loop's exit passes `true`, the break
`false`). Execution reaches b only through p, so b took p's whole memo,
registers included. But the control flow graph still has J's other
predecessors reach b unless MIR threading (`threadConst`) removes those
edges, and it threads only a constant set in the predecessor itself.
Here the loop's exit passes a `true` the loop's header materialized, so
the edge stayed, and the data address of b that only the loop's body
computed (p's memo) was read in the second loop on a path where nothing
wrote it. Register allocation's liveness found it live into the entry.
The simpler `found = true` form passes xzr for `false`, which
`threadConst` threads, so it built.

Fix: a block entered through such a join takes its registers only from
its dominator's memo (as any block with several paths does, memory
facts clobbered) and the facts of p's memo (bounds, `x < y` relations,
passed bounds checks, low zero bits), which hold on every path that
runs and create no register uses: `joinFacts` in internal/arm64/opt.go
and `join_facts` in compiler/arm64/opt.xo, identical. Cost: the
registers p computed are computed again in b (bench/perf/httpparse: 7
functions of http1.xo differ, 39 more instructions in all; the h1
workloads in bench/results/runtime-port-step6-2026-10-13.md).

How to avoid: a memo entry is a register use at the block that takes
it; it is sound only where its definition dominates that block in the
MIR's graph, not merely on every path that executes. Facts are free to
travel along executed paths; registers are not. To find such a bug,
dump the MIR after `isel` and look for a block reached around the
block that defines the register.
