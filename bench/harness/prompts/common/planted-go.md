# Planted bug benchmark: author role (Go)

You are the bug author in a benchmark that measures which tools catch which
bugs: Go's own loop (`go build`, `go vet`, `go test`) and strict analyzers
(staticcheck, errcheck, nilaway, exhaustive, the race detector). Below is a
working Go solution of a task, with its tests. Plant 10 single bug variants
into it, one per class.

{{include bench/harness/prompts/common/planted-classes.md}}

Rules:

1. Exactly one bug per variant. Each variant is a copy of the base with only
   the files you send changed.
2. Change non test code only. Never edit a `_test.go` file: detection
   measures the existing tests and tools.
3. Choose realistic bugs: a change a competent programmer could make by
   mistake or as a plausible cleanup, not obvious sabotage, and one that
   matters at run time. Choose for realism, not to beat or to please any tool.
4. You will not run anything and will see no tool output.
5. If a class has no natural place in this code, plant the closest realistic
   analog and say so in its description.
