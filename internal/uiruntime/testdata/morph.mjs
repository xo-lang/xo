// Runs the runtime's morph (the code between "// morph-begin" and
// "// morph-end" in RUNTIME_JS, path in argv[2]) over a small fake DOM and
// prints a JSON report: which nodes survived a patch, and what they hold.
import { readFileSync } from "node:fs";
import vm from "node:vm";

const all = readFileSync(process.argv[2], "utf8");
const src = all.slice(all.indexOf("// morph-begin"), all.indexOf("// morph-end"));

class Node {
  constructor() { this.parent = null; this.childNodes = []; }
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
  constructor(tag) { super(); this.nodeType = 1; this.tagName = tag.toUpperCase(); this.attrs = new Map(); this.value = ""; this.checked = false; this.type = ""; }
  get attributes() { return [...this.attrs].map(([name, value]) => ({ name, value })); }
  get id() { return this.attrs.get("id") ?? ""; }
  getAttribute(k) { return this.attrs.has(k) ? this.attrs.get(k) : null; }
  setAttribute(k, v) {
    this.attrs.set(k, String(v));
    if (k === "type") this.type = String(v);
  }
  removeAttribute(k) { this.attrs.delete(k); }
  hasAttribute(k) { return this.attrs.has(k); }
}
const VOID = new Set(["input", "br", "img"]);
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
    if (e.tagName === "INPUT") { e.value = e.getAttribute("value") ?? ""; e.checked = e.hasAttribute("checked"); }
    if (e.tagName === "TEXTAREA") e.value = "";
    cur.insertBefore(e, null);
    if (!VOID.has(m[2])) cur = e;
  }
  for (const e of walk(root)) if (e.tagName === "TEXTAREA") e.value = e.textContent;
  return root;
}
function* walk(n) { for (const c of n.childNodes) { yield c; yield* walk(c); } }
function html(n) {
  return n.childNodes.map((c) => {
    if (c.nodeType === 3) return c.nodeValue;
    const at = c.attributes.map((a) => ` ${a.name}="${a.value}"`).join("");
    const tag = c.tagName.toLowerCase();
    return VOID.has(tag) ? `<${tag}${at}>` : `<${tag}${at}>${html(c)}</${tag}>`;
  }).join("");
}
const ctx = {};
vm.runInNewContext(src + ";this.xoMorph=xoMorph;", ctx);
const morph = (root, next, focus = null) => ctx.xoMorph(root, parse(next), focus);
const find = (root, sel) => [...walk(root)].find(sel);

const out = {};
{
  // a keyed list reordered: each item is the same node, in the new order
  const root = parse('<ul><li data-key="a">A</li><li data-key="b">B</li><li data-key="c">C</li></ul>');
  const before = Object.fromEntries(["a", "b", "c"].map((k) => [k, find(root, (n) => n.getAttribute?.("data-key") === k)]));
  morph(root, '<ul><li data-key="c">C</li><li data-key="a">A!</li><li data-key="d">D</li></ul>');
  out.keyedHTML = html(root);
  out.keyedKept = ["a", "c"].every((k) => find(root, (n) => n.getAttribute?.("data-key") === k) === before[k]);
  out.keyedRemoved = !find(root, (n) => n.getAttribute?.("data-key") === "b");
}
{
  // the focused input keeps what the user typed; another input takes the view's value
  const root = parse('<form><input name="title" value=""><input name="other" value="x"><p>0 items</p></form>');
  const title = find(root, (n) => n.getAttribute?.("name") === "title");
  const other = find(root, (n) => n.getAttribute?.("name") === "other");
  title.value = "typing";
  other.value = "stale";
  morph(root, '<form><input name="title" value=""><input name="other" value="y"><p>1 items</p></form>', title);
  out.focusValue = title.value;
  out.focusKept = find(root, (n) => n.getAttribute?.("name") === "title") === title;
  out.otherValue = other.value;
  out.text = find(root, (n) => n.tagName === "P").textContent;
}
{
  // an open <details> the user opened stays open; the node (and its scroll) is kept
  const root = parse('<details><summary>More</summary><div class="box">x</div></details>');
  const d = find(root, (n) => n.tagName === "DETAILS");
  const box = find(root, (n) => n.tagName === "DIV");
  d.setAttribute("open", "");
  box.scrollTop = 120;
  morph(root, '<details><summary>More</summary><div class="box">y</div></details>');
  out.detailsOpen = d.hasAttribute("open");
  out.scrollKept = find(root, (n) => n.tagName === "DIV") === box && box.scrollTop === 120;
  out.boxText = box.textContent;
}
{
  // a checkbox takes the view's state; a changed tag is replaced
  const root = parse('<div><input type="checkbox" name="done"><span>a</span></div>');
  const cb = find(root, (n) => n.tagName === "INPUT");
  morph(root, '<div><input type="checkbox" name="done" checked><b>a</b></div>');
  out.checked = cb.checked;
  out.replacedHTML = html(root);
}
console.log(JSON.stringify(out));
