# std/path changes

## 2026-10-21: test: Go's path package on its test strings

- Kind: test
- Commit: (filled in after the push)
- Found by: the std coverage table (decision 0276: a shipped module without Go's tests)
- Go reference: path Clean, Join, Dir, Base, Ext and path/path_test.go (go1.26.1)
- Test: std/native/path_test.xo (3,549 cases: clean, dir, base, and ext of every string literal of path_test.go and 600 random paths, and 801 joins)

No difference found: the native port gives Go's result on every case.
The Go backend calls Go's path package itself.
