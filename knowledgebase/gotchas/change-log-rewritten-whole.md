# A change log written whole keeps only the new entry

Status: Observed (2026-11-01, found 2026-11-02, Xo Lang 4).

**Symptom.** CHANGELOG.md on main held about 110 lines, all from
2026-11-01 on; std/changes/http.md held only that week's entries. The
release notes draft for 0.36 found it.

**Cause.** c9a2c0392 (decision 0447 step 1) wrote CHANGELOG.md and
std/changes/http.md whole with only its own new entries in them
(6,461 and 1,170 lines deleted). The commit's stat showed it
(`CHANGELOG.md | 6470 +----`), but nothing checked it, and every later
commit added its entry on top of the cut file.

**Fix.** Add an entry with an edit at the "## Unreleased" line (or the
first `## ` entry of a std change log), never by writing the file. Before
a push, read `git show --stat`: a log file with thousands of deleted lines
is a cut. Restored in 2588a1a34 from the commit's parent.
