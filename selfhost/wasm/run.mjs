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
import v8 from 'node:v8';

// V8's concurrent garbage collector crashed (SIGSEGV in
// InnerPointerToCodeCache::GetCacheEntry) in about one run in three of
// programs that allocate much (knowledgebase/gotchas/
// node-wasm-concurrent-gc-crash.md); the single threaded collector does not.
v8.setFlagsFromString('--single-threaded-gc');
// A large module (the self hosted checker, 40 MB) crashed the worker
// (SIGSEGV) when its functions were compiled lazily, at their first call
// deep in the wasm stack; compiled at instantiation it runs
// (knowledgebase/gotchas/wasm-lazy-compile-worker-crash.md).
v8.setFlagsFromString('--no-wasm-lazy-compilation');
// With the single threaded collector, an incremental marking task of the
// main isolate while the worker ran for seconds failed V8's check
// `concurrent_marking_handle_ implies !concurrent_marking_handle_->IsValid()`
// (status 133; gotchas/wasm-busy-loop-crashes-node.md).
v8.setFlagsFromString('--no-incremental-marking');

if (isMainThread) {
  const [file, ...args] = process.argv.slice(2);
  if (!file) {
    process.stderr.write('usage: run.mjs PROG.wasm [args...]\n');
    process.exit(2);
  }
  const w = new Worker(fileURLToPath(import.meta.url), {
    workerData: { file, args },
    resourceLimits: { stackSizeMb: Number(process.env.XO_WASM_STACK_MB || 1024) },
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
  // the file system a native program sees: fd 3 the working directory
  // (relative paths), fd 4 the root (absolute paths; rt4.xo $wpath)
  const preopens = { '.': process.cwd(), '/': '/' };
  const wasi = new WASI({ version: 'preview1', args: [file, ...args], env: process.env, preopens, returnOnExit: true });
  // random_get from a seeded generator, so a run replays: the seed is
  // $XO_WASM_SEED, else a fresh one; it is printed (`xo wasm seed: N`,
  // with the number of random_get and clock_time_get calls) on a trap,
  // with XO_LEAKCHECK, and with XO_WASM_SEED_LOG
  // (gotchas/wasm-regexp-table-intermittent-trap.md)
  const seedEnv = process.env.XO_WASM_SEED;
  const seed = seedEnv ? BigInt.asUintN(64, BigInt(seedEnv)) : BigInt.asUintN(64, BigInt(Math.floor(Math.random() * 2 ** 32)) << 20n ^ BigInt(Date.now()));
  let state = seed;
  let randCalls = 0;
  let clockCalls = 0;
  const next = () => {
    // splitmix64
    state = BigInt.asUintN(64, state + 0x9e3779b97f4a7c15n);
    let z = state;
    z = BigInt.asUintN(64, (z ^ (z >> 30n)) * 0xbf58476d1ce4e5b9n);
    z = BigInt.asUintN(64, (z ^ (z >> 27n)) * 0x94d049bb133111ebn);
    return z ^ (z >> 31n);
  };
  let inst;
  const imports = Object.assign({}, wasi.wasiImport, {
    random_get(ptr, len) {
      randCalls++;
      const mem = new Uint8Array(inst.exports.memory.buffer, ptr >>> 0, len >>> 0);
      for (let i = 0; i < mem.length; i += 8) {
        let v = next();
        for (let j = i; j < i + 8 && j < mem.length; j++) {
          mem[j] = Number(v & 0xffn);
          v >>= 8n;
        }
      }
      return 0;
    },
    clock_time_get(...a) {
      clockCalls++;
      return wasi.wasiImport.clock_time_get(...a);
    },
  });
  inst = new WebAssembly.Instance(mod, { wasi_snapshot_preview1: imports });
  let code;
  let trapped = false;
  try {
    code = wasi.start(inst);
  } catch (e) {
    process.stderr.write(`wasm trap: ${e && e.message}\n`);
    // the wasm frames (function index and byte offset), indented so the
    // goldens' stderr comparison drops them
    const frames = String((e && e.stack) || '').split('\n').slice(1, 40);
    for (const f of frames) {
      process.stderr.write(`  ${f.trim()}\n`);
    }
    code = 134;
    trapped = true;
  }
  if (trapped || process.env.XO_LEAKCHECK || process.env.XO_WASM_SEED_LOG) {
    process.stderr.write(`xo wasm seed: ${seed} (random_get ${randCalls} calls, clock_time_get ${clockCalls} calls)\n`);
  }
  if (process.env.XO_LEAKCHECK && inst.exports.xo_live) {
    process.stderr.write(`xo leakcheck: ${inst.exports.xo_live()} live of ${inst.exports.xo_total()} allocations\n`);
  }
  parentPort.postMessage(code);
}
