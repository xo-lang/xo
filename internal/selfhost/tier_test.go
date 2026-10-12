package selfhost

import (
	"os"
	"strings"
	"testing"
)

// The test tiers of the package (knowledgebase/infrastructure/toolchain.md,
// the TestSelfhost lines):
//
//   - short (`go test -short`): the go backend (the lexer and parser
//     also arm64 and LLVM), small corpora, every 25th run for the SSA
//     builder and the LLVM emitter but the mergeHeavy ones (selfhost/llvm
//     fell in the sample and took 58 s and 4.7 GB in the SSA builder,
//     2026-10-12), no SSA mutations and no Xo test binaries;
//   - merge (the default without -short): the go backend at full size
//     (every run, every corpus and mutation count), except that the
//     comparisons of the SSA builder and the arm64 and LLVM emitters
//     leave the mergeHeavy runs (the self hosted compiler's components
//     and huge_frame) to the full tier: the SSA builder compares only
//     their build stage, the emitters skip them; the SSA mutation test
//     makes the first 300 of its 400 mutations, the emitters' 30; the
//     arm64 emitter takes every second run, the LLVM emitter every
//     third; the arm64 and LLVM lexers and parsers at -short's sizes are
//     the native smoke test, so no native build of the checker, the SSA
//     builder, or the emitters (the slow builds: the LLVM link of
//     selfhost/ssa takes minutes), and no end to end LLVM test;
//   - full (XO_SELFHOST_TIER=full): every backend, every corpus at full
//     size (nightly, release tagging, and changes the merge tier does
//     not cover).
//
// The counts of the XO_SELFHOST_*_FUZZ variables still override a tier,
// and XO_SELFHOST_BACKEND still names one backend. Every tier runs its
// work on the pools of jobs_test.go (XO_SELFHOST_JOBS,
// XO_SELFHOST_HEAVY_JOBS).
const (
	tierShort = "short"
	tierMerge = "merge"
	tierFull  = "full"
)

// selfhostTier is the tier of this run: short under -short, else
// $XO_SELFHOST_TIER (merge when unset); full when the run records the
// Go side (XO_SELFHOST_GOLDENS=record, gogoldens_test.go), so that one
// recording holds what every tier compares.
func selfhostTier(t testing.TB) string {
	t.Helper()
	if goldensRecording() {
		return tierFull
	}
	if testing.Short() {
		return tierShort
	}
	switch v := os.Getenv("XO_SELFHOST_TIER"); v {
	case "":
		return tierMerge
	case tierShort, tierMerge, tierFull:
		return v
	default:
		t.Fatalf("XO_SELFHOST_TIER=%q: want short, merge, or full", v)
		return ""
	}
}

// byTier picks the value of the current tier.
func byTier[T any](t testing.TB, short, merge, full T) T {
	t.Helper()
	switch selfhostTier(t) {
	case tierShort:
		return short
	case tierMerge:
		return merge
	}
	return full
}

// mergeHeavy reports whether the merge tier leaves run d (a directory
// of moduleDirs) to the full tier in the comparisons of the SSA builder
// (build stage only) and the arm64 and LLVM emitters (skipped):
// huge_frame (one 5,000 line function) and every self hosted component
// but the lexer and the parser. Each takes from tens of seconds to
// minutes in each comparison where the other runs take under a second
// (knowledgebase/gotchas/selfhost-ssa-passes-three-runs.md): with
// selfhost/arm64, selfhost/llvm, and selfhost/xo missing from the old
// fixed lists the merge tier took 1,329 s (2026-10-12). A new component
// under selfhost/ is heavy by this rule without an edit here.
func mergeHeavy(d string) bool {
	switch d {
	case "internal/arm64/testdata/run/huge_frame":
		return true
	case "selfhost/lexer", "selfhost/parser":
		return false
	}
	return strings.HasPrefix(d, "selfhost/")
}

// tierBackends are the backends a differential of the parser, checker,
// or SSA builder compares: go, except in the full tier.
func tierBackends(t testing.TB) []string {
	t.Helper()
	if selfhostTier(t) == tierFull {
		return []string{"go", "arm64", "llvm"}
	}
	return []string{"go"}
}
