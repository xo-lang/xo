# A conflict regex under re.S cut a change log to its new entries

Status: Observed (2026-11-01, found 2026-11-02, Xo Lang 4).

**Symptom.** CHANGELOG.md on main held about 110 lines, all from
2026-11-01 on; std/changes/http.md held only that week's entries. The
release notes draft for 0.36 found it.

**Cause.** Not a write of the file: a rebase conflict resolved by a
regex. c9a2c0392 prepended its entries correctly; on the rebase before
the push, the CHANGELOG and std/changes/http.md conflicts (both sides
adding a top entry) were resolved with

```python
re.sub(r'^<<<<<<< .*\n(.*?)^=======\n(.*?)^>>>>>>> .*\n',
       lambda m: m[1] + m[2], s, flags=re.M | re.S)
```

meant to keep both sides. Under `re.S` a `.` also matches a newline, so
the `.*` meant for the rest of a marker line runs across lines: the
greedy `>>>>>>> .*\n` took everything from the end marker to the file's
last newline. The match then replaced the conflict and the whole rest of
the file with the two sides' few lines. Reproduced with `git merge-file`
on the three versions of std/changes/http.md (312f5611e, 25322712b and
the commit before the rebase): 1,198 lines in, 23 out. The commit's stat
showed it (`CHANGELOG.md | 6470 +----`), nothing checked it, and later
commits added their entries on top of the cut file.

**Fix.** Match a marker line with `[^\n]*`, not `.*`, when the pattern
has `re.S` (or drop `re.S` and match the sides line by line), and assert
the line count after: a resolution that keeps both sides grows the file.
Read `git show --stat` before a push. pushq now refuses a push that
deletes more than 200 lines of CHANGELOG.md or std/changes/*.md
(1769a42e8; `PUSHQ_ALLOW_LOG_CUT=1` for a deliberate trim). Restored in
2588a1a34 from the commit's parent.
