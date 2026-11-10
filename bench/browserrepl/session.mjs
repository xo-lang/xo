// node bench/browserrepl/session.mjs compiler/wasm/run.mjs XOC STD bench/browserrepl/session.txt bench/browserrepl/want.txt [WORK]: drives `--play repl warm sess` (0583)
// with SESSION's entries (one a line, as repl30/session.txt), runs each built module with
// run.mjs, and compares each entry's printed value (or out) with WANT's lines (the terminal
// REPL's outputs for the same session, repl30/out.txt, one line per entry that prints).
// Then the commands: type, defs, undo, reset. Prints one line per entry and a summary:
// first entry ms (warm check included), median and max of the later ones, mismatches.
import { spawn, execFileSync } from "node:child_process";
import { createInterface } from "node:readline";
import { readFileSync, mkdirSync, writeFileSync, rmSync } from "node:fs";
import { join } from "node:path";
const [runmjs, xoc, std, sessFile, wantFile, work0] = process.argv.slice(2);
const work = work0 || "/tmp/xo-play-session";
rmSync(work, { recursive: true, force: true });
mkdirSync(join(work, "warm"), { recursive: true });
mkdirSync(join(work, "sess"), { recursive: true });
writeFileSync(join(work, "warm", "main.xo"), 'fn main() {\n  let xs = [1, 2, 3]\n  println("${xs.len()}")\n}\n');
writeFileSync(join(work, "warm", "xo.mod"), "module warm\n");
writeFileSync(join(work, "sess", "xo.mod"), "module sess\n");
const entries = readFileSync(sessFile, "utf8").split("\n").filter((l) => l.trim() !== "");
const want = readFileSync(wantFile, "utf8").split("\n").filter((l) => l !== "");
const p = spawn("node", ["--no-warnings", runmjs, xoc, `--std=${std}`, "--wd=.", "--play", "repl", "warm", "sess"], { cwd: work, stdio: ["pipe", "pipe", "inherit"] });
const rl = createInterface({ input: p.stdout });
const lines = []; let wake = null;
rl.on("line", (l) => { lines.push(l); if (wake) { wake(); wake = null; } });
const next = async () => { while (!lines.length) await new Promise((r) => (wake = r)); return JSON.parse(lines.shift()); };
const ask = async (req) => {
  const t = Date.now();
  p.stdin.write(JSON.stringify(req) + "\n");
  let r = await next();
  const built = Date.now() - t;
  if (r.run) {
    let out = "", code = 0;
    try { out = execFileSync("node", ["--no-warnings", runmjs, join(work, r.run)], { encoding: "utf8" }); } catch (e) { out = e.stdout || ""; code = e.status || 1; }
    p.stdin.write(JSON.stringify({ ran: out, code }) + "\n");
    r = await next();
  }
  return { r, built, total: Date.now() - t };
};
const times = []; let wi = 0; let bad = 0; let first = 0;
for (const [i, src] of entries.entries()) {
  const { r, built, total } = await ask({ in: src });
  if (i === 0) first = total; else times.push(total);
  const shown = r.value != null ? r.value : (r.out || "").trimEnd();
  let verdict = "";
  if (!r.ok) { verdict = "ERROR"; bad++; }
  else if (shown !== "") { verdict = shown === want[wi] ? "ok" : `MISMATCH want ${JSON.stringify(want[wi])}`; if (shown !== want[wi]) bad++; wi++; }
  console.log(`${String(i + 1).padStart(2)} ${src.slice(0, 40).padEnd(40)} build ${String(built).padStart(5)} total ${String(total).padStart(5)} ms ${JSON.stringify(shown).slice(0, 40)} ${verdict} ${r.ok ? "" : JSON.stringify((r.err || "").slice(0, 120))}`);
}
for (const req of [{ cmd: "type", arg: "p.sum()" }, { cmd: "type", arg: "m" }, { cmd: "defs" }, { cmd: "undo" }, { cmd: "type", arg: "nope" }, { cmd: "reset" }, { cmd: "type", arg: "p" }]) {
  const { r, total } = await ask(req);
  console.log(`cmd ${JSON.stringify(req).padEnd(34)} ${String(total).padStart(5)} ms ok=${r.ok} type=${JSON.stringify(r.type)} out=${JSON.stringify((r.out || "").slice(0, 80))} err=${JSON.stringify((r.err || "").slice(0, 100))}`);
}
p.stdin.write(JSON.stringify({ cmd: "quit" }) + "\n");
p.stdin.end();
times.sort((a, b) => a - b);
console.log(`summary: entries ${entries.length}, first ${first} ms, later median ${times[Math.floor(times.length / 2)]} ms, max ${times[times.length - 1]} ms, compared ${wi} of ${want.length} values, problems ${bad}`);
