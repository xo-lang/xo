"""Hidden black box checks for task 01 (CRUD API), run against a live server.

Only the behavior bench/tasks/01-crud-api.md states is checked; anything the
task leaves open (error message text, extra fields, whether an absent
nickname is omitted or null) is accepted. Standard library only.
"""
import json
import urllib.error
import urllib.request


def _call(base, method, path, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(base + path, data=data, method=method)
    if data is not None:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=10) as r:
            raw = r.read()
            status = r.status
    except urllib.error.HTTPError as e:
        raw = e.read()
        status = e.code
    try:
        js = json.loads(raw) if raw.strip() else None
    except ValueError:
        js = None
    return status, js


def _user_ok(u, uid, name, email, nickname=None):
    if not isinstance(u, dict):
        return False
    return (u.get("id") == uid and u.get("name") == name and u.get("email") == email
            and u.get("nickname") == nickname)


def checks(base):
    """Returns [(name, passed, detail)]. Each check builds its own users."""
    res = []

    def check(name, cond, detail=""):
        res.append((name, bool(cond), detail))

    s, u = _call(base, "POST", "/users", {"name": "Ada", "email": "ada@x.io"})
    check("create returns 201 with id 1", s == 201 and _user_ok(u, 1, "Ada", "ada@x.io"), "%s %s" % (s, u))
    s, g = _call(base, "GET", "/users/1")
    check("get returns the created user", s == 200 and _user_ok(g, 1, "Ada", "ada@x.io"), "%s %s" % (s, g))
    s, e = _call(base, "POST", "/users", {"name": "Other", "email": "ada@x.io"})
    check("duplicate email is 409", s == 409, str(s))
    s, e = _call(base, "POST", "/users", {"name": "Bad", "email": "no-at-sign"})
    check("invalid email is 400 with an error key", s == 400 and isinstance(e, dict) and "error" in e, "%s %s" % (s, e))
    s, e = _call(base, "POST", "/users", {"name": "", "email": "e@x.io"})
    check("empty name is 400", s == 400, str(s))
    s, e = _call(base, "POST", "/users", {"name": "n" * 101, "email": "e@x.io"})
    check("name over 100 characters is 400", s == 400, str(s))
    s, e = _call(base, "POST", "/users", {"name": "n" * 100, "email": "long@x.io"})
    check("name of 100 characters is accepted", s == 201, str(s))
    s, b = _call(base, "POST", "/users", {"name": "Bob", "email": "bob@x.io", "nickname": "bobby"})
    bid = b.get("id") if isinstance(b, dict) else None
    check("create with nickname assigns the next id", s == 201 and _user_ok(b, 3, "Bob", "bob@x.io", "bobby"), "%s %s" % (s, b))
    s, lst = _call(base, "GET", "/users")
    ids = [x.get("id") for x in lst] if isinstance(lst, list) else None
    check("list returns all users ordered by id", s == 200 and ids == [1, 2, 3], "%s %s" % (s, ids))
    s, p = _call(base, "PATCH", "/users/1", {"nickname": "countess"})
    check("patch with only nickname keeps other fields", s == 200 and _user_ok(p, 1, "Ada", "ada@x.io", "countess"), "%s %s" % (s, p))
    s, p = _call(base, "PATCH", "/users/1", {"email": "bad"})
    check("patch with an invalid email is 400", s == 400, str(s))
    if bid is not None:
        s, p = _call(base, "PATCH", "/users/%s" % bid, {"email": "ada@x.io"})
        check("patch to an existing email is 409", s == 409, str(s))
    s, p = _call(base, "PATCH", "/users/999", {"name": "Nobody"})
    check("patch of a missing user is 404", s == 404, str(s))
    s, _ = _call(base, "GET", "/users/999")
    check("get of a missing user is 404", s == 404, str(s))
    s, _ = _call(base, "DELETE", "/users/1")
    check("delete is 204", s == 204, str(s))
    s, _ = _call(base, "GET", "/users/1")
    check("get after delete is 404", s == 404, str(s))
    s, _ = _call(base, "DELETE", "/users/1")
    check("delete of a missing user is 404", s == 404, str(s))
    return res
