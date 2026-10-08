# 0019: Stdlib surface spec; HTTP error to status mapping

Status: Accepted (2026-10-07)

## Context
Most of the 116 paper test guesses were stdlib APIs (http, fs, clock, stdio,
env, proc, Duration, Str). Appendix B question 4 (handler errors to status
codes) blocked every HTTP task. (Paper test 2026-10-07, [results](../../bench/results/paper-2026-10-07.md))

## Decision
- `spec/std.md` defines the prelude and std surface: capabilities, http, json, log, task, testing.
- Handler errors implementing `http.StatusError` (`status`, `display`) become that status with `{"error": ...}`; others become 500 and are logged.
- `http.Router` is a value with `call(req)` for in process tests; registering a handler counts as performing its effects.
- `json.Patch[T]` distinguishes absent, null, and value.
