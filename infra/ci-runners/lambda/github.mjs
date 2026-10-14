// GitHub App access for the xo-ci Lambdas (decision 0225): the App's id,
// installation, and key come from SSM (/xo-ci/github-app/*).
import { createSign } from "node:crypto";
import { SSMClient, GetParameterCommand } from "@aws-sdk/client-ssm";

const ssm = new SSMClient({});
const params = {};
export const REPO = process.env.GITHUB_REPO; // owner/name

export async function param(name) {
  if (!(name in params)) {
    const r = await ssm.send(new GetParameterCommand({ Name: name, WithDecryption: true }));
    params[name] = r.Parameter.Value;
  }
  return params[name];
}

const b64url = (b) => Buffer.from(b).toString("base64url");

async function appJwt() {
  const now = Math.floor(Date.now() / 1000);
  const head = b64url(JSON.stringify({ alg: "RS256", typ: "JWT" }));
  const body = b64url(JSON.stringify({ iat: now - 60, exp: now + 540, iss: await param("/xo-ci/github-app/app-id") }));
  const sig = createSign("RSA-SHA256").update(head + "." + body).sign(await param("/xo-ci/github-app/private-key"));
  return head + "." + body + "." + b64url(sig);
}

let token = null;
async function installationToken() {
  if (token && Date.parse(token.expires_at) - Date.now() > 5 * 60 * 1000) return token.token;
  const iid = await param("/xo-ci/github-app/installation-id");
  token = await call("POST", `/app/installations/${iid}/access_tokens`, await appJwt());
  return token.token;
}

async function call(method, path, bearer) {
  const r = await fetch("https://api.github.com" + path, {
    method,
    headers: { Authorization: "Bearer " + bearer, Accept: "application/vnd.github+json", "User-Agent": "xo-ci" },
  });
  if (!r.ok) throw new Error(`${method} ${path}: ${r.status} ${await r.text()}`);
  return r.status === 204 ? null : r.json();
}

export async function gh(method, path) {
  return call(method, path, await installationToken());
}

// Every self-hosted runner of the repository: [{name, status, busy, labels: [names]}].
export async function runners() {
  const out = [];
  for (let page = 1; ; page++) {
    const r = await gh("GET", `/repos/${REPO}/actions/runners?per_page=100&page=${page}`);
    for (const x of r.runners) out.push({ id: x.id, name: x.name, status: x.status, busy: x.busy, labels: x.labels.map((l) => l.name) });
    if (r.runners.length < 100) return out;
  }
}
