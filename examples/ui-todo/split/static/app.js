// The ui-todo front end, written by hand: it renders the server's Model,
// posts each Msg to /api/msg as JSON, and redraws on every change the
// server sends on /api/events. A Msg shows once the server answers.
"use strict";
let ver = -1;
let pending = 0;
const $ = (id) => document.getElementById(id);

function el(tag, attrs, kids) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (k.startsWith("on")) e.addEventListener(k.slice(2), v);
    else e.setAttribute(k, v);
  }
  for (const k of kids || []) e.append(k);
  return e;
}

function render(snap) {
  if (snap.ver <= ver) return;
  ver = snap.ver;
  const m = snap.model;
  $("todos").replaceChildren(...m.todos.map((t) =>
    el("li", { class: t.done ? "done" : "" }, [
      el("button", { class: "check", "aria-label": "toggle", onclick: () => send({ Toggle: { id: t.id } }) }, [t.done ? "✓" : " "]),
      el("span", {}, [t.title]),
      el("button", { class: "remove", "aria-label": "remove " + t.title, onclick: () => send({ Remove: { id: t.id } }) }, ["Remove"]),
    ])));
  $("seats").textContent = "Seats: " + m.seats + " left";
  $("holders").textContent = m.holders.length ? "Reserved: " + m.holders.join(", ") : "No reservations yet.";
}

async function send(msg) {
  pending++;
  $("status").classList.add("pending");
  try {
    const r = await fetch("/api/msg", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(msg) });
    if (r.ok) render(await r.json());
  } finally {
    if (--pending === 0) $("status").classList.remove("pending");
  }
}

function form(id, field, make) {
  $(id).addEventListener("submit", (ev) => {
    ev.preventDefault();
    const input = ev.target.elements[field];
    send(make(input.value));
    input.value = "";
  });
}

form("add", "title", (title) => ({ Add: { title } }));
form("reserve", "name", (name) => ({ Reserve: { name } }));
const events = new EventSource("/api/events");
events.onmessage = (ev) => { $("status").classList.remove("offline"); render(JSON.parse(ev.data)); };
events.onerror = () => $("status").classList.add("offline");
