"""Shared pieces of the agent benchmark harness (standard library only).

Prompt rendering, the per arm feedback loops, tool discovery, and the file
block format that models use to return code. Used by prompts.py, score.py,
and run_openai_compatible.py.
"""
import json
import os
import re
import shutil
import subprocess
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", ".."))
BENCH = os.path.join(ROOT, "bench")
PROMPTS = os.path.join(HERE, "prompts")

ARMS = ["xo", "go-plain", "go-strict"]
TASKS = sorted(f[:-3] for f in os.listdir(os.path.join(BENCH, "tasks")) if f.endswith(".md"))
PLANTED_TASKS = ["01-crud-api", "02-worker-pool", "03-retry-client", "04-file-sync", "05-rate-limiter"]
PLANTED_SETS = ["go", "xo-translate", "xo-independent"]

# Timeouts in seconds. Generous: they only stop hung programs.
T_BUILD = 600
T_TEST = 900


def task_name(arg):
    """Accepts '01', '1', or '01-crud-api'; returns the full task name."""
    for t in TASKS:
        if arg == t or t.split("-")[0] == arg.zfill(2):
            return t
    raise SystemExit("unknown task %r (have %s)" % (arg, ", ".join(TASKS)))


# ---------------------------------------------------------------- prompts

INCLUDE = re.compile(r"^\{\{include ([^}]+)\}\}\s*$", re.M)
FILES = re.compile(r"^\{\{files ([^}]+)\}\}\s*$", re.M)


def _source_block(path, rel):
    with open(path, encoding="utf-8") as f:
        body = f.read()
    if not body.endswith("\n"):
        body += "\n"
    return "=== FILE: %s ===\n%s=== END FILE ===\n" % (rel, body)


def _files_of(dir_rel):
    """Every source file of a directory as file blocks (paths relative to it)."""
    base = os.path.join(ROOT, dir_rel)
    out = []
    for dp, dns, fns in os.walk(base):
        dns[:] = sorted(d for d in dns if not d.startswith("."))
        for fn in sorted(fns):
            if fn.endswith((".go", ".xo", ".mod")) or fn in ("xo.mod", "go.mod"):
                p = os.path.join(dp, fn)
                out.append(_source_block(p, os.path.relpath(p, base)))
    if not out:
        raise SystemExit("no source files under %s" % dir_rel)
    return "\n".join(out)


def render_text(text, depth=0):
    """Expands {{include repo/relative/path}} and {{files repo/relative/dir}}."""
    if depth > 5:
        raise SystemExit("include nesting too deep")

    def inc(m):
        with open(os.path.join(ROOT, m.group(1).strip()), encoding="utf-8") as f:
            return render_text(f.read(), depth + 1).rstrip("\n") + "\n"

    text = INCLUDE.sub(inc, text)
    return FILES.sub(lambda m: _files_of(m.group(1).strip()), text)


def prompt_path(kind, task):
    """kind is an arm (xo, go-plain, go-strict) or a planted set."""
    sub = kind if kind in ARMS else os.path.join("planted", kind)
    return os.path.join(PROMPTS, sub, task + ".md")


DEFAULT_MAX_RUNS = 5


def render_prompt(kind, task, max_runs=DEFAULT_MAX_RUNS):
    with open(prompt_path(kind, task), encoding="utf-8") as f:
        text = f.read()
    # drop the maintainer comment at the top of each prompt file
    text = re.sub(r"\A<!--.*?-->\s*", "", text, flags=re.S)
    return render_text(text).replace("{{max_runs}}", str(max_runs))


def estimate_tokens(text):
    """About 4 characters per token for English, code, and Markdown.

    Real tokenizers differ by model (GPT and Gemini count 10 to 20 percent
    apart on code); this is for budgeting, not billing.
    """
    return (len(text) + 3) // 4


# ---------------------------------------------------------------- file blocks

BLOCK = re.compile(r"^=== FILE: *(.+?) *===[ \t]*\n(.*?)^=== END FILE ===[ \t]*$", re.M | re.S)


def parse_files(reply):
    """Returns [(relative path, content)] from a model reply.

    Raises ValueError on an unsafe path. A Markdown code fence wrapped around
    the content inside a block is removed.
    """
    files = []
    for m in BLOCK.finditer(reply):
        rel = m.group(1).strip().strip("`")
        body = m.group(2)
        lines = body.split("\n")
        if lines and lines[0].startswith("```"):
            lines = lines[1:]
            while lines and lines[-1].strip() == "":
                lines.pop()
            if lines and lines[-1].strip() == "```":
                lines = lines[:-1]
            body = "\n".join(lines) + "\n"
        norm = os.path.normpath(rel)
        if os.path.isabs(rel) or norm.startswith("..") or norm in (".", ""):
            raise ValueError("unsafe path in reply: %r" % rel)
        files.append((norm, body))
    return files


def write_files(dst, files):
    for rel, body in files:
        p = os.path.join(dst, rel)
        os.makedirs(os.path.dirname(p), exist_ok=True)
        with open(p, "w", encoding="utf-8") as f:
            f.write(body)


# ---------------------------------------------------------------- tools

def run(cmd, cwd=None, timeout=T_BUILD, env=None):
    """Runs a command; returns (exit code, combined output). 124 on timeout."""
    e = dict(os.environ)
    if env:
        e.update(env)
    try:
        p = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout, env=e)
        return p.returncode, p.stdout + p.stderr
    except subprocess.TimeoutExpired as ex:
        out = (ex.stdout or b"") + (ex.stderr or b"")
        if isinstance(out, bytes):
            out = out.decode("utf-8", "replace")
        return 124, out + "\n[harness] timed out after %ds\n" % timeout


def tools_bin():
    """Directory with staticcheck, errcheck, nilaway, exhaustive.

    $XO_TOOLS_BIN, else <repo>/.tools/bin, else the main checkout's
    .tools/bin when this is a git worktree.
    """
    cands = []
    if os.environ.get("XO_TOOLS_BIN"):
        cands.append(os.environ["XO_TOOLS_BIN"])
    cands.append(os.path.join(ROOT, ".tools", "bin"))
    rc, out = run(["git", "rev-parse", "--git-common-dir"], cwd=ROOT, timeout=30)
    if rc == 0:
        common = out.strip().splitlines()[-1]
        if not os.path.isabs(common):
            common = os.path.join(ROOT, common)
        cands.append(os.path.join(os.path.dirname(os.path.abspath(common)), ".tools", "bin"))
    for c in cands:
        if os.path.exists(os.path.join(c, "staticcheck")):
            return c
    return cands[0]


_xo = None


def xo_bin():
    """The xo binary: $XO_BIN, else built once from this checkout (current toolchain)."""
    global _xo
    if _xo:
        return _xo
    if os.environ.get("XO_BIN"):
        _xo = os.environ["XO_BIN"]
        return _xo
    cache = os.environ.get("XO_HARNESS_CACHE") or os.path.join(tempfile.gettempdir(), "xo-harness")
    os.makedirs(cache, exist_ok=True)
    out = os.path.join(cache, "xo")
    rc, txt = run(["go", "build", "-o", out, "./cmd/xo"], cwd=ROOT)
    if rc != 0:
        raise SystemExit("building xo failed:\n" + txt)
    _xo = out
    return _xo


def copy_solution(src, dst):
    """Copies a solution, skipping logs, notes, and harness bookkeeping."""
    def ignore(d, names):
        return [n for n in names if n.startswith(".") or n in ("BUILD_LOG.md", "runs.jsonl", "transcript.jsonl", "usage.json")]
    shutil.copytree(src, dst, ignore=ignore, dirs_exist_ok=True)


# ---------------------------------------------------------------- feedback loops

def xo_check(d):
    """Returns (errors, warnings, raw json text). Diagnostics are dicts."""
    rc, out = run([xo_bin(), "check", "--json", d], cwd=d)
    start = out.find("{")
    try:
        data = json.loads(out[start:]) if start >= 0 else {}
    except ValueError:
        data = {}
    diags = data.get("diagnostics") or []
    errs = [x for x in diags if x.get("severity") == "error"]
    warns = [x for x in diags if x.get("severity") == "warning"]
    if rc != 0 and not errs:
        errs = [{"code": "HARNESS", "severity": "error", "message": out.strip()[:2000]}]
    return errs, warns, out.replace(d + os.sep, "").replace(d, ".")


def xo_test(d):
    rc, out = run([xo_bin(), "test", "--race", d], cwd=d, timeout=T_TEST)
    return rc, out.replace(d + os.sep, "").replace(d, ".")


def loop_xo(d):
    errs, warns, out = xo_check(d)
    res = {"arm": "xo", "errors": len(errs), "warnings": len(warns),
           "codes": sorted({x.get("code", "?") for x in errs + warns})}
    if errs:
        res.update(green=False, stage="check", output="$ xo check --json .\n" + out)
        return res
    rc, tout = xo_test(d)
    m = re.search(r"(\d+) passed, (\d+) failed", tout)
    if m:
        res["tests_passed"], res["tests_failed"] = int(m.group(1)), int(m.group(2))
    res.update(green=(rc == 0), stage="test" if rc else "none",
               output="$ xo check --json .\n" + out + "\n$ xo test --race .\n" + tout)
    return res


def loop_go_plain(d):
    log = []
    for stage, cmd in [("build", ["go", "build", "./..."]), ("vet", ["go", "vet", "./..."]),
                       ("test", ["go", "test", "-count=1", "-timeout", "%ds" % T_TEST, "./..."])]:
        rc, out = run(cmd, cwd=d, timeout=T_TEST + 60)
        log.append("$ " + " ".join(cmd) + "\n" + out.replace(d + os.sep, ""))
        if rc != 0:
            return {"arm": "go-plain", "green": False, "stage": stage, "output": "\n".join(log)}
    return {"arm": "go-plain", "green": True, "stage": "none", "output": "\n".join(log)}


def loop_go_strict(d):
    script = os.path.join(BENCH, "go", "strict-check.sh")
    rc, out = run(["sh", script, d], cwd=d, timeout=2 * T_TEST, env={"XO_TOOLS_BIN": tools_bin()})
    failed = re.findall(r"^== (\w+): FAIL", out, re.M)
    return {"arm": "go-strict", "green": rc == 0, "stage": ",".join(failed) or ("none" if rc == 0 else "error"),
            "output": "$ strict-check.sh .\n" + out.replace(d + os.sep, "")}


LOOPS = {"xo": loop_xo, "go-plain": loop_go_plain, "go-strict": loop_go_strict}


def feedback(arm, d):
    """Runs the arm's feedback loop on directory d (in place)."""
    return LOOPS[arm](d)


# ---------------------------------------------------------------- measures

def source_files(d, ext):
    out = []
    for dp, dns, fns in os.walk(d):
        dns[:] = [x for x in dns if not x.startswith(".") and x != "testdata"]
        out += [os.path.join(dp, f) for f in fns if f.endswith(ext)]
    return sorted(out)


def count_lines(d, ext):
    n = 0
    for p in source_files(d, ext):
        with open(p, encoding="utf-8", errors="replace") as f:
            n += sum(1 for _ in f)
    return n


def count_tests(d, ext):
    pat = re.compile(r"^func Test\w*\(" if ext == ".go" else r"^\s*test\s+\"", re.M)
    n = 0
    for p in source_files(d, ext):
        if not p.endswith("_test" + ext):  # the published counts use test files only
            continue
        with open(p, encoding="utf-8", errors="replace") as f:
            n += len(pat.findall(f.read()))
    return n
