# Notes

## Layout

- `pool/pool.go`: generic worker pool, `pool.Run` and `pool.RunWithOptions` (injectable `Clock`).
- `pool/pool_test.go`: the four required tests plus parent cancellation and empty input.
- `main.go`: reads URLs from stdin, fetches with 8 workers and a 30 second overall timeout, prints one line per URL and a summary.
- `main_test.go`: end to end test of the program logic against an `httptest` server on loopback.

## Ambiguities resolved

- Signature: `run_pool(jobs, workers, timeout, f)` became `Run[T, R](ctx, jobs []Job[T], workers, timeout, f func(ctx, T) (R, error)) Report[R]`. A context parameter was added because idiomatic Go passes cancellation through `context.Context`, and `f` receives a context so network calls can be cancelled.
- Job identity: `Job` has a string `ID` and an `Input`. Results and failures also carry the job's index so callers can merge them back into input order (the program uses this to print lines in input order, even with duplicate URLs).
- "Successful results" are `Result{Index, JobID, Value}` rather than bare values, sorted by original job order. Failures are also sorted by original job order.
- Timeout semantics: the overall timeout covers the whole run. When it fires, the context passed to `f` is cancelled, no new jobs start, and every job without a delivered outcome is reported as a failure that `errors.Is(err, pool.ErrTimeout)`. If a job returns a context error after the timeout, the failure wraps both `ErrTimeout` and the job's error. Outcomes delivered before the timeout are kept.
- `f` that ignores its context: `Run` returns at the timeout and does not wait for such calls (their goroutines finish later in the background). Waiting would make the timeout meaningless. The concurrency bound still holds for calls the pool starts.
- Parent context cancellation is reported like a timeout but with the parent's cause (for example `context.Canceled`), not `ErrTimeout`.
- `timeout <= 0` means no overall timeout. `workers < 1` is treated as 1.
- Elapsed time uses the injected clock, so tests can assert it exactly.
- Program: blank lines and surrounding whitespace in input are ignored. Any HTTP response, including 4xx and 5xx, counts as success and its status code is printed; only transport or request errors are failures. Output format is `<url> <status>` or `<url> error: <err>`, then `summary: N urls, X ok, Y failed, elapsed D`. The response body is read up to 1 MiB and discarded so connections can be reused.

## Test approach

- No real time sleeps. The timeout test uses a fake clock whose timer channel the test fires by hand after confirming the slow jobs started.
- Ordering test forces reverse completion by chaining: job i waits for job i+1 to finish.
- Concurrency test holds jobs on a release channel and lets exactly one new job start per release, asserting the observed maximum equals `workers` exactly (so the limit is both reached and never exceeded).
- Tests have short safety timeouts (`time.After`) only as hang guards; they never fire on success.

## Bugs caught by the loop

None. The first run of `go build && go vet && go test` passed, so the loop caught no bugs in this solution.
