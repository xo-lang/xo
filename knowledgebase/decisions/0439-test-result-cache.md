# 0439: replay an unchanged test run's results

Status: Proposed (xo-lead queue for xo-lang-4, 2026-10-30)

Stub: being filled in. `xo test` reruns every test even when the test binary, seed and flags are unchanged; Go replays a passed package's result. This decision replays a passed run whose inputs cannot have changed, and never one whose tests touched the real world outside the fakes.
