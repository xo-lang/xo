# density.py PID LABEL: per 2 MiB aligned region of a process's private
# writable anonymous mappings, how many 4 KiB pages are present
# (/proc/PID/pagemap bit 63; no root needed for presence). Under THP=always a
# touched region inside its mapping becomes one huge page, so the predicted
# RSS under always is touched regions * 2 MiB plus the present pages outside
# aligned regions. Prints that, a fill histogram, and smaps' Rss and
# AnonHugePages for the same mappings.
import sys, struct
pid, label = sys.argv[1], sys.argv[2]
P, H = 4096, 2 << 20
vmas = []
for line in open(f"/proc/{pid}/maps"):
    f = line.split()
    a, b = (int(x, 16) for x in f[0].split("-"))
    name = f[5] if len(f) > 5 else ""
    if f[1].startswith("rw") and f[1][3] == "p" and (name == "" or name == "[heap]" or name.startswith("[anon")):
        vmas.append((a, b))
rss = huge = 0
cur = None
vs = set(vmas)
for line in open(f"/proc/{pid}/smaps"):
    f = line.split()
    if "-" in f[0] and len(f) >= 5 and ":" not in f[0]:
        a, b = (int(x, 16) for x in f[0].split("-"))
        cur = (a, b) in vs
    elif cur and f[0] == "Rss:":
        rss += int(f[1])
    elif cur and f[0] == "AnonHugePages:":
        huge += int(f[1])
pm = open(f"/proc/{pid}/pagemap", "rb")
present = touched = outside = 0
groups = {}  # mapping size class -> [mappings, present pages, touched regions]
hist = [0, 0, 0, 0]  # 1-64, 65-256, 257-511, 512 pages of 512
for a, b in vmas:
    pm.seek(a // P * 8)
    raw = pm.read((b - a) // P * 8)
    bits = [(v >> 63) & 1 for v in struct.unpack(f"{len(raw)//8}Q", raw)]
    present += sum(bits)
    sz = b - a
    key = '4MiB' if sz == 4 << 20 else '<2MiB' if sz < H else '2-4MiB' if sz < 4 << 20 else '>4MiB'
    g = groups.setdefault(key, [0, 0, 0])
    g[0] += 1
    g[1] += sum(bits)
    lo, hi = -(-a // H) * H, b // H * H
    for i, v in enumerate(bits):
        addr = a + i * P
        if v and not (lo <= addr < hi):
            outside += 1
    r = lo
    while r < hi:
        s = (r - a) // P
        n = sum(bits[s:s + H // P])
        if n:
            touched += 1
            g[2] += 1
            hist[0 if n <= 64 else 1 if n <= 256 else 2 if n < 512 else 3] += 1
        r += H
st = {k: v for k, v in (l.split(":", 1) for l in open(f"/proc/{pid}/status"))}
print(f"{label}: RssAnon {st['RssAnon'].strip()}, anon mappings {len(vmas)}, present {present*4//1024} MB, smaps Rss {rss//1024} MB, AnonHugePages {huge//1024} MB")
print(f"{label}: touched 2MiB regions {touched} (fill 1-64:{hist[0]} 65-256:{hist[1]} 257-511:{hist[2]} full:{hist[3]}), outside pages {outside*4//1024} MB, predicted RSS under always {touched*2 + outside*4//1024} MB, mean fill {present*4/1024/max(touched*2,1)*100:.0f}%")
for k in sorted(groups):
    m, pr, t = groups[k]
    print(f"{label}:   {k}: mappings {m}, present {pr*4//1024} MB, touched regions {t}, fill {pr*4/1024/max(t*2,1)*100:.0f}%")
