# Task 02: Worker pool

Write a reusable worker pool and a small program that uses it.

## Requirements

- `run_pool(jobs: List[Job], workers: Int, timeout: Duration, f)` processes
  every job with at most `workers` running at once, using `f` to process one job.
- Returns a `Report` with: the list of successful results in the **original
  job order**, the list of `(job_id, error)` failures, and the elapsed time.
- A failing job does not stop the other jobs.
- If the overall `timeout` passes, unfinished jobs are cancelled and reported as
  failures with a timeout error.
- `f` may perform network calls.
- The program: reads a list of URLs from standard input (one per line), fetches
  each with 8 workers and a 30 second overall timeout, and prints one line per
  URL with status code or error, then a summary line.

## Tests to write

- Results keep original order even when later jobs finish first.
- One failing job does not affect others.
- Timeout cancels slow jobs and reports them.
- Never more than `workers` jobs run at the same time.
