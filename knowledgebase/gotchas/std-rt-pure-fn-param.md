# A std/rt method's `fn() -> T` parameter is `func(*Task) Result[T, Never]`

Status: Observed (2026-10-30, Request.with_trailers, decision 0433)

Symptom: a hand-written std/rt method taking `f: fn() -> Map[Str, Str]`
as `func(*Task) Map[string, string]` failed `go vet ./std/rt/` in the
generated signature table: "cannot use (*Http_Request).M_with_trailers
(...) as func(*Http_Request, *Task, func(*Task) Result[Map[string,
string], Never]) Http_Request value in variable declaration".

Cause: the Go backend gives every function value a result type of
`Result[T, E]`, with `E` Never when the Xo type has no `!`; only the
method's own result is unwrapped. std_sigs_*.go (scripts/stdgen) pins
each std method's Go signature, so a mismatch fails the build there.

Fix: declare such a parameter `func(*Task) Result[T, Never]` and read
the value with `.V` (as Http_stream's `func(*Task, Http_BodyWriter)
Result[Unit, IoErr]`); check a new std/rt method with `go vet ./std/rt/`
after `go run ./scripts/stdgen`, before a build.
