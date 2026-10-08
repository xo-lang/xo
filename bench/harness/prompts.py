#!/usr/bin/env python3
"""Render benchmark prompts and estimate their token cost (standard library only).

  prompts.py list                         every prompt kind and task
  prompts.py render <kind> <task>         one rendered prompt on stdout
  prompts.py all --out DIR                every rendered prompt, DIR/<kind>/<task>.md
  prompts.py tokens [--max-runs N]        token estimate per prompt and per full run

<kind> is an arm (xo, go-plain, go-strict) or a planted bug author set
(go, xo-translate, xo-independent). <task> is '01' or '01-crud-api'.
"""
import argparse
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import common as c  # noqa: E402

# Where the existing Claude outputs live, to size model replies.
CLAUDE = {"go-plain": "bench/go/plain/{t}", "go-strict": "bench/go/strict/{t}",
          "xo": "bench/xo/v0.4/{t}"}
FEEDBACK_TOKENS = 1500  # typical failing loop output shown back to the model (measured on bench logs: 0.3k to 3k)


def kinds():
    out = [(a, t) for a in c.ARMS for t in c.TASKS]
    out += [(p, t) for p in c.PLANTED_SETS for t in c.PLANTED_TASKS]
    return out


def solution_dir(arm, t):
    d = os.path.join(c.ROOT, CLAUDE[arm].format(t=t))
    if arm == "xo" and not os.path.isdir(d):
        d = os.path.join(c.ROOT, "bench/xo/v0.6", t)
    return d


def reply_tokens(kind, t):
    """Size of a typical reply: the existing Claude output for this cell."""
    if kind in c.ARMS:
        d = solution_dir(kind, t)
        exts = (".go", ".xo", ".mod", "NOTES.md")
    else:
        sub = {"go": "planted", "xo-translate": "planted-xo", "xo-independent": "planted-xo2"}[kind]
        d = os.path.join(c.ROOT, "bench", sub, t)
        exts = (".go", ".xo", ".json")
    n = 0
    for dp, dns, fns in os.walk(d):
        if os.path.basename(dp) == "base":
            continue
        for f in fns:
            if f.endswith(exts):
                with open(os.path.join(dp, f), encoding="utf-8", errors="replace") as fh:
                    n += len(fh.read())
    return c.estimate_tokens("x" * n)


def cmd_tokens(max_runs):
    rows = []
    for kind, t in kinds():
        p = c.estimate_tokens(c.render_prompt(kind, t, max_runs))
        o = reply_tokens(kind, t)
        rows.append((kind, t, p, o))
    print("| Prompt | Task | Prompt tokens | Reply tokens (Claude output size) |")
    print("|---|---|---|---|")
    for kind, t, p, o in rows:
        print("| %s | %s | %d | %d |" % (kind, t, p, o))
    print()
    # One run: input = prompt, output = reply. A retry k (k >= 2) resends the
    # whole conversation: prompt + k-1 replies + k-1 feedback messages, and the
    # model answers with changed files (assume half a full reply).
    def cost(runs, p, o):
        inp = out = 0
        hist = p
        for k in range(1, runs + 1):
            inp += hist
            r = o if k == 1 else o // 2
            out += r
            hist += r + FEEDBACK_TOKENS
        return inp, out
    # Runs per cell as in the original Claude runs (score.PUBLISHED).
    import score
    idx = {"go-plain": 0, "go-strict": 1, "xo": 2}

    def claude_runs(kind, t):
        return score.PUBLISHED[t][idx[kind]] if kind in idx else 1

    print("| Part | Run 1 green: input | output | Claude's runs per cell: input | output | %d runs every cell: input | output |" % max_runs)
    print("|---|---|---|---|---|---|---|")
    total = [0] * 6
    for group in c.ARMS + c.PLANTED_SETS:
        g = [r for r in rows if r[0] == group]
        runs_max = max_runs if group in c.ARMS else 1
        vals = []
        for runs_of in (lambda k, t: 1, claude_runs, lambda k, t: runs_max):
            cs = [cost(runs_of(k, t), p, o) for k, t, p, o in g]
            vals += [sum(x[0] for x in cs), sum(x[1] for x in cs)]
        total = [x + y for x, y in zip(total, vals)]
        print("| %s (%d prompts) | %s |" % (group, len(g), " | ".join("%dk" % round(v / 1000) for v in vals)))
    print("| **full run** | %s |" % " | ".join("**%dk**" % round(v / 1000) for v in total))
    print()
    print("Tokens are estimated at 4 characters per token; reply sizes are the existing Claude outputs.")
    print("A retry resends the whole conversation plus about %d tokens of tool output and gets half a reply back." % FEEDBACK_TOKENS)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    sub.add_parser("list")
    r = sub.add_parser("render")
    r.add_argument("kind")
    r.add_argument("task")
    r.add_argument("--max-runs", type=int, default=c.DEFAULT_MAX_RUNS)
    a = sub.add_parser("all")
    a.add_argument("--out", required=True)
    a.add_argument("--max-runs", type=int, default=c.DEFAULT_MAX_RUNS)
    tk = sub.add_parser("tokens")
    tk.add_argument("--max-runs", type=int, default=c.DEFAULT_MAX_RUNS)
    args = ap.parse_args()
    if args.cmd == "list":
        for kind, t in kinds():
            print(kind, t)
    elif args.cmd == "render":
        t = c.task_name(args.task)
        sys.stdout.write(c.render_prompt(args.kind, t, args.max_runs))
    elif args.cmd == "all":
        for kind, t in kinds():
            p = os.path.join(args.out, kind, t + ".md")
            os.makedirs(os.path.dirname(p), exist_ok=True)
            with open(p, "w", encoding="utf-8") as f:
                f.write(c.render_prompt(kind, t, args.max_runs))
        print("wrote %d prompts under %s" % (len(kinds()), args.out))
    elif args.cmd == "tokens":
        cmd_tokens(args.max_runs)


if __name__ == "__main__":
    main()
