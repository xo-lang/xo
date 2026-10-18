#!/usr/bin/env python3
"""Shrink an Xo program while two backends run it differently.

Usage:
    python3 -I scripts/reduce-diff.py [options] XO BACKEND_A BACKEND_B IN.xo [OUT.xo]

XO is an xo binary (go run ./scripts/xobuild prints one). A backend is
`go` (the default Go backend), or a native one (`arm64`, `llvm`), which
builds with --backend=NAME --no-gc. --crash keeps instead the programs
the first backend builds and the second fails to build with the same
compiler fault (its kind and location, crash_signature). IN.xo is a single file program (it
becomes main.xo of a module). The reducer keeps the smallest program
that still

  * builds with both backends,
  * exits with the same status on each backend as IN.xo did, and
  * prints something different: standard output, or standard error
    normalized like runProp in internal/gen (frames, crash bundle paths,
    and the deadlock's location and blocked list are dropped, decision
    0083).

It writes OUT.xo (default IN.min.xo) after every step and prints the
result. Passes: ddmin over lines (removes large chunks first), then
first improvement over single lines, whole `{ ... }` blocks, balanced
parentheses (replaced by 0, 1, or their contents) and integer literals,
repeated until nothing shrinks. Candidates run --jobs at a time; the
result does not depend on the job count.

Builds use a temporary XO_GOCACHE that is removed at the end, so the
first build compiles the runtime (tens of seconds).

Run with -I (isolated mode) so that no file beside the inputs is
imported. See knowledgebase/infrastructure/toolchain.md (reducer).
"""
import argparse
import concurrent.futures
import hashlib
import os
import re
import shutil
import stat
import subprocess
import sys
import tempfile


def norm_stderr(s):
    """The Python twin of normStderr in internal/gen/compileprop_test.go."""
    out = []
    frames = deadlock = blocked = False
    for line in s.splitlines(keepends=True):
        if deadlock and line == "blocked:\n":
            blocked = True
        if blocked and not line.startswith("crash bundle: "):
            continue
        if frames and line.startswith((" ", "\t")):
            continue
        if line.startswith("crash bundle: "):
            continue
        frames = line.startswith(("fault: ", "error: "))
        deadlock = line.startswith("fault: XO-F001")
        if deadlock and " at " in line:
            line = line[: line.index(" at ")] + "\n"
        out.append(line)
    return "".join(out)


def crash_signature(stderr):
    """A compiler fault without its operands: `fault: integer overflow:
    -(-9223372036854775808) does not fit Int at arm64/ranges.xo:294:44`
    is `integer overflow at arm64/ranges.xo:294:44`; other failures are
    their last line."""
    lines = stderr.strip().split("\n")
    for line in lines:
        m = re.match(r"fault: ([a-z ]+?)(:| [0-9-]).* at (\S+:\d+:\d+)$", line.strip())
        if m:
            return m.group(1) + " at " + m.group(3)
    return lines[-1] if lines else ""


class Reducer:
    def __init__(self, xo, a, b, work, timeout, jobs, log, crash=False):
        self.xo, self.backends, self.crash = xo, (a, b), crash
        self.work, self.timeout, self.jobs, self.log = work, timeout, jobs, log
        self.env = dict(os.environ, XO_GOCACHE=os.path.join(work, "gocache"),
                        XO_CRASH_DIR=os.path.join(work, "crash"))
        os.makedirs(self.env["XO_CRASH_DIR"], exist_ok=True)
        self.cache = {}
        self.want_exit = None  # the exit statuses of IN.xo
        self.tests = 0

    def build_run(self, d, backend):
        """(exit status, stdout, normalized stderr) or None if it does not build or hangs.
        With --crash, a build of the second backend that fails is
        ("crash", its fault signature, "")."""
        prog = os.path.join(d, "prog-" + backend)
        # the default backend is native (decision 0248): name go too
        args = [self.xo, "build", "--backend=" + backend]
        if backend != "go":
            args += ["--no-gc"]
        args += ["-o", prog, "."]
        try:
            p = subprocess.run(args, cwd=d, env=self.env, capture_output=True, text=True, timeout=600)
            if p.returncode != 0:
                if self.crash and backend == self.backends[1]:
                    return ("crash", crash_signature(p.stderr), "")
                return None
            if self.crash:
                return (0, "", "")
            p = subprocess.run([prog], cwd=d, env=self.env, capture_output=True, text=True,
                               timeout=self.timeout)
        except subprocess.TimeoutExpired:
            return None
        return p.returncode, p.stdout, norm_stderr(p.stderr)

    def results(self, src):
        key = hashlib.sha256(src.encode()).hexdigest()
        if key in self.cache:
            return self.cache[key]
        self.tests += 1
        d = tempfile.mkdtemp(dir=self.work, prefix="c")
        try:
            with open(os.path.join(d, "main.xo"), "w") as f:
                f.write(src)
            with open(os.path.join(d, "xo.mod"), "w") as f:
                f.write("module example.com/reduce\n")
            ra = self.build_run(d, self.backends[0])
            rb = self.build_run(d, self.backends[1]) if ra is not None else None
        finally:
            shutil.rmtree(d, ignore_errors=True)
        r = None if ra is None or rb is None else (ra, rb)
        self.cache[key] = r
        return r

    def interesting(self, src):
        r = self.results(src)
        if r is None:
            return False
        ra, rb = r
        if self.want_exit is not None and (ra[0], rb[0]) != self.want_exit:
            return False
        return ra != rb

    def first(self, cands):
        """The first interesting candidate in order, testing jobs at a time."""
        batch = []
        with concurrent.futures.ThreadPoolExecutor(self.jobs) as pool:
            for c in cands:
                batch.append(c)
                if len(batch) < self.jobs:
                    continue
                hit = self._batch(pool, batch)
                if hit is not None:
                    return hit
                batch = []
            if batch:
                return self._batch(pool, batch)
        return None

    def _batch(self, pool, batch):
        for c, ok in zip(batch, pool.map(self.interesting, batch)):
            if ok:
                return c
        return None


def ddmin_lines(red, src, save):
    """Delta debugging over lines: remove the largest chunks that keep the difference."""
    lines = src.split("\n")
    n = 2
    while len(lines) >= 2:
        size = -(-len(lines) // n)
        comps = []
        for i in range(0, len(lines), size):
            comps.append("\n".join(lines[:i] + lines[i + size:]))
        hit = red.first(comps)
        if hit is not None:
            lines = hit.split("\n")
            save(hit)
            n = max(n - 1, 2)
            continue
        if n >= len(lines):
            break
        n = min(n * 2, len(lines))
    return "\n".join(lines)


def parens(src):
    """Spans of balanced (...) groups, largest first."""
    out, stack = [], []
    for i, c in enumerate(src):
        if c == "(":
            stack.append(i)
        elif c == ")" and stack:
            out.append((stack.pop(), i + 1))
    return sorted(out, key=lambda s: s[0] - s[1])


def blocks(lines):
    """(first, last) line index pairs of `... {` lines and their closing `}` line."""
    out = []
    for i, l in enumerate(lines):
        if not l.rstrip().endswith("{"):
            continue
        ind = len(l) - len(l.lstrip())
        for j in range(i + 1, len(lines)):
            m = lines[j]
            if m.strip() and len(m) - len(m.lstrip()) == ind and m.strip().startswith("}"):
                out.append((i, j))
                break
    return sorted(out, key=lambda s: s[0] - s[1])


def candidates(src):
    lines = src.split("\n")
    for i, j in blocks(lines):
        yield "\n".join(lines[:i] + lines[j + 1:])
        # keep the body, drop the `{` and `}` lines
        yield "\n".join(lines[:i] + lines[i + 1:j] + lines[j + 1:])
    for i in range(len(lines)):
        yield "\n".join(lines[:i] + lines[i + 1:])
    for a, b in parens(src):
        whole, inner = src[a:b], src[a + 1:b - 1]
        for rep in ("0", "1", inner):
            if rep != whole:
                yield src[:a] + rep + src[b:]
    for m in re.finditer(r"-?\d+", src):
        if m.group() not in ("0", "1"):
            for rep in ("0", "1"):
                yield src[:m.start()] + rep + src[m.end():]


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0],
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("xo", help="the xo binary")
    ap.add_argument("a", help="first backend: go, arm64, or llvm")
    ap.add_argument("b", help="second backend")
    ap.add_argument("input", help="the program, a single .xo file")
    ap.add_argument("output", nargs="?", help="where to write the result (default IN.min.xo)")
    ap.add_argument("--jobs", type=int, default=4, help="candidates tested at once (default 4)")
    ap.add_argument("--timeout", type=float, default=30, help="seconds a run may take (default 30)")
    ap.add_argument("--no-ddmin", action="store_true", help="skip the ddmin pass")
    ap.add_argument("-q", "--quiet", action="store_true", help="no progress on stderr")
    ap.add_argument("--crash", action="store_true",
                    help="keep programs the first backend builds and the second fails to build with the same compiler fault")
    args = ap.parse_args()
    out = args.output or re.sub(r"(\.xo)?$", ".min.xo", args.input, count=1)
    xo = os.path.abspath(args.xo)
    with open(args.input) as f:
        src = f.read()

    work = tempfile.mkdtemp(prefix="xo-reduce-")
    try:
        log = (lambda *a: None) if args.quiet else (lambda *a: print(*a, file=sys.stderr, flush=True))
        red = Reducer(xo, args.a, args.b, work, args.timeout, max(1, args.jobs), log, args.crash)
        r = red.results(src)
        if r is None:
            sys.exit("reduce-diff: the input does not build with both backends (or a run timed out)")
        if args.crash and r[1][0] != "crash":
            sys.exit("reduce-diff: --crash: the input builds with %s" % args.b)
        if r[0] == r[1]:
            sys.exit("reduce-diff: the input runs alike on %s and %s" % (args.a, args.b))
        red.want_exit = (r[0][0], r[1][0])

        def save(s):
            with open(out, "w") as f:
                f.write(s)
            log("reduce-diff: %d bytes, %d lines (%d tests)" % (len(s), s.count("\n") + 1, red.tests))

        save(src)
        if not args.no_ddmin:
            src = ddmin_lines(red, src, save)
        while True:
            hit = red.first(c for c in candidates(src) if len(c) < len(src))
            if hit is None:
                break
            src = hit
            save(src)
        print(src, end="" if src.endswith("\n") else "\n")
        ra, rb = red.results(src)
        log("reduce-diff: done after %d tests\n--- %s: exit %s\n%s%s--- %s: exit %s\n%s%s" % (
            red.tests, args.a, ra[0], ra[1], ra[2], args.b, rb[0], rb[1], rb[2]))
    finally:
        # the Go build cache may hold read only directories
        for root, dirs, _ in os.walk(work):
            for d in dirs:
                p = os.path.join(root, d)
                os.chmod(p, os.stat(p).st_mode | stat.S_IWUSR | stat.S_IXUSR | stat.S_IRUSR)
        shutil.rmtree(work, ignore_errors=True)


if __name__ == "__main__":
    main()
