# std/ui changes

## 2026-11-15: feature: std/ui, local-first UI core (Early)

- Kind: feature
- Commit: this push
- Found by: decision 0659 stage 1 (owner, 2026-11-15)
- Go reference: none (Xo's own design; the reconciliation follows the experiment in bench/results/ui-local-first-2026-11-15.md)
- Test: std/ui_test.xo: rendering and escaping, page state escaping, prediction and confirmation, duplicates and gaps, an overturned prediction, snapshot after compaction, convergence over a lossy, reordering network (with and without compaction, and as a property)
- Shipped in: not yet released

New module, Early: `App` (init, a pure update, a view), `Html` views rendered with escaping and no raw HTML, `on` events as JSON data attributes, `page` (the rendered view plus the state for the client runtime), and the reconciliation protocol as pure values: `Server` (applies each client's Msgs once and in order, a log with acks, batches and compaction with snapshots) and `Replica` (predicts at once, replays the log, folds pending Msgs on top). Transport and the browser runtime come in later steps.
