// Node twin of bench/perf/http/go: an in-memory users CRUD API on node:http
// (one process: the store is in memory, so a cluster of workers would each
// have its own), listening on loopback. Same routes, validation order,
// status codes and error messages as the Go server (handlers.go).
import * as http from "node:http";

const MAX_BODY_BYTES = 1 << 20;

interface User {
  id: number;
  name: string;
  email: string;
  nickname?: string;
}

let nextId = 1;
const users = new Map<number, User>();
const byEmail = new Map<string, number>();

function send(res: http.ServerResponse, status: number, body?: string): void {
  if (body === undefined) {
    res.writeHead(status);
    res.end();
    return;
  }
  res.writeHead(status, { "Content-Type": "application/json", "Content-Length": Buffer.byteLength(body) });
  res.end(body);
}

function fail(res: http.ServerResponse, status: number, msg: string): void {
  send(res, status, JSON.stringify({ error: msg }) + "\n");
}

function sendUser(res: http.ServerResponse, status: number, u: User): void {
  send(res, status, JSON.stringify(u) + "\n");
}

function validateName(name: string): string | null {
  let n = 0;
  for (const _ of name) n++;
  return n < 1 || n > 100 ? "name must be 1 to 100 characters" : null;
}

function validateEmail(email: string): string | null {
  return email.includes("@") ? null : "email must contain @";
}

type Fields = Record<string, unknown>;

class BadRequest extends Error {}

function readFields(body: Buffer): Fields {
  const text = body.toString("utf8").trim();
  if (text.length === 0 || text[0] !== "{") throw new BadRequest("body must be a JSON object");
  let v: unknown;
  try {
    v = JSON.parse(text);
  } catch {
    throw new BadRequest("invalid JSON body");
  }
  const fields = v as Fields;
  for (const k of Object.keys(fields)) {
    if (k !== "name" && k !== "email" && k !== "nickname") throw new BadRequest(`unknown field ${JSON.stringify(k)}`);
  }
  return fields;
}

function stringField(fields: Fields, key: string): string | undefined {
  if (!(key in fields)) return undefined;
  const v = fields[key];
  if (typeof v !== "string") throw new BadRequest(`${key} must be a string`);
  return v;
}

// absent: undefined; null: null; a string: the nickname
function nicknameField(fields: Fields): string | null | undefined {
  if (!("nickname" in fields)) return undefined;
  const v = fields["nickname"];
  if (v === null) return null;
  if (typeof v !== "string") throw new BadRequest("nickname must be a string or null");
  return v;
}

function parseId(s: string): number | null {
  if (!/^[+-]?\d+$/.test(s)) return null;
  const n = Number(s);
  return Number.isSafeInteger(n) && n >= 1 ? n : null;
}

function list(): string {
  const ids = [...users.keys()].sort((a, b) => a - b);
  return "[" + ids.map((id) => JSON.stringify(users.get(id))).join(",") + "]\n";
}

function create(res: http.ServerResponse, body: Buffer): void {
  const fields = readFields(body);
  const name = stringField(fields, "name");
  if (name === undefined) return fail(res, 400, "name is required");
  let e = validateName(name);
  if (e) return fail(res, 400, e);
  const email = stringField(fields, "email");
  if (email === undefined) return fail(res, 400, "email is required");
  e = validateEmail(email);
  if (e) return fail(res, 400, e);
  const nick = nicknameField(fields);
  if (byEmail.has(email)) return fail(res, 409, "email already exists");
  const u: User = { id: nextId++, name, email };
  if (typeof nick === "string") u.nickname = nick;
  users.set(u.id, u);
  byEmail.set(email, u.id);
  sendUser(res, 201, u);
}

function patch(res: http.ServerResponse, id: number, body: Buffer): void {
  const fields = readFields(body);
  const name = stringField(fields, "name");
  if (name !== undefined) {
    const e = validateName(name);
    if (e) return fail(res, 400, e);
  }
  const email = stringField(fields, "email");
  if (email !== undefined) {
    const e = validateEmail(email);
    if (e) return fail(res, 400, e);
  }
  const nick = nicknameField(fields);
  const u = users.get(id);
  if (!u) return fail(res, 404, "user not found");
  if (email !== undefined && email !== u.email) {
    if (byEmail.has(email)) return fail(res, 409, "email already exists");
    byEmail.delete(u.email);
    u.email = email;
    byEmail.set(email, id);
  }
  if (name !== undefined) u.name = name;
  if (nick === null) delete u.nickname;
  else if (nick !== undefined) u.nickname = nick;
  sendUser(res, 200, u);
}

function handle(req: http.IncomingMessage, res: http.ServerResponse, body: Buffer): void {
  const url = req.url ?? "/";
  const q = url.indexOf("?");
  const path = q < 0 ? url : url.slice(0, q);
  const m = req.method ?? "GET";
  if (path === "/users") {
    if (m === "POST") return create(res, body);
    if (m === "GET") return send(res, 200, list());
    return send(res, 405);
  }
  if (path.startsWith("/users/") && path.indexOf("/", 7) < 0) {
    if (m !== "GET" && m !== "PATCH" && m !== "DELETE") return send(res, 405);
    const id = parseId(path.slice(7));
    if (id === null) return fail(res, 404, "user not found");
    if (m === "PATCH") return patch(res, id, body);
    const u = users.get(id);
    if (!u) return fail(res, 404, "user not found");
    if (m === "GET") return sendUser(res, 200, u);
    users.delete(id);
    byEmail.delete(u.email);
    return send(res, 204);
  }
  send(res, 404, "404 page not found\n");
}

const server = http.createServer((req, res) => {
  const chunks: Buffer[] = [];
  let size = 0;
  req.on("data", (c: Buffer) => {
    size += c.length;
    if (size > MAX_BODY_BYTES) {
      fail(res, 400, "could not read body");
      req.destroy();
      return;
    }
    chunks.push(c);
  });
  req.on("end", () => {
    if (size > MAX_BODY_BYTES) return;
    try {
      handle(req, res, Buffer.concat(chunks));
    } catch (e) {
      if (e instanceof BadRequest) fail(res, 400, e.message);
      else fail(res, 500, "internal error");
    }
  });
});
server.keepAliveTimeout = 65000;
const port = Number(process.env.PORT ?? "8080");
server.listen(port, "127.0.0.1", () => console.error(`listening on 127.0.0.1:${port}`));
