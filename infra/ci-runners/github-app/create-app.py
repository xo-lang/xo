#!/usr/bin/env python3
"""Create the xo-ci GitHub App through the manifest flow (decision 0225).

Serves a page on http://localhost:8975 that posts the app manifest to
GitHub. After the owner confirms in the browser, GitHub redirects back
with a code, which this script exchanges for the app's id and private
key. Both go to SSM Parameter Store (the key as a SecureString); the key
is never printed or kept on disk. The owner then installs the app on the
repository; GitHub redirects to /installed with the installation id,
which the script checks with a signed request and stores too.

Run: python3 -I infra/ci-runners/github-app/create-app.py
Then open http://localhost:8975 in a browser signed in to GitHub.
"""

import base64
import html
import http.server
import json
import os
import secrets
import subprocess
import sys
import tempfile
import time
import urllib.parse
import urllib.request

PORT = 8975
REPO = "josephspurrier/xo"
REGION = "us-east-1"
PREFIX = "/xo-ci/github-app"
STATE = secrets.token_hex(16)

MANIFEST = {
    "name": "xo-ci-runners-josephspurrier",
    "url": "https://github.com/" + REPO,
    "description": "Registers the Xo self-hosted CI runners (decision 0225).",
    "public": False,
    "hook_attributes": {"url": "https://example.invalid/unused", "active": False},
    "redirect_url": "http://localhost:%d/callback" % PORT,
    "setup_url": "http://localhost:%d/installed" % PORT,
    "default_permissions": {
        "administration": "write",  # runner registration tokens, runner list and removal
        "actions": "read",  # workflow jobs, for the autostart Lambda
        "repository_hooks": "write",  # the workflow_job webhook (Terraform github provider)
        "metadata": "read",
    },
    "default_events": [],
}

app = {}


def ssm_put(name, value, secure=False):
    with tempfile.NamedTemporaryFile("w", delete=False) as f:
        os.chmod(f.name, 0o600)
        f.write(value)
    try:
        cmd = ["aws", "ssm", "put-parameter", "--region", REGION, "--overwrite",
               "--name", PREFIX + "/" + name, "--value", "file://" + f.name,
               "--type", "SecureString" if secure else "String"]
        subprocess.run(cmd, check=True, stdout=subprocess.DEVNULL)
    finally:
        os.remove(f.name)


def b64url(b):
    return base64.urlsafe_b64encode(b).rstrip(b"=").decode()


def app_jwt():
    now = int(time.time())
    head = b64url(json.dumps({"alg": "RS256", "typ": "JWT"}).encode())
    body = b64url(json.dumps({"iat": now - 60, "exp": now + 540, "iss": str(app["id"])}).encode())
    signing = (head + "." + body).encode()
    with tempfile.NamedTemporaryFile("w", delete=False) as f:
        os.chmod(f.name, 0o600)
        f.write(app["pem"])
    try:
        sig = subprocess.run(["openssl", "dgst", "-sha256", "-sign", f.name],
                             input=signing, capture_output=True, check=True).stdout
    finally:
        os.remove(f.name)
    return head + "." + body + "." + b64url(sig)


def gh(method, path, token=None, bearer=None):
    req = urllib.request.Request("https://api.github.com" + path, method=method)
    req.add_header("Accept", "application/vnd.github+json")
    if bearer:
        req.add_header("Authorization", "Bearer " + bearer)
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.load(r)


class Handler(http.server.BaseHTTPRequestHandler):
    def reply(self, body):
        data = ("<!doctype html><meta charset=utf-8><body style='font:16px sans-serif;margin:2em'>" + body).encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, *a):
        pass

    def do_GET(self):
        u = urllib.parse.urlparse(self.path)
        q = urllib.parse.parse_qs(u.query)
        if u.path == "/":
            self.reply(
                "<form id=f method=post action='https://github.com/settings/apps/new?state=%s'>"
                "<input type=hidden name=manifest value='%s'>"
                "<button>Create the xo-ci GitHub App</button></form>"
                "<script>document.getElementById('f').submit()</script>"
                % (STATE, html.escape(json.dumps(MANIFEST), quote=True)))
        elif u.path == "/callback":
            if q.get("state", [""])[0] != STATE:
                self.reply("State mismatch; start again at http://localhost:%d" % PORT)
                return
            code = q["code"][0]
            r = gh("POST", "/app-manifests/%s/conversions" % urllib.parse.quote(code))
            app.update(id=r["id"], slug=r["slug"], pem=r["pem"])
            ssm_put("app-id", str(r["id"]))
            ssm_put("slug", r["slug"])
            ssm_put("private-key", r["pem"], secure=True)
            print("app created: id %s, slug %s; id and key stored in SSM %s" % (r["id"], r["slug"], PREFIX), flush=True)
            link = "https://github.com/apps/%s/installations/new" % r["slug"]
            self.reply("App <b>%s</b> created and its key stored in AWS. "
                       "Now install it on <b>%s</b> only: <a href='%s'>%s</a>" % (r["slug"], REPO, link, link))
        elif u.path == "/installed":
            iid = q.get("installation_id", [""])[0]
            if not app or not iid.isdigit():
                self.reply("Unexpected request.")
                return
            inst = gh("GET", "/app/installations/" + iid, bearer=app_jwt())
            ssm_put("installation-id", iid)
            print("installed: installation %s on %s (%s); stored in SSM" %
                  (iid, inst["account"]["login"], inst["repository_selection"]), flush=True)
            self.reply("Installed (installation %s). You can close this tab." % iid)
            app["done"] = True
        else:
            self.send_response(404)
            self.end_headers()


def main():
    srv = http.server.HTTPServer(("127.0.0.1", PORT), Handler)
    print("open http://localhost:%d in a browser signed in to GitHub" % PORT, flush=True)
    deadline = time.time() + 1800
    while not app.get("done") and time.time() < deadline:
        srv.timeout = 5
        srv.handle_request()
    if not app.get("done"):
        sys.exit("timed out after 30 minutes")


if __name__ == "__main__":
    main()
