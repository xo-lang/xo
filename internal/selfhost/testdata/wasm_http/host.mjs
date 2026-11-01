// The host side of internal/selfhost TestWasmComponentHTTP (decision 0447): runs a wasm
// backend build of a wasi:http component's core module under Node with stub wasi:http 0.2.0
// imports written from the canonical ABI, calls its handle export once per request, and prints
// what each request got back (status, sorted fields, body, the handles it set and dropped).
//   node host.mjs CORE.wasm fresh|reuse REQUEST...   (a REQUEST is a path, or "POST path")
import { readFileSync } from "node:fs";
import { WASI } from "node:wasi";

const file = process.argv[2];
const fresh = process.argv[3] === "fresh";
const paths = process.argv.slice(4);
const mod = new WebAssembly.Module(readFileSync(file));
console.log("exports:", WebAssembly.Module.exports(mod).map((e) => e.name).filter((n) => !n.startsWith("xo_")).sort().join(" "));

let inst, mem;
const u8 = () => new Uint8Array(mem.buffer);
const dv = () => new DataView(mem.buffer);
const td = new TextDecoder();
const str = (p, n) => td.decode(u8().slice(p, p + n));
function put(s) {
  const b = new TextEncoder().encode(s);
  const p = inst.exports.cabi_realloc(0, 0, 1, b.length);
  u8().set(b, p);
  return [p, b.length];
}

let path, method, bodyLeft, res;
const H = { req: 1, out: 2, fields: 10, resp: 20, body: 30, stream: 40, reqFields: 50 };
const reqHeaders = [["host", "localhost"], ["user-agent", "w47-node"], ["x-test", "a"], ["x-test", "b"], ["x-empty", ""]];
const http = {
  "[method]incoming-request.method": (r, ret) => { dv().setUint8(ret, method === "POST" ? 2 : 0) }, // get, post
  "[method]incoming-request.consume": (r, ret) => { dv().setUint8(ret, 0); dv().setUint32(ret + 4, 60, true) },
  "[method]incoming-body.stream": (b, ret) => { dv().setUint8(ret, 0); dv().setUint32(ret + 4, 70, true) },
  "[resource-drop]incoming-body": (b) => { res.drops.push("incoming-body") },
  "[method]incoming-request.path-with-query": (r, ret) => {
    const [p, n] = put(path);
    dv().setUint8(ret, 1); dv().setUint32(ret + 4, p, true); dv().setUint32(ret + 8, n, true);
  },
  "[resource-drop]incoming-request": (r) => { res.drops.push("incoming-request") },
  "[method]incoming-request.headers": (r) => H.reqFields,
  "[method]fields.entries": (f, ret) => {
    // list<tuple<string, list<u8>>>: 16 byte elements, each string its own block
    const n = reqHeaders.length;
    const lp = n ? inst.exports.cabi_realloc(0, 0, 4, n * 16) : 0;
    reqHeaders.forEach(([k, v], i) => {
      const [kp, kn] = put(k), [vp, vn] = put(v);
      const e = lp + i * 16;
      dv().setUint32(e, kp, true); dv().setUint32(e + 4, kn, true); dv().setUint32(e + 8, vp, true); dv().setUint32(e + 12, vn, true);
    });
    dv().setUint32(ret, lp, true); dv().setUint32(ret + 4, n, true);
  },
  "[resource-drop]fields": (f) => { res.drops.push("fields") },
  "[constructor]fields": () => H.fields,
  "[method]fields.append": (f, np, nl, vp, vl, ret) => {
    res.headers.push([str(np, nl), str(vp, vl)]); dv().setUint8(ret, 0);
  },
  "[constructor]outgoing-response": (f) => { res.respFields = f; return H.resp },
  "[method]outgoing-response.set-status-code": (r, code) => { res.status = code; return 0 },
  "[method]outgoing-response.body": (r, ret) => { dv().setUint8(ret, 0); dv().setUint32(ret + 4, H.body, true) },
  "[method]outgoing-body.write": (b, ret) => { dv().setUint8(ret, 0); dv().setUint32(ret + 4, H.stream, true) },
  "[static]outgoing-body.finish": (b, tag, tr, ret) => { res.finished = b === H.body && tag === 0; dv().setUint8(ret, 0) },
  "[static]response-outparam.set": (o, tag, r, ...rest) => { res.outparam = { o, tag, r } },
};
const streams = {
  "[method]output-stream.blocking-write-and-flush": (s, p, n, ret) => { res.body += str(p, n); dv().setUint8(ret, 0) },
  "[resource-drop]output-stream": (s) => { res.drops.push("output-stream") },
  "[method]input-stream.blocking-read": (s, max, ret) => {
    // the body in pieces of at most 6 bytes (and max), then closed
    if (bodyLeft.length === 0) { dv().setUint8(ret, 1); dv().setUint8(ret + 4, 1); return }
    const piece = bodyLeft.slice(0, Math.min(6, Number(max))); bodyLeft = bodyLeft.slice(piece.length);
    const [p, n] = put(piece);
    dv().setUint8(ret, 0); dv().setUint32(ret + 4, p, true); dv().setUint32(ret + 8, n, true);
  },
  "[resource-drop]input-stream": (s) => { res.drops.push("input-stream") },
};
// fresh: a new instance per request, as wasmtime serve runs a component by default
let wasi, handle;
function start() {
  wasi = new WASI({ version: "preview1", args: ["xo"], env: {}, returnOnExit: true });
  inst = new WebAssembly.Instance(mod, {
    wasi_snapshot_preview1: wasi.wasiImport,
    "wasi:http/types@0.2.0": http,
    "wasi:io/streams@0.2.0": streams,
    "wasi:io/error@0.2.0": { "[resource-drop]error": (e) => {} },
  });
  mem = inst.exports.memory;
  wasi.initialize({ exports: { memory: mem } });
  handle = inst.exports["wasi:http/incoming-handler@0.2.0#handle"];
}
start();
console.log(fresh ? "mode: an instance per request" : "mode: one instance for every request");
let bad = 0, first = true;
for (const p of paths) {
  if (fresh && !first) start();
  first = false;
  // "POST /path" sends a body
  [method, path] = p.startsWith("POST ") ? ["POST", p.slice(5)] : ["GET", p];
  bodyLeft = method === "POST" ? "hello request body" : "";
  res = { headers: [], body: "", drops: [] };
  handle(H.req, H.out);
  const ok = res.outparam && res.outparam.o === H.out && res.outparam.tag === 0 && res.outparam.r === H.resp && res.finished;
  if (!ok) bad++;
  console.log(`${p}: ${res.status} ${JSON.stringify(res.body)} ${JSON.stringify(res.headers.sort())} drops ${res.drops.sort().join(",")} ${ok ? "ok" : "BAD"}`);
}
console.log(bad === 0 ? "NODE-HOST OK" : `NODE-HOST ${bad} BAD`);
process.exit(bad === 0 ? 0 : 1);
