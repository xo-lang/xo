# A package loaded from export data lists only the imports its API uses

Status: Observed (2026-10-30)

## Symptom

gobind's widened upcasts (decision 0416 addendum) gave
`impl.Hello` no `as_greeter` in the test fixture example.com/upwide, though
impl imports iface and Hello implements iface.Greeter; aws's
credentials providers did get `as_credentials_provider`.

## Cause

gobind reads packages from `go list -export` data (importer.ForCompiler).
A `*types.Package` from export data has `Imports()` only for the packages
its exported API refers to: impl used iface only in
`var _ iface.Greeter = (*Hello)(nil)`, so iface was not among them, while
credentials' API names aws.Credentials, so aws was.

## Fix

Take a package's direct imports from go list (`-json=...,Imports`, the
listed struct's Imports), not from `types.Package.Imports()`; the importer
still loads each one's export data, since `go list -deps` reported them.
