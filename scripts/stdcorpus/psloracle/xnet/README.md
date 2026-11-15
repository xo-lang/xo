golang.org/x/net v0.60.0's publicsuffix package (list.go, table.go and
its data files, unchanged, BSD license in LICENSE), so psloracle builds
with GOPROXY=off on a machine whose module cache lacks x/net (lead,
2026-11-15: the corpus check panicked on host 3). psloracle's go.mod
replaces golang.org/x/net with this directory.
