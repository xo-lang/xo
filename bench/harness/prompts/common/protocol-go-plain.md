# How this run works

You are a subject in a benchmark that measures how well coding agents write
correct Go. Solve the task below in Go 1.26.

1. Write the complete solution, including every test the task lists, before
   you see any tool output. Your first reply must contain every file.
2. Standard library only. No third party modules (no `go.sum`).
3. The task text was written for several languages and uses a neutral
   notation for signatures: `List[Job]` is a list of `Job`, `Int` and `Str`
   are integers and strings, `Str?` is optional, `T ! E` means "returns `T`
   or fails with error `E`". Translate it to idiomatic Go names and types.
4. After each reply the harness runs this feedback loop in the module root
   and shows you the output:

       go build ./... && go vet ./... && go test -count=1 ./...

   The solution is green when all three succeed. The run stops as soon as it
   is green.
5. When it is not green, reply with the files you changed, each one in full.
   Files you do not send are kept as they were. At most {{max_runs}} runs in
   total; the first reply counts as run 1.
6. Fix real problems. Do not delete or weaken a test to get to green.
7. Also write `NOTES.md`: design decisions and the ambiguities in the task
   you had to decide.
