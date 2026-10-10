// Starts `xo lsp` for .xo files. The binary is the first of: the `xo.path`
// setting; .tools/bin/xo in a workspace folder, the active file's
// directory, or any directory above them; .tools/bin/xo of the checkout
// this extension lives in (editors/vscode is symlinked into
// ~/.vscode/extensions); `xo` on PATH.
const fs = require("fs");
const path = require("path");
const vscode = require("vscode");
const { LanguageClient } = require("vscode-languageclient/node");

let client;

function isFile(p) {
  try {
    return fs.statSync(p).isFile();
  } catch {
    return false;
  }
}

// upward returns .tools/bin/xo in dir or the nearest directory above it.
function upward(dir) {
  for (let d = dir; d; ) {
    const c = path.join(d, ".tools", "bin", "xo");
    if (isFile(c)) {
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
  return { command: p, tried: starts.map((s) => s + " (and above)/.tools/bin/xo").concat(["xo on PATH"]) };
}

async function start(context) {
  if (!vscode.workspace.getConfiguration("xo").get("lsp.enabled")) {
    return;
  }
  const { command, tried } = xoPath(context);
  if (!command) {
    vscode.window.showWarningMessage(
      "Xo: no xo binary found for the language server. Build one with `go build -o .tools/bin/xo ./cmd/xo` in the Xo checkout, or set xo.path. Looked in: " +
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
      synchronize: {
        fileEvents: vscode.workspace.createFileSystemWatcher("**/{*.xo,xo.mod,xo.work}"),
      },
    }
  );
  try {
    await client.start();
  } catch (err) {
    vscode.window.showWarningMessage(
      `Xo: could not start "${command} lsp" (${(err && err.message) || err}). Set xo.path or rebuild .tools/bin/xo.`
    );
  }
}

async function stop() {
  if (client) {
    const c = client;
    client = undefined;
    await c.stop();
  }
}

function activate(context) {
  context.subscriptions.push(
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
