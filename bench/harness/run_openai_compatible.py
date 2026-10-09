#!/usr/bin/env python3
"""Run the agent benchmark against any OpenAI compatible chat endpoint (standard library only).

Environment (never put keys in files or on the command line):
  OPENAI_BASE_URL   endpoint root, for example https://api.openai.com/v1 or
                    https://generativelanguage.googleapis.com/v1beta/openai
                    (default https://api.openai.com/v1)
  OPENAI_API_KEY    bearer token (may be empty for a local server)
  OPENAI_MODEL      model name, for example gpt-5 or gemini-2.5-pro
  OPENAI_EXTRA_BODY optional JSON object merged into every request body,
                    for example {"reasoning_effort": "high"}

Usage:
  run_openai_compatible.py --out runs/gpt5 [--arms xo,go-plain,go-strict]
      [--tasks 01,09] [--planted go,xo-translate,xo-independent | --planted none]
      [--max-runs 5] [--temperature T] [--max-tokens N] [--resume] [--dry-run]

Protocol (matches the original runs, see README.md): the model gets one
prompt with the task, the required layout, and the output contract, and
writes the whole solution in its first reply. The harness writes the files,
runs the arm's feedback loop (xo: `xo check --fix --json`, which applies the
machine fixes, then `xo test --race`;
go-plain: build, vet, test; go-strict: strict-check.sh), and stops when it is
green. Otherwise it sends the tool output back and the model replies with
the files it changed, up to --max-runs runs in total. Planted bug authors get
one prompt and no tool output (one extra request only if the reply has no
valid manifest.json).

Writes <out>/<arm>/<task>/ (the solution, runs.jsonl, transcript.jsonl,
usage.json, .attempts/run-N/ snapshots) and <out>/planted/<set>/<task>/.
Then score with: score.py solutions <out>; score.py planted <out>.
"""
import argparse
import json
import os
import shutil
import sys
import time
import urllib.error
import urllib.request

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import common as c  # noqa: E402

FEEDBACK_LIMIT = 12000  # characters of tool output sent back per run


class Client:
    def __init__(self, base, key, model, temperature=None, max_tokens=None, timeout=900, extra=None):
        self.url = base.rstrip("/") + "/chat/completions"
        self.key, self.model = key, model
        self.temperature, self.max_tokens, self.timeout = temperature, max_tokens, timeout
        self.extra = extra or {}

    def chat(self, messages):
        body = {"model": self.model, "messages": messages}
        if self.temperature is not None:
            body["temperature"] = self.temperature
        if self.max_tokens:
            body["max_tokens"] = self.max_tokens
        body.update(self.extra)
        data = json.dumps(body).encode()
        delay = 5
        for attempt in range(5):
            req = urllib.request.Request(self.url, data=data, method="POST")
            req.add_header("Content-Type", "application/json")
            if self.key:
                req.add_header("Authorization", "Bearer " + self.key)
            try:
                with urllib.request.urlopen(req, timeout=self.timeout) as r:
                    resp = json.loads(r.read())
                msg = resp["choices"][0]["message"]
                return msg.get("content") or "", resp.get("usage") or {}
            except urllib.error.HTTPError as e:
                detail = e.read()[:500].decode("utf-8", "replace")
                if e.code in (408, 409, 429) or e.code >= 500:
                    print("  HTTP %d, retrying in %ds: %s" % (e.code, delay, detail), file=sys.stderr)
                else:
                    raise SystemExit("request failed: HTTP %d: %s" % (e.code, detail))
            except (urllib.error.URLError, TimeoutError, ConnectionError) as e:
                print("  %s, retrying in %ds" % (e, delay), file=sys.stderr)
            if attempt < 4:
                time.sleep(delay)
                delay *= 3
        raise SystemExit("request failed after 5 attempts")


def tail(text, limit=FEEDBACK_LIMIT):
    if len(text) <= limit:
        return text
    half = limit // 2
    return text[:half] + "\n[... %d characters cut ...]\n" % (len(text) - limit) + text[-half:]


def append_jsonl(path, rec):
    with open(path, "a", encoding="utf-8") as f:
        f.write(json.dumps(rec) + "\n")


def snapshot(d, run):
    dst = os.path.join(d, ".attempts", "run-%d" % run)
    shutil.rmtree(dst, ignore_errors=True)
    c.copy_solution(d, dst)


def add_usage(total, usage):
    for k in ("prompt_tokens", "completion_tokens", "total_tokens"):
        total[k] = total.get(k, 0) + int(usage.get(k) or 0)
    total["requests"] = total.get("requests", 0) + 1


def run_solution(client, arm, task, out, max_runs, resume):
    d = os.path.join(out, arm, task)
    runs_path = os.path.join(d, "runs.jsonl")
    if os.path.exists(runs_path):
        if resume:
            print("skip %s %s (already run)" % (arm, task), file=sys.stderr)
            return
        shutil.rmtree(d)
    os.makedirs(d, exist_ok=True)
    messages = [{"role": "user", "content": c.render_prompt(arm, task, max_runs)}]
    usage = {}
    for run in range(1, max_runs + 1):
        print("%s %s run %d" % (arm, task, run), file=sys.stderr, flush=True)
        reply, u = client.chat(messages)
        add_usage(usage, u)
        append_jsonl(os.path.join(d, "transcript.jsonl"), {"run": run, "reply": reply, "usage": u})
        messages.append({"role": "assistant", "content": reply})
        rec = {"run": run, "prompt_tokens": u.get("prompt_tokens"), "completion_tokens": u.get("completion_tokens")}
        try:
            files = c.parse_files(reply)
        except ValueError as e:
            files, rec["format_error"] = [], str(e)
        rec["files"] = [p for p, _ in files]
        if run == 1 and not files:
            fb = {"green": False, "stage": "format", "output": rec.get("format_error") or
                  "No file blocks found. Use the === FILE: path === ... === END FILE === format."}
        else:
            c.write_files(d, files)
            snapshot(d, run)
            fb = c.feedback(arm, d, agent=True)
        rec.update({k: v for k, v in fb.items() if k not in ("output", "arm")})
        append_jsonl(runs_path, rec)
        print("  green=%s stage=%s" % (fb["green"], fb.get("stage")), file=sys.stderr, flush=True)
        if fb["green"]:
            break
        if run < max_runs:
            messages.append({"role": "user", "content": (
                "Run %d of %d is not green (failed at: %s). Tool output:\n\n%s\n\n"
                "Reply with the full content of every file you change, in the same block format."
                % (run, max_runs, fb.get("stage"), tail(fb["output"])))})
    with open(os.path.join(d, "usage.json"), "w", encoding="utf-8") as f:
        json.dump(usage, f, indent=2)


def run_planted(client, set_name, task, out, resume):
    d = os.path.join(out, "planted", set_name, task)
    if os.path.exists(os.path.join(d, "manifest.json")) and resume:
        print("skip planted %s %s" % (set_name, task), file=sys.stderr)
        return
    shutil.rmtree(d, ignore_errors=True)
    os.makedirs(d)
    messages = [{"role": "user", "content": c.render_prompt(set_name, task)}]
    usage = {}
    for attempt in (1, 2):
        print("planted %s %s request %d" % (set_name, task, attempt), file=sys.stderr, flush=True)
        reply, u = client.chat(messages)
        add_usage(usage, u)
        append_jsonl(os.path.join(d, "transcript.jsonl"), {"request": attempt, "reply": reply, "usage": u})
        try:
            files = c.parse_files(reply)
            man = dict(files).get("manifest.json")
            if man is None:
                raise ValueError("no manifest.json block")
            json.loads(man)
            bad = [p for p, _ in files if p != "manifest.json" and not p.startswith("variants" + os.sep)]
            if bad:
                raise ValueError("files outside variants/: %s" % ", ".join(bad))
            c.write_files(d, files)
            break
        except ValueError as e:
            if attempt == 2:
                print("  no valid reply: %s" % e, file=sys.stderr)
                break
            messages += [{"role": "assistant", "content": reply},
                         {"role": "user", "content": "The reply could not be used (%s). Send the whole answer "
                                                     "again in the required block format." % e}]
    with open(os.path.join(d, "usage.json"), "w", encoding="utf-8") as f:
        json.dump(usage, f, indent=2)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--out", required=True)
    ap.add_argument("--arms", default=",".join(c.ARMS), help="comma separated, or 'none'")
    ap.add_argument("--tasks", default="", help="comma separated task numbers, default all")
    ap.add_argument("--planted", default=",".join(c.PLANTED_SETS), help="comma separated sets, or 'none'")
    ap.add_argument("--max-runs", type=int, default=c.DEFAULT_MAX_RUNS)
    ap.add_argument("--temperature", type=float)
    ap.add_argument("--max-tokens", type=int)
    ap.add_argument("--timeout", type=int, default=900, help="seconds per request")
    ap.add_argument("--resume", action="store_true", help="skip cells that already have results")
    ap.add_argument("--dry-run", action="store_true", help="only render prompts into --out/prompts")
    args = ap.parse_args()
    arms = [] if args.arms == "none" else [a for a in args.arms.split(",") if a]
    sets = [] if args.planted == "none" else [s for s in args.planted.split(",") if s]
    for a in arms:
        if a not in c.ARMS:
            ap.error("unknown arm %r" % a)
    for s in sets:
        if s not in c.PLANTED_SETS:
            ap.error("unknown planted set %r" % s)
    tasks = [c.task_name(t) for t in args.tasks.split(",")] if args.tasks else list(c.TASKS)
    if args.dry_run:
        for kind in arms + sets:
            for t in tasks if kind in c.ARMS else [t for t in tasks if t in c.PLANTED_TASKS]:
                p = os.path.join(args.out, "prompts", kind, t + ".md")
                os.makedirs(os.path.dirname(p), exist_ok=True)
                with open(p, "w", encoding="utf-8") as f:
                    f.write(c.render_prompt(kind, t, args.max_runs))
        print("prompts written under %s/prompts" % args.out)
        return
    model = os.environ.get("OPENAI_MODEL")
    if not model:
        ap.error("set OPENAI_MODEL (and OPENAI_API_KEY, OPENAI_BASE_URL)")
    extra = json.loads(os.environ["OPENAI_EXTRA_BODY"]) if os.environ.get("OPENAI_EXTRA_BODY") else None
    client = Client(os.environ.get("OPENAI_BASE_URL", "https://api.openai.com/v1"),
                    os.environ.get("OPENAI_API_KEY", ""), model, args.temperature, args.max_tokens,
                    args.timeout, extra)
    os.makedirs(args.out, exist_ok=True)
    with open(os.path.join(args.out, "run.json"), "w", encoding="utf-8") as f:
        json.dump({"model": model, "base_url": os.environ.get("OPENAI_BASE_URL", "https://api.openai.com/v1"),
                   "max_runs": args.max_runs, "temperature": args.temperature, "extra_body": extra,
                   "started": time.strftime("%Y-%m-%dT%H:%M:%S")}, f, indent=2)
    for arm in arms:
        for t in tasks:
            run_solution(client, arm, t, args.out, args.max_runs, args.resume)
    for s in sets:
        for t in [t for t in tasks if t in c.PLANTED_TASKS]:
            run_planted(client, s, t, args.out, args.resume)


if __name__ == "__main__":
    main()
