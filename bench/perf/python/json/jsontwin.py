"""Python twin of bench/perf/jsontwin: json.loads and json.dumps of 1000
users, 100 rounds each. Usage: python jsontwin.py decode|encode."""
import json
import sys

DOCS = 1000
ROUNDS = 100


def users():
    us = []
    seed = 12345
    for _ in range(DOCS):
        seed = (seed * 1103515245 + 12345) % 2147483648
        n = seed % 100000
        us.append({"id": n, "name": f"user {n}", "email": f"user_{n}@example.com", "score": (seed % 1000) + 0.5, "active": seed % 2 == 0, "tags": [f"t{t}" for t in range(seed % 5)]})
    return us


def text(u):
    # built by hand, not with the encoder under test
    tags = ",".join(f'"{t}"' for t in u["tags"])
    active = "true" if u["active"] else "false"
    return f'{{"id":{u["id"]},"name":"{u["name"]}","email":"{u["email"]}","score":{u["score"]:.1f},"active":{active},"tags":[{tags}]}}'


def main():
    w = sys.argv[1] if len(sys.argv) > 1 else "decode"
    us = users()
    h = 0
    if w == "decode":
        ds = [text(u) for u in us]
        loads = json.loads
        for _ in range(ROUNDS):
            for d in ds:
                u = loads(d)
                h = (h * 31 + u["id"] + len(u["name"]) + len(u["email"]) + len(u["tags"]) + int(u["score"]) % 100 + (1 if u["active"] else 0)) % 1000000007
    elif w == "encode":
        enc = json.JSONEncoder(separators=(",", ":")).encode
        for _ in range(ROUNDS):
            for u in us:
                h = (h * 31 + len(enc(u).encode())) % 1000000007
    else:
        print("unknown workload", w, file=sys.stderr)
        sys.exit(2)
    print(w, h)


main()
