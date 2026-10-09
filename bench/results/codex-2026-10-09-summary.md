<!-- Written by Codex as the benchmark subject (runs/codex/SUMMARY.md, copied unedited 2026-10-09). Links below point into runs/codex/, which is gitignored. -->

# Codex benchmark summary

All 30 solution cells completed the authorized loops. All 15 planted cells were authored before scoring, with base tests unchanged. No commits or pushes.

| Measure | Codex Xo | Claude Xo | Codex Go plain | Claude Go plain | Codex Go strict | Claude Go strict |
|---|---:|---:|---:|---:|---:|---:|
| First try pass rate | 3/10 | 9/10 | 10/10 | 8/10 | 8/10 | 2/10 |
| Runs in total | 20 | 11 | 10 | 12 | 12 | 18 |
| Lines (code and tests) | 1564 | 5211 | 248 | 9630 | 256 | 10202 |
| Green now (current toolchain) | 10/10 | 8/10 | 10/10 | 10/10 | 10/10 | 10/10 |
| Diagnostics on final code | 0 | 2 | 15 | 68 | 0 | 0 |
| Hidden checks, task 01 | 17/17 | 17/17 | 17/17 | 17/17 | 17/17 | 17/17 |

The Xo run total includes one sandbox cache-write failure. The mistyped executable path did not launch Xo and was not counted. Go source was written densely with multiple statements per line; raw line counts are not a fair verbosity or effort comparison with formatted Claude code. Green means the prescribed checks passed, not that every requirement was independently verified. Claude Xo task 03 currently fails tests and task 06 currently fails checking, despite their recorded green histories.

| Task | Codex Xo runs | Claude Xo runs | Codex Go plain runs | Claude Go plain runs | Codex Go strict runs | Claude Go strict runs |
|---|---:|---:|---:|---:|---:|---:|
| 01 crud-api | 3 | 1 | 1 | 1 | 1 | 1 |
| 02 worker-pool | 1 | 1 | 1 | 1 | 1 | 2 |
| 03 retry-client | 1 | 1 | 1 | 1 | 1 | 2 |
| 04 file-sync | 2 | 1 | 1 | 1 | 1 | 2 |
| 05 rate-limiter | 2 | 1 | 1 | 1 | 1 | 2 |
| 06 job-queue | 2 | 1 | 1 | 2 | 2 | 2 |
| 07 cache | 4 | 1 | 1 | 1 | 2 | 2 |
| 08 notes-service | 2 | 2 | 1 | 1 | 1 | 1 |
| 09 inventory | 1 | 1 | 1 | 1 | 1 | 2 |
| 10 log-pipeline | 2 | 1 | 1 | 2 | 1 | 2 |

Planted Go cells: each fraction is caught out of five task variants. Strict findings count only additions to the unmodified base.

| Class | Codex Go plain | Claude Go plain | Codex Go plain + strict | Claude Go plain + strict |
|---|---:|---:|---:|---:|
| X1 ignored error | 3/5 | 1/5 | 3/5 | 1/5 |
| X2 nil dereference | 4/5 | 4/5 | 4/5 | 4/5 |
| X3 missing case | 1/5 | 1/5 | 3/5 | 3/5 |
| X4 data race | 0/5 | 0/5 | 2/5 | 2/5 |
| X5 goroutine or resource leak | 1/5 | 0/5 | 1/5 | 0/5 |
| X6 lost update | 3/5 | 3/5 | 4/5 | 4/5 |
| N1 off by one | 5/5 | 5/5 | 5/5 | 5/5 |
| N2 wrong status or kind | 5/5 | 5/5 | 5/5 | 5/5 |
| N3 wrong arithmetic | 3/5 | 3/5 | 3/5 | 3/5 |
| N4 wrong condition | 4/5 | 4/5 | 4/5 | 4/5 |

Xo cells below show **I/C/T/W/E**: inexpressible / caught by checker / caught by tests / escaped with new warning / escaped without warning. Each row has five cases. Inexpressible sometimes reflects a missing operation in the supplied Xo base, rather than a language guarantee. Translation and independent variants differ from Go and from Claude; detection rates are descriptive, not a controlled causal comparison. The scorer subtracts baseline warnings, but not baseline test failures; a caught-by-test result alone does not prove that the planted change caused the failure.

| Set | Class | Codex I/C/T/W/E | Claude I/C/T/W/E |
|---|---|---|---|
| xo-translate | X1 ignored error | 2/2/0/0/1 | 0/5/0/0/0 |
| xo-translate | X2 nil dereference | 2/0/3/0/0 | 5/0/0/0/0 |
| xo-translate | X3 missing case | 1/0/2/0/2 | 0/5/0/0/0 |
| xo-translate | X4 data race | 5/0/0/0/0 | 4/1/0/0/0 |
| xo-translate | X5 goroutine or resource leak | 3/0/2/0/0 | 3/0/1/0/1 |
| xo-translate | X6 lost update | 2/2/1/0/0 | 0/3/2/0/0 |
| xo-translate | N1 off by one | 0/0/4/0/1 | 0/0/4/0/1 |
| xo-translate | N2 wrong status or kind | 0/0/3/0/2 | 0/0/3/0/2 |
| xo-translate | N3 wrong arithmetic | 0/0/3/0/2 | 0/0/4/0/1 |
| xo-translate | N4 wrong condition | 1/0/3/0/1 | 0/0/5/0/0 |
| xo-independent | X1 ignored error | 0/2/1/0/2 | 0/1/2/0/2 |
| xo-independent | X2 nil dereference | 0/0/5/0/0 | 1/4/0/0/0 |
| xo-independent | X3 missing case | 0/1/2/1/1 | 0/0/4/0/1 |
| xo-independent | X4 data race | 1/0/2/2/0 | 2/0/1/2/0 |
| xo-independent | X5 goroutine or resource leak | 0/0/3/0/2 | 3/0/0/0/2 |
| xo-independent | X6 lost update | 0/0/5/0/0 | 0/3/2/0/0 |

Where Xo helped or hurt compared with Go:

- Structured task scopes, cancellation, fake clocks, map_concurrent_until, and Once made pool/cache code direct and reproducible.
- Option and closed error variants made missing values and error classification explicit. Value semantics required deliberate collection writeback.
- Xo needed more correction runs while I learned its syntax, effect arguments, imports, and standard-library signatures. Go plain was first-try green everywhere, but strict scoring found escaped findings.
- Xo Clock.sleep cancellation unwinds rather than returning a typed cancellation error, complicating the retry API.
- Xo MemFs isolates tests from actual project source; my dependency guard test tests forbidden-import recognition rather than inspecting actual imports. Go tests inspect source imports directly.
- Xo blocked stdin lacks a selectable stop-request API in the permitted spec; my Xo log program does not fully satisfy signal shutdown while read_line is blocked. Go closes the reader, drains accepted work, and joins workers.
- Strict Go required explicit error handling and corrected a copied atomic in the queue test and an error-sentinel naming finding in the cache.

Spec and task ambiguities encountered:

- Character means Unicode scalar count, rather than bytes or grapheme clusters. Email uniqueness case sensitivity was unspecified; my solution cells compare exactly.
- CRUD null name/email patch behavior differed between my Xo interpretation (keep via apply_to) and Go (reject); null nickname clears. Unknown-key and body-size policy were unspecified.
- Worker Job/result/error shapes and clock argument were unspecified. I chose URL jobs, integer HTTP statuses, and an explicit clock/context. Invalid worker count policy was unspecified.
- Go processors/loaders must cooperate with context cancellation; arbitrary callbacks cannot be forcibly stopped. Timeout completion and elapsed-time semantics for noncooperative callbacks were unclear.
- Retry Profile shape and endpoint were unspecified. I chose id/name and injected or fixed an upstream base.
- Retry policy defaults versus explicit zero values required a constructor in Go; validation of negative durations/zero attempts was unspecified.
- Jitter applies after the exponential cap; Retry-After bypasses jitter. Integer seconds, malformed headers, unsupported statuses, and body-read errors needed interpretation.
- Returning FetchErr.Cancelled while Clock.sleep unwinds is a spec/API tension.
- Filesystem destination metadata must be readable despite the read-src/write-dst authority wording; write_only would block required comparisons.
- Fs.walk symlink semantics and case folding versus actual volume settings were unspecified. Go refuses symlinks and uses rooted handles.
- Filesync missing destination dry-run, overlapping trees, cleanup failures, temporary-name collisions, and destination type changes needed policy. My Go dry-run currently requires an existing destination.
- Rate-limiter invalid configuration and zero refill behavior were unspecified. Idle means last request, including denial. Empty header interpretation differed from the supplied planted base.
- Lease attempts increment on lease; expiry immediately redelivers, while explicit nack backoffs. Whether lease expiry also needs backoff was ambiguous.
- Worker-id persistence and the example job handler were unspecified. Queue executables expose the API; the runner is implemented/tested separately.
- Xo queue shutdown test exercises scope cancellation rather than a real process signal/grace deadline.
- Cache TTL begins when loading finishes. Errors replace stale successes, invalidate fences old loads, and capacity must be positive; these boundary policies were unspecified.
- Cache upstream URL and exact HTTP error schema were unspecified. Uncooperative Go loaders can retain a goroutine after timeout.
- Notes newest-first ordering with equal timestamps was unclear; monotonic ids provide creation order. Cursor carries last id, and response shape is notes/cursor.
- Notes cross-process locking, filesystem-error classification, empty tags, cursor validation, and deletion between pages were unspecified.
- Inventory idempotency keys are global and retained indefinitely; repeated terminal operations conflict. Key expiry/reuse and repeated cancel behavior were unspecified.
- Log flushes are cumulative snapshots; reset behavior, restart filename collisions, message ties, line limits, and read/accepted boundaries were unspecified.
- Xo tuple Json support and Duration.parse were not defined; used Frequency structs and JSON Duration decoding.
- Xo omitted generic effect arguments stayed abstract in a concrete service; an explicit net.dial effect argument was required.
- I initially used Rust-style Hash + Eq constraints, comma-separated match arms, and var at call sites; these were mistakes from other-language habits, not demonstrated spec defects.
- Some planted translations were inexpressible because the Xo base lacks streamed response reads, nil destination roots, lazy shared Fetcher fields, special-file kind classification, or Go-style policy normalization.
- Task/resource-lifetime and filesystem-atomicity planted analogs do not establish equivalence to Go detached goroutine leaks or memory races.

Detailed results: [solutions](solutions.md), [planted](planted.md), [Claude solutions](claude-solutions.md), [Claude planted](claude-planted.md).
