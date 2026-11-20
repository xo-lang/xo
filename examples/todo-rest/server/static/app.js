// todo-rest's client (task #432): renders the server's State, sends each
// action to the JSON API and polls /api/ver every 2 s for other people's
// changes. It does not predict: the page changes when the answer comes.
"use strict";
let ver = 0;
let pending = 0;
const $ = (id) => document.getElementById(id);

function status(text, error) {
  const s = $("status");
  s.textContent = text;
  s.className = error ? "status error" : "status";
}

function render(st) {
  ver = st.ver;
  const ul = $("todos");
  ul.replaceChildren(...st.todos.map((t) => {
    const li = document.createElement("li");
    li.className = t.done ? "done" : "";
    const check = document.createElement("button");
    check.className = "check";
    check.setAttribute("aria-label", "toggle");
    check.textContent = t.done ? "✓" : " ";
    check.onclick = () => send("POST", `/api/todos/${t.id}/toggle`);
    const span = document.createElement("span");
    span.textContent = t.title;
    const rm = document.createElement("button");
    rm.className = "remove";
    rm.setAttribute("aria-label", `remove ${t.title}`);
    rm.textContent = "Remove";
    rm.onclick = () => send("DELETE", `/api/todos/${t.id}`);
    li.append(check, span, rm);
    return li;
  }));
  $("seats").textContent = `Seats: ${st.seats} left`;
  $("holders").textContent = st.holders.length === 0 ? "No reservations yet." : `Reserved: ${st.holders.join(", ")}`;
}

async function send(method, path, body) {
  pending++;
  status("Saving…");
  try {
    const r = await fetch(path, {
      method,
      headers: body ? { "Content-Type": "application/json" } : {},
      body: body ? JSON.stringify(body) : undefined,
    });
    const v = await r.json();
    if (r.status === 409) {
      render(v);
      status("No seats left.", true);
      return;
    }
    if (!r.ok) {
      status(v.error || `error ${r.status}`, true);
      return;
    }
    render(v);
    status("");
  } catch (e) {
    status("Offline: the change was not sent.", true);
  } finally {
    pending--;
  }
}

async function refresh() {
  const r = await fetch("/api/state");
  render(await r.json());
}

async function poll() {
  if (pending === 0) {
    try {
      const r = await fetch("/api/ver");
      const v = await r.json();
      if (v.ver !== ver) {
        await refresh();
      }
    } catch (e) {
      status("Offline.", true);
    }
  }
  setTimeout(poll, 2000);
}

function form(id, field, path) {
  $(id).onsubmit = (ev) => {
    ev.preventDefault();
    const input = ev.target.elements[field];
    const v = input.value;
    input.value = "";
    send("POST", path, { [field]: v });
  };
}

form("add", "title", "/api/todos");
form("reserve", "name", "/api/seats");
refresh().then(poll);
