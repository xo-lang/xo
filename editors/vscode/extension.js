// Starts `xo lsp` for .xo files. The binary is the first of: the `xo.path`
// setting; in a workspace folder, the active file's directory, or any
// directory above them, bin/xo of an Xo checkout (what `go run
// ./scripts/xobuild` builds) or .tools/bin/xo; the same in the checkout this
// extension lives in (editors/vscode is symlinked into ~/.vscode/extensions);
// `xo` on PATH.
const cp = require("child_process");
const fs = require("fs");
const path = require("path");
const vscode = require("vscode");
const { LanguageClient, RevealOutputChannelOn } = require("vscode-languageclient/node");

let client;
// output is the one "Xo" output channel; each LanguageClient would
// otherwise open its own, one more per restart.
let output;

function isFile(p) {
  try {
    return fs.statSync(p).isFile();
  } catch {
    return false;
  }
}

// candidates returns the binaries dir may hold, best first. bin/xo counts
// only in an Xo checkout (it has selfhost/xo/main.xo), where it is the
// current compiler and .tools/bin/xo may be an old one.
function candidates(d) {
  const out = [];
  if (isFile(path.join(d, "selfhost", "xo", "main.xo"))) {
    out.push(path.join(d, "bin", "xo"));
  }
  out.push(path.join(d, ".tools", "bin", "xo"));
  return out;
}

// upward returns the first binary of candidates in dir or the nearest
// directory above it that has one.
function upward(dir) {
  for (let d = dir; d; ) {
    const c = candidates(d).find(isFile);
    if (c) {
      return c;
    }
    const parent = path.dirname(d);
    if (parent === d) {
      break;
    }
    d = parent;
  }
  return undefined;
}

function onPath(name) {
  for (const dir of (process.env.PATH || "").split(path.delimiter)) {
    const c = path.join(dir, name);
    if (dir && isFile(c)) {
      return c;
    }
  }
  return undefined;
}

// xoPath returns the binary and every place it looked.
function xoPath(context) {
  const configured = vscode.workspace.getConfiguration("xo").get("path");
  if (configured) {
    return { command: configured, tried: [configured] };
  }
  const starts = (vscode.workspace.workspaceFolders || []).map((f) => f.uri.fsPath);
  const doc = vscode.window.activeTextEditor && vscode.window.activeTextEditor.document;
  if (doc && doc.uri.scheme === "file") {
    starts.push(path.dirname(doc.uri.fsPath));
  }
  try {
    starts.push(path.resolve(fs.realpathSync(context.extensionPath), "..", ".."));
  } catch {}
  for (const s of starts) {
    const c = upward(s);
    if (c) {
      return { command: c, tried: starts };
    }
  }
  const p = onPath("xo");
  return { command: p, tried: starts.map((s) => s + " (and above)/{bin/xo of a checkout,.tools/bin/xo}").concat(["xo on PATH"]) };
}

// version returns the first line of `xo version`, so the output channel
// shows which compiler answers (an old one rejects `edition preview`).
function version(command) {
  try {
    return cp.execFileSync(command, ["version"], { timeout: 5000 }).toString().split("\n")[0].trim();
  } catch {
    return "version unknown";
  }
}

async function start(context) {
  if (!vscode.workspace.getConfiguration("xo").get("lsp.enabled")) {
    return;
  }
  const { command, tried } = xoPath(context);
  if (!command) {
    vscode.window.showWarningMessage(
      "Xo: no xo binary found for the language server. Build one with `go run ./scripts/xobuild` in the Xo checkout (it writes bin/xo), or set xo.path. Looked in: " +
        tried.join(", ")
    );
    return;
  }
  client = new LanguageClient(
    "xo",
    "Xo",
    { command, args: ["lsp"] },
    {
      documentSelector: [{ scheme: "file", language: "xo" }],
      // A failed request (hover over a package whose xo.mod does not load)
      // goes to the output channel, not a popup per mouse move.
      revealOutputChannelOn: RevealOutputChannelOn.Never,
      outputChannel: output,
      synchronize: {
        fileEvents: vscode.workspace.createFileSystemWatcher("**/{*.xo,xo.mod,xo.work}"),
      },
    }
  );
  try {
    await client.start();
    client.outputChannel.appendLine(`xo lsp: ${command} (${version(command)})`);
  } catch (err) {
    vscode.window.showWarningMessage(
      `Xo: could not start "${command} lsp" (${(err && err.message) || err}). Set xo.path or rebuild with \`go run ./scripts/xobuild\`.`
    );
  }
}

async function stop() {
  if (client) {
    const c = client;
    client = undefined;
    await c.dispose();
  }
}

function activate(context) {
  output = vscode.window.createOutputChannel("Xo");
  context.subscriptions.push(
    output,
    vscode.commands.registerCommand("xo.restartServer", async () => {
      await stop();
      await start(context);
    }),
    vscode.workspace.onDidChangeConfiguration(async (e) => {
      if (e.affectsConfiguration("xo")) {
        await stop();
        await start(context);
      }
    })
  );
  return start(context);
}

function deactivate() {
  return stop();
}

module.exports = { activate, deactivate };
