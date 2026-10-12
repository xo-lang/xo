// run.mjs: runs a module of the WebAssembly backend (decision 0179) under
// Node's WASI preview1, as the playground's worker will (decision 0144):
//
//   node --no-warnings selfhost/wasm/run.mjs PROG.wasm [args...]
//
// The module's only imports must be wasi_snapshot_preview1's. It runs in
// a worker with a 1 GiB stack (deep recursion of the goldens), its exit
// status is the program's, and a trap prints `wasm trap: <message>` with
// status 134. With XO_LEAKCHECK set, the allocations still live at exit
// are printed as the native runtime prints them (xo_live, xo_total).
import { readFileSync } from 'node:fs';
import { Worker, isMainThread, workerData, parentPort } from 'node:worker_threads';
import { WASI } from 'node:wasi';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

if (isMainThread) {
  const [file, ...args] = process.argv.slice(2);
  if (!file) {
    process.stderr.write('usage: run.mjs PROG.wasm [args...]\n');
    process.exit(2);
  }
  const w = new Worker(fileURLToPath(import.meta.url), {
    workerData: { file, args },
    resourceLimits: { stackSizeMb: 1024 },
    stdout: false,
    stderr: false,
  });
  w.on('message', (code) => { process.exitCode = code; });
  w.on('error', (e) => {
    process.stderr.write(`wasm trap: ${e && e.message}\n`);
    process.exitCode = 134;
  });
} else {
  const { file, args } = workerData;
  const bytes = readFileSync(file);
  const mod = new WebAssembly.Module(bytes);
  for (const imp of WebAssembly.Module.imports(mod)) {
    if (imp.module !== 'wasi_snapshot_preview1') {
      throw new Error(`import ${imp.module}.${imp.name} is not WASI preview1`);
    }
  }
  const wasi = new WASI({ version: 'preview1', args: [file, ...args], env: process.env, returnOnExit: true });
  const inst = new WebAssembly.Instance(mod, { wasi_snapshot_preview1: wasi.wasiImport });
  let code;
  try {
    code = wasi.start(inst);
  } catch (e) {
    process.stderr.write(`wasm trap: ${e && e.message}\n`);
    code = 134;
  }
  if (process.env.XO_LEAKCHECK && inst.exports.xo_live) {
    process.stderr.write(`xo leakcheck: ${inst.exports.xo_live()} live of ${inst.exports.xo_total()} allocations\n`);
  }
  parentPort.postMessage(code);
}
