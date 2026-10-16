#!/usr/bin/env python3
"""The bytes of an Xo darwin/arm64 executable by category and module
(bench/results/binary-size-native-2026-10-16.md).

    breakdown.py BINARY [--modules]

Code (__text) is attributed per symbol (the distance to the next symbol
of the section), each symbol to its module: _xo_f_MOD_2eFN is function
FN of module MOD (`__` in MOD is `_`); a symbol without `_2e`, and its
closures (FN_2efnN), is the program's own (its main module);
_xo_f_xo_2econst_5bMOD_2eNAME_5d builds MOD's constant table NAME; the
other _xo_f_xo_2e symbols (map, JSON, and drop list instances) and the
_xo_ symbols that are not functions (the emitters' helpers: drops,
shares, hashes, trampolines) are "generated". The other sections are reported whole. Modules group into
categories by name (CATEGORIES below)."""
import collections, re, subprocess, sys

CATEGORIES = [
    ("TLS and crypto", r"^(tls.*|x509.*|nistec.*|aesgcm|chacha|sha1|sha256|sha3|sha512|hmac|hkdf|rsa|mlkem|x25519|ed25519|ct|cpu|bigmod|digest.*|pem|asn1|cert.*|crypto|ghash|poly1305|curve.*|p256.*|fiat.*|ecdsa|ecdh|rand.*)$"),
    ("HTTP and net", r"^(routes|http.*|h2.*|hpack|hserve|hconn|hclient|url|dns|net.*|sock|cabi_net|cabi_http|httpwire.*|mime|router.*)$"),
    ("JSON, SQL, log, time", r"^(json.*|sql.*|log.*|time.*|tz.*|regexp.*|csv|toml)$"),
    ("prelude", r"^(prelude.*|llrt_prelude)$"),
    ("profiler and its line tables", r"^(prof|dwline|heapprof)$"),
    ("test world (FakeClock, FakeNet)", r"^(testworld|twrun)$"),
    ("other OSes (linux, windows on darwin)", r"^(linux|windows)$"),
    ("runtime", r".*"),
]

def main():
    binary = sys.argv[1]
    out = subprocess.run(["nm", "-nm", binary], capture_output=True, text=True, check=True).stdout
    syms = []
    for line in out.splitlines():
        m = re.match(r"^([0-9a-f]+) \((__\w+),(__\w+)\) \S+(?: \S+)? (\S+)$", line)
        if m:
            syms.append((int(m.group(1), 16), m.group(2) + "," + m.group(3), m.group(4)))
    sect = subprocess.run(["size", "-m", binary], capture_output=True, text=True, check=True).stdout
    sections = {}
    for line in sect.splitlines():
        m = re.match(r"^\s+Section (\S+): (\d+)", line)
        if m:
            sections[m.group(1)] = int(m.group(2))
    text = sorted(s for s in syms if s[1] == "__TEXT,__text")
    text_end = text[0][0] + sections.get("__text", 0) if text else 0
    # the program's own functions have no module prefix; their closures
    # are FN_2efnN, like a module's function, so they are told apart by FN
    progfns = {n[len("_xo_f_"):] for _, _, n in text if n.startswith("_xo_f_") and "_2e" not in n}
    mods = collections.Counter()
    for i, (a, _, name) in enumerate(text):
        size = (text[i + 1][0] if i + 1 < len(text) else text_end) - a
        if name.startswith("_xo_f_"):
            rest = name[len("_xo_f_"):]
            head = rest.split("_2e")[0]
            c = re.match(r"^xo_2econst_5b(\w+?)_2e", rest)
            if c:
                # xo.const[MOD.NAME]: a constant table's initializer, MOD's
                mod = c.group(1).replace("__", "_")
            elif "_2e" not in rest or head in progfns or head == "main":
                mod = "(the program)"
            elif head == "xo":
                # xo.map_..., xo.json_..., xo.droplist_...: instances
                # the compiler generates per type
                mod = "(generated)"
            else:
                mod = head.replace("__", "_")
        elif name.startswith("_xo_"):
            mod = "(generated)"
        else:
            mod = "(other)"
        mods[mod] += size
    cats = collections.Counter()
    catof = {}
    for mod, n in mods.items():
        if mod.startswith("("):
            catof[mod] = mod.strip("()")
        else:
            catof[mod] = next(cat for cat, pat in CATEGORIES if re.match(pat, mod))
        cats[catof[mod]] += n
    total = sum(sections.values())
    print(f"{'category':32} {'bytes':>10}")
    order = ["the program", "generated"] + [c for c, _ in CATEGORIES] + ["other"]
    for cat in order:
        print(f"{'code: ' + cat:32} {cats[cat]:>10}")
    for s in ["__const", "__cstring", "__eh_frame", "__gcc_except_tab", "__unwind_info", "__data"]:
        if s in sections:
            print(f"{s:32} {sections[s]:>10}")
    print(f"{'sections total':32} {total:>10}")
    if "--modules" in sys.argv:
        print()
        for mod, n in mods.most_common():
            print(f"{mod:32} {n:>10}  {catof[mod]}")

main()
