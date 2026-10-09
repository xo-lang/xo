# Reconstruct Codex's failing first-run Xo snapshots from its final code and
# its own notes (runs/codex/xo/*/NOTES.md). Usage: reconstruct.py <codex xo dir> <out dir>
import sys, os, shutil

src, out = sys.argv[1], sys.argv[2]
shutil.rmtree(out, ignore_errors=True)
os.makedirs(out)


def snap(name, task, edits):
    d = os.path.join(out, name)
    shutil.copytree(os.path.join(src, task), d, ignore=shutil.ignore_patterns("runs.jsonl", "NOTES.md"))
    for rel, old, new, count in edits:
        p = os.path.join(d, rel)
        s = open(p).read()
        n = s.count(old)
        if n == 0 or (count and n != count):
            raise SystemExit(f"{name}: {rel}: found {n} of {old!r}")
        s = s.replace(old, new)
        open(p, "w").write(s)


# 04: "I initially used comma-separated match arms"
snap("04-file-sync", "04-file-sync", [
    ("main.xo", "    Linux => p\n", "    Linux => p,\n", 1),
    ("main.xo", "    Darwin => path.base(p) == \".DS_Store\"\n", "    Darwin => path.base(p) == \".DS_Store\",\n", 1),
    ("main.xo", "    Windows => path.base(p).to_lower() == \"thumbs.db\"\n", "    Windows => path.base(p).to_lower() == \"thumbs.db\",\n", 1),
])
# 05: "an unused std/task import and the required reason argument of Map.get_or_fault"
snap("05-rate-limiter", "05-rate-limiter", [
    ("main.xo", "use std/http\n", "use std/http\nuse std/task\n", 1),
    ("main.xo", "buckets.get_or_fault(key, \"key from snapshot\")", "buckets.get_or_fault(key)", 1),
])
# 06: "used var at a call site"
snap("06-job-queue", "06-job-queue", [
    ("main.xo", "    expire(d, now, q.max_attempts)", "    expire(var d, now, q.max_attempts)", 4),
])
# 07 run 1: "reached for Rust-style Hash + Eq bounds"
snap("07-cache-run1", "07-cache", [
    ("main.xo", "K: Hash, V", "K: Hash + Eq, V", 2),
])
# 07 run 2: "omitted effect argument stayed abstract Cache.fx"
snap("07-cache-run2", "07-cache", [
    ("main.xo", "fn routes(c: Cache[Int, Item, net.dial])", "fn routes(c: Cache[Int, Item])", 1),
])
# 08: "placed use after declarations; Xo requires imports first"
snap("08-notes-service", "08-notes-service", [
    ("api/api.xo", "use std/json\nstruct Page { notes: List[domain.Note], cursor: Str } derive Json\n",
     "struct Page { notes: List[domain.Note], cursor: Str } derive Json\nuse std/json\n", 1),
])
# 10: "assumed tuple JSON encoding and Duration.parse"
snap("10-log-pipeline", "10-log-pipeline", [
    ("main.xo", "json.decode_str[Duration](json.encode_str(args[i + 1]))?", "Duration.parse(args[i + 1]).or_err(\"invalid flush\")?", 1),
    ("main.xo", "top: List[Frequency] } derive Json", "top: List[(Str, Int)] } derive Json", 1),
    ("main.xo", ".map(fn(p) { Frequency{message: p.1.0, count: p.1.1} })}", ".map(fn(p) { p.1 })}", 1),
])
