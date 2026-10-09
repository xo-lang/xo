# A v2+ version in a require without /vN is XO1301, not a fetch error

Status: Observed (2026-10-09)

Symptom: after `/vN` major version paths were added (decision 0086), the
"no tag" case of `TestFetchProblems` (`require github.com/acme/money
v9.9.9`) reported XO1301 ("major version 9 needs the module path
github.com/acme/money/v9") instead of the XO1302 it expected. The
`xo explain XO1302` example had the same version.

Cause: a module path without a `/vN` suffix takes only v0 and v1 tags,
checked while parsing xo.mod, before anything is fetched.

Fix: test fixtures and examples for a missing tag use a v1 version
(`v1.9.9`). To require major version N >= 2, write the `/vN` path:
`require github.com/acme/money/v9 v9.9.9`.
