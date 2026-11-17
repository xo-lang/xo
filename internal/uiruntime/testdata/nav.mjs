// Runs the whole runtime (RUNTIME_JS, path in argv[2]) on a server page
// (body data-xo-page, decision 0670) over a small fake DOM, history and
// fetch, and prints a JSON report of one scenario (argv[3]): what was
// fetched, what loaded in full, the URL, history and the page after it.
import { readFileSync } from "node:fs";
import vm from "node:vm";

const src = readFileSync(process.argv[2], "utf8");
const scenario = process.argv[3];
const ORIGIN = "http://test";
const STAMP = "s1";

class Node {
  constructor() { this.parent = null; this.childNodes = []; }
  get parentNode() { return this.parent; }
  get firstChild() { return this.childNodes[0] ?? null; }
  get nextSibling() {
    if (!this.parent) return null;
    const s = this.parent.childNodes;
    return s[s.indexOf(this) + 1] ?? null;
  }
  insertBefore(n, ref) {
    if (n.parent) n.parent.removeChild(n);
    const i = ref ? this.childNodes.indexOf(ref) : this.childNodes.length;
    this.childNodes.splice(i, 0, n);
    n.parent = this;
    return n;
  }
  appendChild(n) { return this.insertBefore(n, null); }
  removeChild(n) {
    this.childNodes.splice(this.childNodes.indexOf(n), 1);
    n.parent = null;
    return n;
  }
  get textContent() { return this.childNodes.map((c) => (c.nodeType === 3 ? c.nodeValue : c.textContent)).join(""); }
}
class Text extends Node {
  constructor(v) { super(); this.nodeType = 3; this.nodeValue = v; }
}
class El extends Node {
  constructor(tag) { super(); this.nodeType = 1; this.tagName = tag.toUpperCase(); this.attrs = new Map(); this.value = ""; this.checked = false; this.type = ""; this.disabled = false; this.style = {}; }
  get attributes() { return [...this.attrs].map(([name, value]) => ({ name, value })); }
  get id() { return this.attrs.get("id") ?? ""; }
  get name() { return this.attrs.get("name") ?? ""; }
  get href() { return new URL(this.getAttribute("href"), loc.href).href; }
  get target() { return this.getAttribute("target") ?? ""; }
  get form() { let p = this.parent; while (p && p.tagName !== "FORM") p = p.parent; return p ?? null; }
  getAttribute(k) { return this.attrs.has(k) ? this.attrs.get(k) : null; }
  setAttribute(k, v) {
    this.attrs.set(k, String(v));
    if (k === "type") this.type = String(v);
  }
  removeAttribute(k) { this.attrs.delete(k); }
  hasAttribute(k) { return this.attrs.has(k); }
  contains(n) { for (; n; n = n.parent) if (n === this) return true; return false; }
  closest(sel) {
    if (sel !== "a[href]") throw new Error("closest " + sel);
    for (let n = this; n; n = n.parent) if (n.tagName === "A" && n.hasAttribute("href")) return n;
    return null;
  }
  querySelectorAll(sel) {
    if (sel !== "button,input[type=submit]") throw new Error("querySelectorAll " + sel);
    return [...walk(this)].filter((n) => n.tagName === "BUTTON" || (n.tagName === "INPUT" && n.type === "submit"));
  }
  querySelector(sel) {
    const m = /^script\[src\*="([^"]+)"\]$/.exec(sel);
    if (!m) throw new Error("querySelector " + sel);
    return [...walk(this)].find((n) => n.tagName === "SCRIPT" && (n.getAttribute("src") ?? "").includes(m[1])) ?? null;
  }
  getElementById(id) { return [...walk(this)].find((n) => n.nodeType === 1 && n.id === id) ?? null; }
}
const VOID = new Set(["input", "br", "img", "meta"]);
// a parser for the well formed HTML of these scenarios
function parse(html) {
  const root = new El("fragment");
  let cur = root;
  const re = /<\/([a-z]+)>|<([a-z]+)((?:\s+[a-z-]+(?:="[^"]*")?)*)\s*>|([^<]+)/g;
  for (const m of html.matchAll(re)) {
    if (m[1]) { cur = cur.parent; continue; }
    if (m[4]) { cur.insertBefore(new Text(m[4]), null); continue; }
    const e = new El(m[2]);
    for (const a of m[3].matchAll(/([a-z-]+)(?:="([^"]*)")?/g)) e.setAttribute(a[1], a[2] ?? "");
    if (e.tagName === "INPUT") e.value = e.getAttribute("value") ?? "";
    cur.insertBefore(e, null);
    if (!VOID.has(m[2])) cur = e;
  }
  return root;
}
function* walk(n) { for (const c of n.childNodes) { yield c; yield* walk(c); } }
function doc(html) {
  const root = parse(html);
  const h = root.getElementById ? [...walk(root)].find((n) => n.tagName === "HTML") : null;
  const head = [...walk(h)].find((n) => n.tagName === "HEAD");
  const body = [...walk(h)].find((n) => n.tagName === "BODY");
  const t = [...walk(head)].find((n) => n.tagName === "TITLE");
  return { documentElement: h, head, body, title: t ? t.textContent : "", getElementById: (id) => h.getElementById(id), querySelector: (s) => h.querySelector(s) };
}
// a page that loads this runtime (stamp s1), another build of it, or none
function page(title, body, stamp = STAMP) {
  const s = stamp === null ? "" : `<script src="/_xo/ui.js?v=${stamp}"></script>`;
  return `<html><head><title>${title}</title>${s}</head><body data-xo-page>${body}</body></html>`;
}

// the server: path (with query) -> {status, html, redirect}
const routes = new Map();
const fetches = [];
const fullLoads = [];
const realSubmits = [];
let hold = null;
const loc = {
  url: new URL(ORIGIN + "/"),
  get href() { return this.url.href; },
  set href(v) { fullLoads.push(new URL(v, this.url).pathname + new URL(v, this.url).search); },
  get pathname() { return this.url.pathname; },
  get search() { return this.url.search; },
  get origin() { return this.url.origin; },
  get hash() { return this.url.hash; },
  protocol: "http:", host: "test",
  reload() { fullLoads.push("(reload)"); },
};
const entries = [];
let at = -1;
const hist = {
  scrollRestoration: "auto",
  get state() { return entries[at]?.state ?? null; },
  get length() { return entries.length; },
  pushState(s, _t, u) { entries.splice(at + 1); entries.push({ state: s, url: new URL(u, loc.url).href }); at++; loc.url = new URL(entries[at].url); },
  replaceState(s, _t, u) { entries[at] = { state: s, url: new URL(u, loc.url).href }; loc.url = new URL(entries[at].url); },
};
const win = { scrollY: 0, listeners: {}, addEventListener(n, f) { this.listeners[n] = f; }, scrollTo(_x, y) { this.scrollY = y; }, AbortController, WebSocket: class {} };
const docListeners = {};
let start = doc(page("A", "<p>start</p>"));
const document = {
  get head() { return start.head; },
  get body() { return start.body; },
  title: start.title,
  documentElement: start.documentElement,
  activeElement: null,
  hidden: false,
  getElementById: (id) => start.documentElement.getElementById(id),
  querySelector: (s) => start.documentElement.querySelector(s),
  createElement: (t) => new El(t),
  addEventListener(n, f) { (docListeners[n] ??= []).push(f); },
};
class FakeFormData {
  constructor(f, s) {
    this.list = [];
    for (const n of walk(f)) if (n.tagName === "INPUT" && n.name && n.type !== "submit") this.list.push([n.name, n.value]);
    if (s && s.name) this.list.push([s.name, s.getAttribute("value") ?? ""]);
  }
  get(k) { return this.list.find((e) => e[0] === k)?.[1] ?? null; }
  [Symbol.iterator]() { return this.list[Symbol.iterator](); }
}
class FakeForm {}
FakeForm.prototype.submit = function () { realSubmits.push(this.getAttribute("action")); };
const timers = new Map();
let tid = 0;
const ctx = {
  AbortController, document, window: win, location: loc, history: hist, URL, URLSearchParams, FormData: FakeFormData,
  HTMLFormElement: FakeForm, DOMParser: class { parseFromString(t) { return doc(t); } },
  fetch: (href, init) => {
    const u = new URL(href, loc.url);
    const key = u.pathname + u.search;
    fetches.push({ key, method: init.method ?? "GET", multipart: init.body instanceof FakeFormData, body: init.body && !(init.body instanceof FakeFormData) ? String(init.body) : "" });
    const answer = () => {
      const r = routes.get(key);
      if (!r) return Promise.reject(new Error("offline"));
      const to = r.redirect ? new URL(r.redirect, loc.url).href : u.href;
      const resp = { ok: r.status >= 200 && r.status < 300, status: r.status, url: to, redirected: !!r.redirect, headers: { get: () => "text/html; charset=utf-8" }, text: async () => r.html };
      return Promise.resolve(resp);
    };
    if (hold) return new Promise((res) => { hold.push(() => res(answer())); }).then((x) => x);
    return answer();
  },
  setTimeout: (f) => { timers.set(++tid, f); return tid; },
  clearTimeout: (i) => { timers.delete(i); },
  setInterval: () => 0, clearInterval: () => {},
  localStorage: { getItem: () => null, setItem() {}, removeItem() {}, length: 0, key: () => null },
  sessionStorage: { getItem: () => null, setItem() {}, removeItem() {}, length: 0, key: () => null },
  JSON, Math, Date, Error, Promise, Array, Object, String,
};
const settle = () => new Promise((r) => setTimeout(r, 20));
function flush() { const fs = [...timers.values()]; timers.clear(); for (const f of fs) f(); }
function boot(path, body) {
  loc.url = new URL(ORIGIN + path);
  entries.push({ state: null, url: loc.url.href }); at = 0;
  start = doc(page("A", body));
  document.documentElement = start.documentElement;
  document.title = start.title;
  vm.runInNewContext(src, ctx);
}
// an event the browser would act on unless the runtime takes it
function fire(name, ev) {
  ev.defaultPrevented = false;
  ev.preventDefault = function () { this.defaultPrevented = true; };
  for (const f of docListeners[name] ?? []) f(ev);
  return ev.defaultPrevented;
}
const byId = (id) => document.getElementById(id);
const click = (el, mods = {}) => fire("click", { target: el, button: 0, metaKey: false, ctrlKey: false, shiftKey: false, altKey: false, ...mods });
const submit = (f, s = null) => fire("submit", { target: f, submitter: s });
async function back() {
  at--; loc.url = new URL(entries[at].url);
  win.listeners.popstate?.({ state: entries[at].state });
  await settle();
}
const here = () => loc.url.pathname + loc.url.search;
const out = { fetches, fullLoads, realSubmits };

if (scenario === "link") {
  // a link to a page with the same runtime: fetched and patched in, the
  // header node kept, a new history entry, the new title
  routes.set("/b", { status: 200, html: page("B", '<header id="top">site</header><p id="body">page b</p>') });
  boot("/", '<header id="top">site</header><p id="body">page a</p><a id="go" href="/b">b</a>');
  const top = byId("top");
  out.taken = click(byId("go"));
  await settle();
  out.path = here(); out.title = document.title; out.text = byId("body").textContent;
  out.kept = byId("top") === top; out.history = hist.length;
}
if (scenario === "back") {
  // back restores the entry's scroll; a new page starts at the top
  routes.set("/", { status: 200, html: page("A", '<p id="body">page a</p><a id="go" href="/b">b</a>') });
  routes.set("/b", { status: 200, html: page("B", '<p id="body">page b</p>') });
  boot("/", '<p id="body">page a</p><a id="go" href="/b">b</a>');
  win.scrollY = 500;
  click(byId("go")); await settle();
  out.scrollB = win.scrollY;
  await back();
  out.path = here(); out.text = byId("body").textContent; out.scroll = win.scrollY;
  out.restoration = hist.scrollRestoration;
}
if (scenario === "getform") {
  // a GET form submits in place with its query; with
  // data-xo-submit-on-input it submits after typing, replacing the entry
  const form = (q) => `<form id="f" action="/search" data-xo-submit-on-input="100"><input id="q" name="q" value="${q}"><button id="b">go</button></form><p id="res">${q}</p>`;
  routes.set("/search?q=tea", { status: 200, html: page("S", form("tea")) });
  routes.set("/search?q=teapot", { status: 200, html: page("S", form("teapot")) });
  boot("/", form(""));
  byId("q").value = "tea";
  out.taken = submit(byId("f"), byId("b"));
  await settle();
  out.path = here(); out.text = byId("res").textContent;
  const n = hist.length;
  byId("q").value = "teapot";
  fire("input", { target: byId("q") });
  out.beforeTimer = fetches.length;
  flush(); await settle();
  out.typed = here(); out.typedText = byId("res").textContent; out.historyGrew = hist.length - n;
}
if (scenario === "postform") {
  // a POST answered 303 is the page it went to; answered 422 it is shown
  // in place with no new URL; buttons are disabled while it waits; a
  // multipart form sends FormData
  const form = (err, enc = "") => `<form id="f" method="post" action="/items"${enc}><input name="title" value="x"><button id="b">add</button></form><p id="msg">${err}</p>`;
  routes.set("/items/1", { status: 200, html: page("Item", '<p id="msg">item 1</p>') });
  // the 303: fetch follows it, so /items answers with /items/1
  routes.set("/items", { status: 200, redirect: "/items/1", html: page("Item", '<p id="msg">item 1</p>') });
  boot("/new", form(""));
  hold = [];
  out.taken = submit(byId("f"));
  out.disabledWhile = byId("b").disabled;
  out.busyWhile = document.documentElement.hasAttribute("data-xo-busy");
  hold.forEach((f) => f()); hold = null;
  await settle();
  out.path = here(); out.text = byId("msg").textContent; out.busyAfter = document.documentElement.hasAttribute("data-xo-busy");
  routes.set("/items", { status: 422, html: page("New", form("title taken")) });
  routes.set("/new", { status: 200, html: page("New", form("")) });
  await back();
  out.backPath = here();
  submit(byId("f")); await settle();
  out.path422 = here(); out.text422 = byId("msg").textContent;
  routes.set("/up", { status: 200, html: page("Up", '<p id="msg">uploaded</p>') });
  loc.url = new URL(ORIGIN + "/new");
  start.body.childNodes.splice(0);
  for (const c of [...parse(`<form id="m" method="post" action="/up" enctype="multipart/form-data"><input name="f" value="data"></form>`).childNodes]) start.body.appendChild(c);
  submit(byId("m")); await settle();
  out.multiText = byId("msg").textContent;
}
if (scenario === "frame") {
  // a link with data-xo-frame patches only that element: the rest of the
  // page keeps what it had even where the new page differs
  routes.set("/list?p=2", { status: 200, html: page("L", '<p id="side">other side</p><div id="list" data-xo-frame>page 2</div>') });
  boot("/list", '<p id="side">side</p><div id="list" data-xo-frame>page 1<a id="next" href="/list?p=2" data-xo-frame="list">next</a></div>');
  click(byId("next")); await settle();
  out.path = here(); out.text = byId("list").textContent; out.side = byId("side").textContent;
}
if (scenario === "optouts") {
  // each of these is left to the browser: no fetch, the event not taken
  routes.set("/b", { status: 200, html: page("B", "<p>b</p>") });
  routes.set("/other", { status: 200, html: page("O", "<p>o</p>", "s2") });
  routes.set("/plain", { status: 200, html: page("P", "<p>p</p>", null) });
  boot("/", '<a id="r" href="/b" data-xo-reload>r</a><a id="t" href="/b" target="_blank">t</a><a id="d" href="/b" download>d</a><a id="m" href="/b">m</a><a id="x" href="https://elsewhere.example/b">x</a><a id="s" href="/other">s</a><a id="p" href="/plain">p</a><a id="gone" href="/missing">g</a><form id="pf" method="post" action="/missing"><input name="a" value="1"></form>');
  out.taken = ["r", "t", "d", "x"].map((id) => click(byId(id)));
  out.taken.push(click(byId("m"), { metaKey: true }), click(byId("m"), { ctrlKey: true }), click(byId("m"), { shiftKey: true }));
  out.fetchedOptOuts = fetches.length;
  // a page with another build of the runtime, one with none, or a failed
  // fetch: loaded in full; a POST that fails is submitted for real
  click(byId("s")); await settle();
  click(byId("p")); await settle();
  click(byId("gone")); await settle();
  submit(byId("pf")); await settle();
}
console.log(JSON.stringify(out));
