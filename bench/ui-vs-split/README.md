# ui-vs-split

Task #499's measurement of examples/ui-todo served by std/ui (wasm client
and thin) and by the hand written front end in examples/ui-todo/split.
Results: [../results/ui-vs-split-2026-11-20.md](../results/ui-vs-split-2026-11-20.md).

- `drive.mjs`: drives one running site in headless Chrome (Node 22,
  Google Chrome): `node drive.mjs URL [ADDS] [RTT_MS]`, one JSON object
  out; `TRACE=1` adds the requests of one more add.
- `raw/`: the three rounds behind the results file.

To rerun: `xo build -o ui ./server` and `xo build -o split ./split` in
examples/ui-todo; start `ui 127.0.0.1:8781`, `ui 127.0.0.1:8782 thin` and
`split 127.0.0.1:8783`, one at a time, and run the driver against each.
