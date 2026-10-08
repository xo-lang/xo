# How this run works

You are a subject in a benchmark that measures how well coding agents write
correct Go under strict static analysis. Solve the task below in Go 1.26.

1. Write the complete solution, including every test the task lists, before
   you see any tool output. Your first reply must contain every file.
2. Standard library only. No third party modules (no `go.sum`).
3. The task text was written for several languages and uses a neutral
   notation for signatures: `List[Job]` is a list of `Job`, `Int` and `Str`
   are integers and strings, `Str?` is optional, `T ! E` means "returns `T`
   or fails with error `E`". Translate it to idiomatic Go names and types.
4. After each reply the harness runs `strict-check.sh` in the module root
   and shows you the output. It runs, in order: `go vet ./...`,
   `staticcheck ./...`, `errcheck ./...`, `nilaway ./...`,
   `exhaustive ./...`, and `go test -race -count=1 ./...`. The solution is
   green when every check passes. The run stops as soon as it is green.
5. When it is not green, reply with the files you changed, each one in full.
   Files you do not send are kept as they were. At most {{max_runs}} runs in
   total; the first reply counts as run 1.
6. Fix findings in the code. Do not silence a tool with `//nolint`,
   `//lint:ignore`, or a `_ =` discard, and do not delete or weaken a test.
   If you judge a finding a false positive, restructure the code so the
   tool can see it is safe, and record it in `NOTES.md`.
7. Also write `NOTES.md`: design decisions, the ambiguities in the task you
   had to decide, and every analyzer finding with your judgment (real bug or
   false positive).
