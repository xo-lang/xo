# A struct that contains itself through Option is an invalid Go type

Status: Observed (2026-10-07, P1 code generation)

Symptom: `struct Node { value: Int, next: Node? }` failed `go build`:
"invalid recursive type Node". `rt.Option[T]` stores T inline.

Fix: a field whose type contains its struct by value (through Option,
Result, tuples, newtypes, or other structs) is stored as `rt.Box[T]`, an
immutable shared pointer: reads call `.Get()`, writes store `rt.BoxOf(v)`,
so value semantics hold. Enums never need it (payloads are `any`), nor do
collections. Golden test: `places_boxes`.
