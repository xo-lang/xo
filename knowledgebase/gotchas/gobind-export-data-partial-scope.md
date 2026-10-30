# A dependency's scope from export data holds only what the importing package names

Status: Observed (2026-10-29, Lang 1, decision 0416 upcasts)

Symptom: gobind's bindUpcasts offered bytes.Buffer only `as_reader` and `as_writer`, never io.WriterTo,
io.ByteReader or io.ReadWriter, and net/url no fmt.Stringer. A debug print showed `g.pkg.Imports()` of
net/url with one package and io's scope with two names.

Cause: the gc importer reads the bound package's export data, which carries only the objects of its
dependencies that its own API mentions. `pkg.Imports()` and an import's `Scope()` are those partial views,
not the dependencies' APIs.

Fix: import the dependency itself through the same importer (`g.imp.Import("io")`). It shares its package
map, so the `*types.Package` is completed in place and its types stay identical to the ones the bound
package's signatures use (`types.AssignableTo` works). A path with no export data (not a dependency) fails
the import and is skipped.
