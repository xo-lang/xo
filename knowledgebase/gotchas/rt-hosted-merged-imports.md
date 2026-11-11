# A std module generated into std/rt names `MISSING_PKG_x_std_M`

Status: Observed (2026-11-11, Xo Lang 3: std/image's merge, then std/norm
and std/backoff's)

## Symptom

After merging a std module (decision 0588), `go vet` or `go build` of
std/rt fails:

```
std/rt/std_gen_image.go:1216:30: undefined: MISSING_PKG_x_std_binaryF_get_u32_be
std/rt/std_gen_otlp.go:156:13: undefined: MISSING_PKG_x_std_backoffBackoff
```

TestStdRTBuildTags (internal/repotest) fails the same way.

## Cause

Code generated into package rt (every unmerged go_compiled std module's
std_gen_M.go, and every merged module listed in std/rt_hosted.txt) can
only call what is in package rt. A merged module that is not rt-hosted
is generated with each program as its own package, which std/rt cannot
import. So when std/image (rt-hosted) uses std/binary (merged, not
hosted), or std/otlp and std/idna (unmerged, generated into rt) use a
newly merged std/backoff or std/norm, the generator has no package to
name.

## Fix

List the used module in std/rt_hosted.txt too (std/binary with
std/image; std/backoff with its merge, for std/otlp), or merge the user
too (std/idna merged with std/norm, so norm needs no hosting). Before merging a
module, find its std users with `grep -rl 'use std/M' std` and check
whether any of them is generated into std/rt (not in std/merged.txt, or
in std/rt_hosted.txt); if so, the merged module goes in rt_hosted.txt.
