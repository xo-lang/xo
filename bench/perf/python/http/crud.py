"""The users CRUD API of bench/perf/http/go, in Python: the store and the
request logic, shared by the two servers (server_asyncio.py: asyncio.Protocol
on the standard library; server_starlette.py: Starlette on uvicorn). Same
routes, validation order, status codes and error messages as the Go server
(handlers.go). handle() takes the method, the path and the body and returns
(status, body bytes or None)."""
import json

_dumps = json.JSONEncoder(ensure_ascii=False, separators=(",", ":")).encode


class _Bad(Exception):
    pass


users: dict[int, dict] = {}
by_email: dict[str, int] = {}
next_id = 1


def _fail(status: int, msg: str):
    return status, (_dumps({"error": msg}) + "\n").encode()


def _user(status: int, u: dict):
    return status, (_dumps(u) + "\n").encode()


def _read_fields(body: bytes) -> dict:
    text = body.decode("utf-8", "replace").strip()
    if not text or text[0] != "{":
        raise _Bad("body must be a JSON object")
    try:
        fields = json.loads(text)
    except ValueError:
        raise _Bad("invalid JSON body")
    for k in fields:
        if k not in ("name", "email", "nickname"):
            raise _Bad("unknown field " + json.dumps(k))
    return fields


def _string(fields: dict, key: str):
    if key not in fields:
        return None
    v = fields[key]
    if not isinstance(v, str):
        raise _Bad(key + " must be a string")
    return v


_ABSENT = object()


def _nickname(fields: dict):
    if "nickname" not in fields:
        return _ABSENT
    v = fields["nickname"]
    if v is None:
        return None
    if not isinstance(v, str):
        raise _Bad("nickname must be a string or null")
    return v


def _parse_id(s: str):
    t = s[1:] if s[:1] in "+-" else s
    if not t.isascii() or not t.isdigit():
        return None
    n = int(s)
    return n if n >= 1 else None


def _name_error(name: str):
    return None if 1 <= len(name) <= 100 else "name must be 1 to 100 characters"


def _email_error(email: str):
    return None if "@" in email else "email must contain @"


def _create(body: bytes):
    global next_id
    fields = _read_fields(body)
    name = _string(fields, "name")
    if name is None:
        return _fail(400, "name is required")
    e = _name_error(name)
    if e:
        return _fail(400, e)
    email = _string(fields, "email")
    if email is None:
        return _fail(400, "email is required")
    e = _email_error(email)
    if e:
        return _fail(400, e)
    nick = _nickname(fields)
    if email in by_email:
        return _fail(409, "email already exists")
    u = {"id": next_id, "name": name, "email": email}
    if isinstance(nick, str):
        u["nickname"] = nick
    next_id += 1
    users[u["id"]] = u
    by_email[email] = u["id"]
    return _user(201, u)


def _patch(uid: int, body: bytes):
    fields = _read_fields(body)
    name = _string(fields, "name")
    if name is not None:
        e = _name_error(name)
        if e:
            return _fail(400, e)
    email = _string(fields, "email")
    if email is not None:
        e = _email_error(email)
        if e:
            return _fail(400, e)
    nick = _nickname(fields)
    u = users.get(uid)
    if u is None:
        return _fail(404, "user not found")
    if email is not None and email != u["email"]:
        if email in by_email:
            return _fail(409, "email already exists")
        del by_email[u["email"]]
        u["email"] = email
        by_email[email] = uid
    if name is not None:
        u["name"] = name
    if nick is None:
        u.pop("nickname", None)
    elif nick is not _ABSENT:
        u["nickname"] = nick
    return _user(200, u)


def handle(method: str, target: str, body: bytes):
    q = target.find("?")
    path = target if q < 0 else target[:q]
    try:
        if path == "/users":
            if method == "POST":
                return _create(body)
            if method == "GET":
                return 200, ("[" + ",".join(_dumps(users[i]) for i in sorted(users)) + "]\n").encode()
            return 405, None
        if path.startswith("/users/") and "/" not in path[7:]:
            if method not in ("GET", "PATCH", "DELETE"):
                return 405, None
            uid = _parse_id(path[7:])
            if uid is None:
                return _fail(404, "user not found")
            if method == "PATCH":
                return _patch(uid, body)
            u = users.get(uid)
            if u is None:
                return _fail(404, "user not found")
            if method == "GET":
                return _user(200, u)
            del users[uid]
            del by_email[u["email"]]
            return 204, None
    except _Bad as e:
        return _fail(400, str(e))
    return 404, b"404 page not found\n"
