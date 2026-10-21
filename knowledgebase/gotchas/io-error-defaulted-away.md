# An I/O error turned into a default answered from part of the input

Status: Observed (2026-10-21, a sweep of compiler/ after a bare `xo def`
miss at a worktree root under memory pressure)

Symptom: `xo def` answered "no declaration" for the compiler's own names
three times in a row, then never again; nothing reported an error.

Cause: `c.fs.read_dir(dir).ok() ?? []` in the checker's root walk: a
listing that failed for a transient reason (no file descriptors or
memory) looked like an empty directory, so a whole subtree's modules were
left out. The same shape elsewhere was worse: `xo mod tidy` read xo.mod
with `.ok() ?? b""` and could write it empty; the effect and escape hatch
audits passed on a partial tree; `xo edit` could restore a file as
empty; `xo test` could replace a regressions file. The runtime has the
class too (std/native/llrt names() ends a listing on a read error and
returns success; reported to std's owner).

Fix: match the result, keep the default only for the errors it is right
for (`Err(NotFound(_))` for a missing file), and return or report the
rest. `xo vet` warns on the shape (XO0903); a default that is right for
every error (a hint, a cache miss that is rebuilt) says why with
`// xo:allow XO0903 <reason>`. The fixes are on the branches
check-walk-err, mod-audit-read-errs (XO1318), dev-discovery-errs, and
unread-overwrites.
