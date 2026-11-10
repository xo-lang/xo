#!/usr/bin/env python3
"""release-coverage.py NOTES.md vPREV [REV]

Prints what a release ships that its notes do not mention (lead, 2026-11-10:
0.46.0's draft notes missed decision 0574's C libraries, in the candidate
with no CHANGELOG line, and nine std modules' features, which std logs in
std/changes rather than CHANGELOG.md, decision 0276).

Two lists, over the commits vPREV..REV (REV defaults to HEAD), run in the
candidate's checkout:

1. Decisions. The first decision number of each commit subject (`decision
   0574`, `ADR 0574`, `0574:`; later ones are cited in passing) whose commit
   changes code (compiler/, std/, or a
   tool the release ships), and whose ADR is Accepted. Covered when NOTES.md
   names the number anywhere (`0574`, in the text or a comment).
2. std/changes entries. Every heading `## DATE: KIND: TITLE` added to
   std/changes/*.md in the range, of a kind users see (feature, port, fix,
   behavior, security, breaking; perf is left to the notes' Performance
   section). Covered when NOTES.md names the change log file
   (`std/changes/gif.md`), its module (`std/gif`), a backticked name of the
   title, or a dotted name in it (`json.decode_go`).

An item left out on purpose is named in a comment of NOTES.md:
`<!-- coverage-skip: 0564 (reverted) -->` or
`<!-- coverage-skip: std/changes/http.md 2026-11-08 (test only) -->`.

Exit 0 when everything is covered, 1 when something is not (each printed),
2 on a usage error.
"""
import os
import re
import subprocess
import sys

USER_KINDS = {"feature", "port", "fix", "behavior", "security", "breaking"}
DEC = re.compile(r"\b(?:decision|decisions|ADR)\s+(\d{4})\b|^(\d{4}):", re.I)
CODE = re.compile(r"^(compiler/|std/(?!changes/)|internal/xotool/|scripts/xobuild/)")


def git(*args):
    return subprocess.run(["git", *args], check=True, capture_output=True, text=True).stdout


def adr_status(num):
    d = "knowledgebase/decisions"
    for f in sorted(os.listdir(d)):
        if f.startswith(num + "-"):
            for line in open(os.path.join(d, f), encoding="utf-8"):
                if line.startswith("Status"):
                    return f, line.strip()
            return f, ""
    return None, ""


def main(argv):
    if len(argv) not in (3, 4):
        print(__doc__.split("\n\n")[0], file=sys.stderr)
        return 2
    notes_path, prev = argv[1], argv[2]
    rev = argv[3] if len(argv) == 4 else "HEAD"
    try:
        notes = open(notes_path, encoding="utf-8").read()
    except OSError as e:
        print(f"release-coverage: {e}", file=sys.stderr)
        return 2
    skips = " ".join(re.findall(r"coverage-skip:([^>]*)-->", notes))
    rng = f"{prev}..{rev}"
    missing = 0

    # 1. decisions cited by code commits
    cited = {}
    for block in git("log", "--format=%x00%h %s", "--name-only", rng).split("\x00")[1:]:
        lines = block.strip().splitlines()
        if not lines:
            continue
        head, files = lines[0], lines[1:]
        if not any(CODE.match(f) for f in files):
            continue
        subject = head.split(" ", 1)[1] if " " in head else ""
        # the subject's first decision is the commit's own; later ones are
        # cited in passing (`decision 0574 ... as decision 0259 does`)
        m = DEC.search(subject)
        if m:
            cited.setdefault(m.group(1) or m.group(2), head)
    for num in sorted(cited):
        if num in notes or num in skips:
            continue
        f, status = adr_status(num)
        if f is None or "Accepted" not in status:
            continue
        print(f"DECISION {num} ({f}): {cited[num][:150]}")
        missing += 1

    # 2. std/changes headings added in the range
    diff = git("diff", rng, "--", "std/changes/")
    cur = None
    for line in diff.splitlines():
        if line.startswith("+++ b/"):
            cur = line[6:]
            continue
        m = re.match(r"^\+## (\d{4}-\d\d-\d\d): ([a-z]+)\b[^:]*: (.*)", line)
        if not m or cur is None or cur.endswith("README.md"):
            continue
        date, kind, title = m.groups()
        if kind not in USER_KINDS:
            continue
        stem = os.path.basename(cur)[:-3]
        names = [cur, f"std/{stem}"] + re.findall(r"`([^`]+)`", title) + re.findall(r"\b[A-Za-z_]\w*\.[a-z_]\w*", title)
        if any(n in notes for n in names) or f"{cur} {date}" in skips:
            continue
        print(f"STD {cur} {date} {kind}: {title[:150]}")
        missing += 1

    if missing:
        print(f"release-coverage: {missing} item(s) not in {notes_path} (add a line, or a coverage-skip comment with the reason)")
        return 1
    print("release-coverage: ok")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
