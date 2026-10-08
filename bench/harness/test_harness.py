#!/usr/bin/env python3
"""Self test of the benchmark harness. Never calls an external endpoint.

The adapter test talks to a fake OpenAI compatible server on 127.0.0.1 that
returns canned replies. Run: python3 bench/harness/test_harness.py
(also run by `go test ./internal/benchharness`).
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import common as c  # noqa: E402
import score  # noqa: E402

GO_MOD = "module benchtask\n\ngo 1.26\n"
GO_MAIN = "package main\n\nfunc Add(a, b int) int { return a + b }\n\nfunc main() {}\n"
GO_TEST_BAD = ('package main\n\nimport "testing"\n\n'
               'func TestAdd(t *testing.T) {\n\tif Add(1, 2) != 4 {\n\t\tt.Fatal("want 4")\n\t}\n}\n')
GO_TEST_GOOD = GO_TEST_BAD.replace("!= 4", "!= 3").replace("want 4", "want 3")


def read_json(path):
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def read_jsonl(path):
    with open(path, encoding="utf-8") as f:
        return [json.loads(x) for x in f if x.strip()]


def block(path, body):
    return "=== FILE: %s ===\n%s=== END FILE ===\n" % (path, body)


def chat_response(text):
    return {"choices": [{"message": {"role": "assistant", "content": text}}],
            "usage": {"prompt_tokens": 100, "completion_tokens": 20, "total_tokens": 120}}


class FakeServer:
    """A local OpenAI compatible endpoint that pops canned replies in order."""

    def __init__(self, replies):
        self.replies = list(replies)
        self.requests = []
        outer = self

        class H(BaseHTTPRequestHandler):
            def do_POST(self):
                n = int(self.headers.get("Content-Length", "0"))
                outer.requests.append({"path": self.path, "auth": self.headers.get("Authorization"),
                                       "body": json.loads(self.rfile.read(n))})
                text = outer.replies.pop(0) if outer.replies else "no more canned replies"
                data = json.dumps(chat_response(text)).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def log_message(self, *a):
                pass

        self.httpd = ThreadingHTTPServer(("127.0.0.1", 0), H)
        self.url = "http://127.0.0.1:%d/v1" % self.httpd.server_address[1]
        self.thread = threading.Thread(target=self.httpd.serve_forever, daemon=True)

    def __enter__(self):
        self.thread.start()
        return self

    def __exit__(self, *a):
        self.httpd.shutdown()
        self.httpd.server_close()


def run_adapter(url, out, *args):
    env = dict(os.environ, OPENAI_BASE_URL=url, OPENAI_API_KEY="test-key", OPENAI_MODEL="fake-model")
    env.pop("OPENAI_EXTRA_BODY", None)
    p = subprocess.run([sys.executable, os.path.join(HERE, "run_openai_compatible.py"), "--out", out] + list(args),
                       capture_output=True, text=True, env=env, timeout=600)
    if p.returncode != 0:
        raise AssertionError("adapter failed:\n" + p.stdout + p.stderr)
    return p


class TestPrompts(unittest.TestCase):
    def test_every_prompt_renders(self):
        for kind in c.ARMS + c.PLANTED_SETS:
            tasks = c.TASKS if kind in c.ARMS else c.PLANTED_TASKS
            for t in tasks:
                text = c.render_prompt(kind, t, 7)
                self.assertNotIn("{{", text, (kind, t))
                self.assertIn("=== FILE:", text)
                self.assertIn("# Task %s" % t[:2], text)
                if kind in c.ARMS:
                    self.assertIn("At most 7 runs", text)
                if kind.startswith("xo"):
                    self.assertIn("# Xo Language Specification (Core)", text)
                    self.assertIn("# Xo Standard Library and Prelude", text)

    def test_parse_files(self):
        reply = ("intro\n" + block("a.go", "package a\n") + "between\n"
                 + block("sub/b.xo", "```xo\nfn f() {}\n```\n") + "outro")
        self.assertEqual(c.parse_files(reply), [("a.go", "package a\n"), ("sub/b.xo", "fn f() {}\n")])
        with self.assertRaises(ValueError):
            c.parse_files(block("../x.go", "x\n"))
        with self.assertRaises(ValueError):
            c.parse_files(block("/etc/x", "x\n"))

    def test_build_logs_match_published_runs(self):
        for t, pub in score.PUBLISHED.items():
            for i, rel in enumerate(["bench/go/plain/%s", "bench/go/strict/%s"]):
                h = score.parse_build_log(os.path.join(c.ROOT, rel % t, "BUILD_LOG.md"))
                self.assertEqual(h["runs_to_green"], pub[i], (rel % t))
            xd = os.path.join(c.ROOT, "bench/xo/v0.4", t)
            if not os.path.isdir(xd):
                xd = os.path.join(c.ROOT, "bench/xo/v0.6", t)
            h = score.parse_build_log(os.path.join(xd, "BUILD_LOG.md"))
            self.assertEqual(h["runs_to_green"], pub[2], xd)

    def test_line_and_test_counts_match_published(self):
        for t, pub in score.PUBLISHED.items():
            gd = os.path.join(c.ROOT, "bench/go/plain", t)
            xd = os.path.join(c.ROOT, "bench/xo/v0.4", t)
            if not os.path.isdir(xd):
                xd = os.path.join(c.ROOT, "bench/xo/v0.6", t)
            self.assertEqual(c.count_lines(gd, ".go"), pub[3], t)
            self.assertEqual(c.count_lines(xd, ".xo"), pub[4], t)
            if pub[5] is not None:
                self.assertEqual(c.count_tests(gd, ".go"), pub[5], t)
                self.assertEqual(c.count_tests(xd, ".xo"), pub[6], t)

    def test_strict_findings_count(self):
        out = ("== vet: ok\n== errcheck: FAIL\nmain_test.go:45:23:\tdefer resp.Body.Close()\n"
               "== nilaway: FAIL\n/x/main_test.go:47:29: Potential nil panic\n\t- http/client.go:648:12: trace\n"
               "== test: FAIL\nWARNING: DATA RACE\n")
        self.assertEqual(score.strict_findings(out), 3)


class TestAdapter(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp(prefix="xo-harness-test-")

    def tearDown(self):
        shutil.rmtree(self.tmp, ignore_errors=True)

    def test_solution_loop_retries_until_green(self):
        first = "Here it is.\n" + block("go.mod", GO_MOD) + block("main.go", GO_MAIN) + block("main_test.go", GO_TEST_BAD)
        second = "Fixed the test.\n" + block("main_test.go", GO_TEST_GOOD)
        out = os.path.join(self.tmp, "out")
        with FakeServer([first, second]) as srv:
            run_adapter(srv.url, out, "--arms", "go-plain", "--tasks", "01", "--planted", "none", "--max-runs", "3")
        self.assertEqual(len(srv.requests), 2)
        self.assertEqual(srv.requests[0]["path"], "/v1/chat/completions")
        self.assertEqual(srv.requests[0]["auth"], "Bearer test-key")
        self.assertEqual(srv.requests[0]["body"]["model"], "fake-model")
        msgs = srv.requests[1]["body"]["messages"]
        self.assertEqual([m["role"] for m in msgs], ["user", "assistant", "user"])
        self.assertIn("Run 1 of 3 is not green (failed at: test)", msgs[2]["content"])
        self.assertIn("want 4", msgs[2]["content"])
        d = os.path.join(out, "go-plain", "01-crud-api")
        runs = read_jsonl(os.path.join(d, "runs.jsonl"))
        self.assertEqual([r["green"] for r in runs], [False, True])
        self.assertTrue(os.path.isdir(os.path.join(d, ".attempts", "run-1")))
        self.assertEqual(read_json(os.path.join(d, "usage.json"))["requests"], 2)
        with open(os.path.join(out, "run.json")) as f:
            self.assertNotIn("test-key", f.read())
        # the scorer reads the history and reruns the final code
        rec = score.score_cell("go-plain", "01-crud-api", d, hidden=False)
        self.assertEqual(rec["history"]["runs_to_green"], 2)
        self.assertFalse(rec["history"]["first_green"])
        self.assertTrue(rec["final"]["green"])
        self.assertEqual(rec["tests"], 1)

    def test_stops_after_max_runs(self):
        bad = block("go.mod", GO_MOD) + block("main.go", "package main\n\nfunc main() { undefined() }\n")
        out = os.path.join(self.tmp, "out")
        with FakeServer([bad, bad]) as srv:
            run_adapter(srv.url, out, "--arms", "go-plain", "--tasks", "02", "--planted", "none", "--max-runs", "2")
        self.assertEqual(len(srv.requests), 2)
        runs = read_jsonl(os.path.join(out, "go-plain", "02-worker-pool", "runs.jsonl"))
        self.assertEqual([(r["green"], r["stage"]) for r in runs], [(False, "build"), (False, "build")])

    def test_planted_author_reply_is_written(self):
        man = json.dumps([{"id": "X1", "class": "ignored error", "status": "variant"},
                          {"id": "X2", "class": "nil", "status": "inexpressible", "reason": "no nil"}])
        reply = block("manifest.json", man + "\n") + block("variants/X1/store.xo", "fn x() {}\n")
        out = os.path.join(self.tmp, "out")
        with FakeServer(["no blocks at all", reply]) as srv:
            run_adapter(srv.url, out, "--arms", "none", "--tasks", "01", "--planted", "xo-independent")
        self.assertEqual(len(srv.requests), 2)  # one format retry
        d = os.path.join(out, "planted", "xo-independent", "01-crud-api")
        self.assertEqual(len(read_json(os.path.join(d, "manifest.json"))), 2)
        self.assertTrue(os.path.exists(os.path.join(d, "variants", "X1", "store.xo")))


if __name__ == "__main__":
    unittest.main(verbosity=1)
