# Editing a fixture's xo.mod breaks its xo.lock hashes

Status: Observed (2026-10-11, rewriting every xo.mod to `edition preview`,
decision 0155)

**Symptom.** After a mechanical edit of xo.mod files, `go test ./internal/mod`
fails `TestMVSAndLock` and `TestTidy` (`mvs.lock differs`, every hash
changed), and the `effect_ceiling_vendor` golden of `internal/check` gains
`XO1304` "content hash ... does not match xo.lock" for each vendored module.

**Cause.** A module version's content hash (core 4.3) covers its whole
tree, xo.mod included, and the `/xo.mod` line hashes xo.mod alone. The
fixture repositories under `internal/mod/testdata/repos/` and the modules
under a golden's `vendor/` are "published" versions whose hashes are
recorded in golden xo.lock files.

**Fix.** For `internal/mod`, regenerate with
`go test ./internal/mod -run 'TestMVSAndLock|TestTidy' -update` and review
that only hashes changed. For a vendored golden, either leave the vendored
xo.mod files as they were (a published version never changes; that is
what was done for `edition 2026` there) or rewrite the golden's xo.lock
with the new hashes. A sweep that rewrites xo.mod files should exclude
`vendor/` directories.
