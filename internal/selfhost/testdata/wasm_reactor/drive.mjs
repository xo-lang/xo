// The host side of TestWasmReactorLib: node drive.mjs LIB.wasm instantiates the reactor (decision 0688), calls its exports and prints what they return.
import { readFileSync } from "node:fs";
import { WASI } from "node:wasi";
const wasi = new WASI({ version: "preview1", args: [], env: {} });
const mod = await WebAssembly.compile(readFileSync(process.argv[2]));
const inst = await WebAssembly.instantiate(mod, { wasi_snapshot_preview1: wasi.wasiImport });
wasi.initialize(inst);
const x = inst.exports;
const enc = new TextEncoder();
function put(s) { const b = enc.encode(s); const p = x.xo_alloc(b.length); new Uint8Array(x.memory.buffer, p, b.length).set(b); return [p, b.length]; }
console.log("add", x.add(3n, 4n));
console.log("greet_len", x.greet_len(...put("world")));
console.log("count", x.count(...put("héllo")));
const live0 = x.xo_live();
for (let i = 0; i < 1000; i++) x.greet_len(...put("w" + i));
console.log("live delta after 1000", x.xo_live() - live0);
const dec = new TextDecoder();
function get(p) { return dec.decode(new Uint8Array(x.memory.buffer, p, x.xo_result_len())); }
console.log("greet", get(x.greet(...put("wasm"))));
const l1 = x.xo_live();
for (let i = 0; i < 1000; i++) get(x.greet(...put("n" + i)));
console.log("greet live delta after 1000", x.xo_live() - l1);
function lasterr() { const p = x.xo_last_error(); return p ? dec.decode(new Uint8Array(x.memory.buffer, p, x.xo_last_error_len())) : null; }
console.log("parse ok", x.parse(...put("123")), x.xo_failed(), lasterr());
console.log("parse err", x.parse(...put("1x3")), x.xo_failed(), lasterr());
console.log("parse ok again", x.parse(...put("12")), x.xo_failed(), lasterr());
console.log("shout ok", get(x.shout(...put("hey"))), x.xo_failed());
x.shout(...put("")); console.log("shout err", x.xo_failed(), lasterr());
const l2 = x.xo_live();
for (let i = 0; i < 1000; i++) { x.parse(...put(i % 2 ? "x" + i : "" + i)); get(x.shout(...put(i % 3 ? "s" + i : ""))); }
console.log("error live delta after 1000", x.xo_live() - l2);
try { x.boom(7n); console.log("no trap"); } catch (e) { console.log("trapped:", e.constructor.name); }
