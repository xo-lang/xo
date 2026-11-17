# std/protowire changes

## 2026-11-17: change: the unused bool writer wb is removed

- Kind: change
- Commit: this commit
- Found by: TestStdAPICovered on main (protowire.wb had no caller)
- Go reference: none (internal wire writer shared by protodesc, protowkt and protojson)
- Test: internal/repotest TestStdAPICovered
- Shipped in: not yet released

`protowire.wb` wrote a bool only when true. Its last caller moved to
the seen-aware `wbp` in 6f818c531 (a false that was on the wire is
written back), so the helper is removed. std/protowire is internal:
no user code could call it.
