// Runs std/ui's browser runtime (RUNTIME_JS, path in argv[2]) as several
// tabs of one browser: one shared localStorage, a sessionStorage each, and
// a fake sync server that keeps one seq per client and drops a Msg whose
// seq it has seen, as std/ui's Server.receive does. Prints a JSON report.
import { readFileSync } from "node:fs";
import vm from "node:vm";

const src = readFileSync(process.argv[2], "utf8");
const scenario = process.argv[3];

function storage() {
  const m = new Map();
  return {
    get length() { return m.size; },
    key(i) { return [...m.keys()][i] ?? null; },
    getItem(k) { return m.has(k) ? m.get(k) : null; },
    setItem(k, v) { m.set(k, String(v)); },
    removeItem(k) { m.delete(k); },
  };
}
const local = storage();
const server = { acked: new Map(), applied: [], down: false, status: 200, reloads: 0 };
function receive(up) {
  let a = server.acked.get(up.client) ?? 0;
  for (const e of up.msgs) {
    if (e.seq <= a) continue;
    a = e.seq;
    server.applied.push(e.msg);
  }
  server.acked.set(up.client, a);
  return { down: { acked: a, reset: null, entries: [] }, ver: server.applied.length, html: "<p>" + server.applied.length + "</p>" };
}

function tab() {
  const handlers = {};
  const attrs = {};
  const root = {
    innerHTML: "",
    getAttribute: (k) => (k === "data-xo-sync" ? "/_xo/sync" : null),
    addEventListener: (n, f) => { handlers[n] = f; },
    contains: () => true,
  };
  const stEl = { textContent: JSON.stringify({ ver: 0, model: {} }) };
  const listeners = {};
  const docHandlers = {};
  const timers = [];
  const ctx = {
    document: {
      getElementById: (id) => (id === "xo-ui" ? root : id === "xo-state" ? stEl : null),
      documentElement: { setAttribute: (k, v) => { attrs[k] = v; }, removeAttribute: (k) => { delete attrs[k]; }, hasAttribute: (k) => k in attrs },
      addEventListener: (n, f) => { docHandlers[n] = f; },
      hidden: false,
      activeElement: null,
    },
    window: { addEventListener: (n, f) => { listeners[n] = f; } },
    location: { pathname: "/", protocol: "http:", host: "test", reload: () => { server.reloads++; } },
    localStorage: local,
    sessionStorage: storage(),
    fetch: async (url, init) => {
      if (server.down) throw new Error("offline");
      if (server.status !== 200) return { ok: false, status: server.status, json: async () => ({}) };
      const body = receive(JSON.parse(init.body));
      return { ok: true, status: 200, json: async () => body };
    },
    setInterval: () => 0,
    setTimeout: (f) => { timers.push(f); return timers.length; },
    JSON, Math, Date, Error, Promise, Array, Object, String,
  };
  vm.runInNewContext(src, ctx);
  return {
    click(msg) {
      const t = { getAttribute: () => JSON.stringify(msg), closest() { return t; }, name: "", form: null };
      handlers.click({ target: t, preventDefault() {} });
    },
    pagehide() { listeners.pagehide?.(); },
    pointer(kind) { docHandlers["pointer" + kind]?.({}); },
    flush() { while (timers.length) timers.shift()(); },
    view: () => root.innerHTML,
    attrs,
    session: ctx.sessionStorage,
  };
}
const settle = () => new Promise((r) => setTimeout(r, 20));

const sent = [];
const click = async (t, title) => { sent.push(title); t.click({ Add: { title } }); await settle(); };
if (scenario === "two-tabs") {
  // A sends, then a second tab opens and sends, then A sends again.
  const a = tab(); await settle();
  await click(a, "a1");
  const b = tab(); await settle();
  await click(b, "b1");
  await click(a, "a2");
  await click(b, "b2");
} else if (scenario === "reopen") {
  // A tab closes with Msgs queued while offline; a new tab sends them.
  const a = tab(); await settle();
  server.down = true;
  await click(a, "a1");
  a.pagehide();
  server.down = false;
  const b = tab(); await settle();
  await click(b, "b1");
}
let extra = {};
if (scenario === "unauthorized" || scenario === "forbidden") {
  const a = tab(); await settle();
  server.status = scenario === "unauthorized" ? 401 : 403;
  await click(a, "a1");
  const keys = [];
  for (let i = 0; i < local.length; i++) keys.push(local.key(i));
  extra = { attrs: Object.keys(a.attrs).sort(), reloads: server.reloads, stored: keys.length, session: a.session.length };
}
if (scenario === "pointer") {
  // a sync reply lands while the pointer is down on the view: the view is
  // kept until the pointer is up and the click has run
  const a = tab(); await settle(); a.flush();
  const before = a.view();
  a.pointer("down");
  await click(a, "a1");
  const during = a.view();
  a.pointer("up");
  const afterUp = a.view();
  a.flush();
  extra = { before, during, afterUp, after: a.view() };
}
console.log(JSON.stringify({ sent, applied: server.applied.map((m) => m.Add.title), clients: server.acked.size, ...extra }));
