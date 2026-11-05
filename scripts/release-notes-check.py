#!/usr/bin/env python3
"""release-notes-check.py NOTES.md VERSION PREV [CHANGELOG.md]

Checks a release's notes against CHANGELOG.md (Lead, 2026-11-04: v0.36.0's
sort change, `sort.slice` and its kin refused with XO1102, was in the
CHANGELOG but missing from the notes' breaking changes, found by the owner).

The entries checked are those of every CHANGELOG section after PREV (the
release before, the one users upgrade from; 0.38.1 follows 0.36.0, since
0.38.0 was never released) up to VERSION, or the Unreleased section when
VERSION has none yet.

1. Every entry marked breaking (`Breaking` or `BREAKING` in its first line,
   or an entry under a `### Breaking` heading) must be named in the notes'
   breaking section (a heading with "Breaking" in it): by a decision number
   the entry cites (`decision 0485`), or by a backticked name of its first
   line (`netip.Addr`). An entry with neither is matched by its first eight
   words. A missing one fails the check (exit 1).
2. An entry not marked breaking whose text reads as one (a name refused,
   skipped or removed, a field made private, an interface that gains a
   method) is listed as a suspect, so the notes' author decides; suspects
   fail the check only with --strict.

3. An entry in a released section (PREV or older) of the CHANGELOG must be in
   PREV's own CHANGELOG, as tagged (`git show vPREV:CHANGELOG.md` in the
   CHANGELOG's checkout), matched by its first 100 characters: one that is
   not was filed under a release it did not ship in, so the notes would miss
   it (Xo Lang 2, 2026-11-05: five changes after 0.39.0 sat under v0.39.0 and
   std/jpeg encode under v0.38.0). Each is printed as MISFILED and fails the
   check. Skipped, with a note, when the tag cannot be read.

Exit 0 when every breaking entry is in the notes and none is misfiled, 1 when
one is missing or misfiled (or, with --strict, a suspect), 2 on a usage error.
"""
import os
import re
import subprocess
import sys

BREAK = re.compile(r"\bbreaking\b", re.I)
SUSPECT = [
    (re.compile(r"\bis (?:now )?(?:XO1102|skipped|refused|rejected|an error)\b|\bare (?:now )?(?:skipped|refused|rejected)\b", re.I), "a name refused or skipped"),
    (re.compile(r"\b(?:removed|renamed|no longer (?:accepts?|compiles?|builds?|exists?|available))\b", re.I), "removed or renamed"),
    (re.compile(r"\b(?:are|is) (?:now )?private\b", re.I), "made private"),
    (re.compile(r"\binterface gains\b|\bgains? (?:a )?required\b", re.I), "an interface gains a method"),
]


def sections(text):
    """[(version or 'Unreleased', body)] in file order."""
    out = []
    heads = list(re.finditer(r"^## (Unreleased|v\d+\.\d+\.\d+)\b.*$", text, re.M))
    for i, m in enumerate(heads):
        end = heads[i + 1].start() if i + 1 < len(heads) else len(text)
        out.append((m.group(1), text[m.end():end]))
    return out


def entries(body):
    """[(first line, whole text, under a ### Breaking heading)]: top level
    `- ` items with their indented continuation lines."""
    out, cur, sub = [], None, ""
    for line in body.splitlines():
        if line.startswith("### "):
            sub = line[4:]
            cur = None
            continue
        if line.startswith("- "):
            cur = [line[2:], [line[2:]], bool(BREAK.search(sub))]
            out.append(cur)
        elif cur is not None and line.startswith((" ", "\t")) and line.strip():
            cur[1].append(line.strip())
        elif not line.strip():
            cur = None
    return [(c[0], " ".join(c[1]), c[2]) for c in out]


def vkey(v):
    return tuple(int(x) for x in v.lstrip("v").split("."))


def main(argv):
    strict = "--strict" in argv
    args = [a for a in argv if a != "--strict"]
    if len(args) not in (3, 4):
        print(__doc__.strip().splitlines()[0], file=sys.stderr)
        return 2
    notes_path, version, prev = args[0], args[1], args[2]
    cl_path = args[3] if len(args) == 4 else "CHANGELOG.md"
    notes = open(notes_path, encoding="utf-8").read()
    cl = open(cl_path, encoding="utf-8").read()
    secs = sections(cl)
    want = vkey(version)
    lo = vkey(prev)
    picked = [(v, b) for v, b in secs if v != "Unreleased" and lo < vkey(v) <= want]
    if not any(v != "Unreleased" and vkey(v) == want for v, _ in secs):
        picked += [(v, b) for v, b in secs if v == "Unreleased"]
    if not picked:
        print(f"release-notes-check: no CHANGELOG section after {prev} up to {version}", file=sys.stderr)
        return 2
    # the notes' breaking section: from a heading with "breaking" to the next heading
    m = re.search(r"^#+ [^\n]*breaking[^\n]*\n(.*?)(?=^#+ |\Z)", notes, re.I | re.M | re.S)
    brk = m.group(1) if m else ""
    brk_l = brk.lower()
    missing, suspects, marked = [], [], 0
    for v, body in picked:
        for first, whole, under in entries(body):
            if under or BREAK.search(first):
                marked += 1
                decs = re.findall(r"decision (\d{4})", whole)
                names = re.findall(r"`([^`]+)`", first)
                words = " ".join(re.sub(r"[*`]", "", first).split()[:8]).lower()
                hit = any(d in brk for d in decs) or any(n.lower() in brk_l for n in names)
                if not decs and not names:
                    hit = words in brk_l
                if not hit:
                    missing.append((v, first))
                continue
            for rx, why in SUSPECT:
                if rx.search(whole):
                    suspects.append((v, why, first))
                    break
    misfiled, tag_note = [], ""
    ptag = prev if prev.startswith("v") else "v" + prev
    shipped = subprocess.run(["git", "-C", os.path.dirname(os.path.abspath(cl_path)), "show", f"{ptag}:CHANGELOG.md"],
                             capture_output=True, text=True)
    if shipped.returncode != 0:
        tag_note = f"  (the misfiled check is skipped: {ptag}:CHANGELOG.md cannot be read)"
    else:
        keys = {first[:100] for _, b in sections(shipped.stdout) for first, _, _ in entries(b)}
        for v, b in secs:
            if v != "Unreleased" and vkey(v) <= lo:
                misfiled += [(v, first) for first, _, _ in entries(b) if first[:100] not in keys]
    print(f"release-notes-check: {version} (after {prev}): {sum(len(entries(b)) for _, b in picked)} entries in {', '.join(v for v, _ in picked)}; {marked} marked breaking, {len(missing)} missing from the notes' breaking section, {len(suspects)} suspects, {len(misfiled)} misfiled")
    if not m:
        print("  the notes have no breaking section (a heading with \"Breaking\")" + (" and none is needed" if not marked else ""))
    for v, first in missing:
        print(f"  MISSING ({v}): {first[:200]}")
    for v, why, first in suspects:
        print(f"  suspect, not marked breaking ({v}; {why}): {first[:200]}")
    for v, first in misfiled:
        print(f"  MISFILED (under {v}, not in {ptag}'s tagged CHANGELOG): {first[:200]}")
    if tag_note:
        print(tag_note)
    if missing or misfiled or (strict and suspects):
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
