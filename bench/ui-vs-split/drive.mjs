// Measures one running ui-todo site (std/ui with its wasm client, std/ui
// thin, or the split front end in examples/ui-todo/split) in headless
// Chrome at 390 by 844: bytes on the wire for a cold load, first
// contentful paint, and the time from submitting the Add form until the
// new todo is in the page, for ADDS adds 600 ms apart, with no added latency and with
// RTT ms added to every request (Network.emulateNetworkConditions). Also
// fetches / without a browser and reports whether the todos are in the
// HTML before any script runs. Prints one JSON object.
//
//   node drive.mjs http://127.0.0.1:8781/ [ADDS] [RTT]
import { spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
const BASE = process.argv[2];
const ADDS = Number(process.argv[3] || 30);
const RTT = Number(process.argv[4] || 100);
const PORT = 9300 + Math.floor(Math.random() * 600);
const prof = mkdtempSync(join(tmpdir(), "uivs-"));
const chrome = spawn("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
  ["--headless=new", "--disable-gpu", "--no-first-run", `--remote-debugging-port=${PORT}`, `--user-data-dir=${prof}`, "about:blank"], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function cdp(path, method = "GET") {
  for (let i = 0; i < 100; i++) {
    try { return await (await fetch(`http://127.0.0.1:${PORT}${path}`, { method })).json(); } catch { await sleep(100); }
  }
  throw new Error("no Chrome");
}
class Page {
  constructor(ws) {
    this.ws = ws; this.id = 0; this.wait = new Map(); this.bytes = new Map();
    ws.onmessage = (m) => {
      const d = JSON.parse(m.data);
      if (d.id && this.wait.has(d.id)) { this.wait.get(d.id)(d); this.wait.delete(d.id); }
      if (this.trace && d.method === "Network.requestWillBeSent") this.trace.push(["send", d.params.request.method, new URL(d.params.request.url).pathname + new URL(d.params.request.url).search, Math.round(d.params.timestamp * 1000)]);
      if (this.trace && d.method === "Network.loadingFinished") this.trace.push(["done", d.params.requestId, Math.round(d.params.timestamp * 1000)]);
      if (d.method === "Network.responseReceived") this.bytes.set(d.params.requestId, { url: d.params.response.url, bytes: 0 });
      if (d.method === "Network.loadingFinished" && this.bytes.has(d.params.requestId)) this.bytes.get(d.params.requestId).bytes = d.params.encodedDataLength;
    };
  }
  send(method, params = {}) { const id = ++this.id; this.ws.send(JSON.stringify({ id, method, params })); return new Promise((r) => this.wait.set(id, r)); }
  async eval(expr) {
    const r = await this.send("Runtime.evaluate", { expression: expr, awaitPromise: true, returnByValue: true });
    if (r.result.exceptionDetails) throw new Error(JSON.stringify(r.result.exceptionDetails));
    return r.result.result.value;
  }
}
async function open() {
  const t = await cdp("/json/new?about:blank", "PUT");
  const ws = new WebSocket(t.webSocketDebuggerUrl);
  await new Promise((r) => (ws.onopen = r));
  const p = new Page(ws);
  await p.send("Page.enable"); await p.send("Network.enable"); await p.send("Runtime.enable");
  await p.send("Network.setCacheDisabled", { cacheDisabled: true });
  await p.send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 1, mobile: true });
  return p;
}
const pct = (a, q) => { const s = [...a].sort((x, y) => x - y); return s.length ? Math.round(s[Math.min(s.length - 1, Math.floor(s.length * q))] * 10) / 10 : -1; };
// submits the Add form with title and resolves with the ms until an li
// holding it is in the document
const addOnce = (title) => `new Promise((done) => {
  const has = () => [...document.querySelectorAll("li span")].some((s) => s.textContent === ${JSON.stringify(title)});
  const t0 = performance.now();
  const ob = new MutationObserver(() => { if (has()) { ob.disconnect(); done(performance.now() - t0); } });
  ob.observe(document.body, { childList: true, subtree: true, characterData: true });
  const f = document.querySelector("input[name=title]").form;
  f.elements.title.value = ${JSON.stringify(title)};
  f.requestSubmit();
})`;
async function adds(p, tag) {
  const ms = [];
  for (let i = 0; i < ADDS; i++) {
    // each add starts with no request in flight, so it measures one add
    // alone rather than one queued behind the last add's follow-ups
    await sleep(600);
    ms.push(await p.eval(addOnce(`${tag} ${i}`)));
  }
  return { p50: pct(ms, 0.5), p95: pct(ms, 0.95), max: pct(ms, 1) };
}
const out = { url: BASE, adds: ADDS, rtt_ms: RTT };
try {
  const p = await open();
  await p.send("Page.navigate", { url: BASE });
  await p.eval(`new Promise((r) => { if (document.readyState === "complete") r(); else addEventListener("load", () => r()); })`);
  await sleep(1500);   // a wasm client compiles and the page settles
  const res = [...p.bytes.values()].filter((r) => r.url.startsWith("http"));
  out.cold_load = { requests: res.length, bytes: res.reduce((n, r) => n + r.bytes, 0), by_resource: res.map((r) => [new URL(r.url).pathname, r.bytes]) };
  out.fcp_ms = await p.eval(`Math.round((performance.getEntriesByName("first-contentful-paint")[0] || {startTime: -1}).startTime)`);
  out.add_local = await adds(p, "local");
  await p.send("Network.emulateNetworkConditions", { offline: false, latency: RTT, downloadThroughput: -1, uploadThroughput: -1 });
  out.add_rtt = await adds(p, "rtt");
  if (process.env.TRACE) {
    // the requests one more add makes, with ms since the first
    p.trace = [];
    await p.eval(addOnce("trace"));
    await sleep(500);
    const t0 = p.trace.length ? p.trace[0][p.trace[0].length - 1] : 0;
    out.trace = p.trace.map((e) => [...e.slice(0, -1), e[e.length - 1] - t0]);
  }
  const html = await (await fetch(BASE)).text();
  out.html_bytes = html.length;
  out.todos_in_html = html.includes("local 0");
} finally {
  chrome.kill();
  await sleep(300);
  rmSync(prof, { recursive: true, force: true });
}
console.log(JSON.stringify(out));
