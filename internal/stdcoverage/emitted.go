package stdcoverage

// A native prelude helper the compiler emits a call to has no call site in
// any Xo source: an intrinsic, a lowered construct (a channel's word call,
// a string's number text) or an index check reaches it through the
// lowering, not through a body. Until 2026-10-27 allow.txt covered them
// with `prelude.llrt_*`, one wildcard for the family, which also hid a
// helper nothing could emit any more (llrt_time_ok and llrt_time,
// removed the same day). Emitted reads the
// compiler's sources instead, by three strict rules:
//
//  1. a whole string literal in compiler/ssa that is the helper's name,
//     with or without `prelude.` (`"prelude.slice_fail"`, `"str_eq_lower"`),
//     or without its `llrt_` (b.llrt_call("chan_send_w", ...) emits
//     llrt_chan_send_w, and `name is "eprintln" | ...` passes the name on);
//     only compiler/ssa, where the SSA builders emit every prelude call: the
//     Go import "time" in the parser's habit hints once credited llrt_time,
//     which nothing emits;
//  2. a template passed straight to an emitting call, whose fixed part
//     names the family: prelude_impl("llrt_concat_${k}") and
//     llrt_call("chan_${fname}_w"); a template whose fixed part is only
//     `llrt_` or empty ("llrt_${name}") takes its name from rule 1 or 3;
//  3. a call of the intrinsic rt.X anywhere in std/, which the native
//     backends lower to the prelude's llrt_X (compiler/ssa build.xo llrt,
//     llrt_or_none).
//
// A literal must be the whole name, never a word in a message, so a helper
// is credited only when the compiler can spell it.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	nameLitRe  = regexp.MustCompile(`"(?:prelude\.)?([a-z_][a-z0-9_]*)"`)
	emitTmplRe = regexp.MustCompile(`(?:prelude_impl|llrt_call|llrt_or_none|llrt_or_rt|llrt|member\(nm,)\(\s*"(?:llrt_)?([a-z0-9_]*)\$\{[^}"]*\}([a-z0-9_]*)"`)
	rtCallRe   = regexp.MustCompile(`\brt\.([a-z_][a-z0-9_]*)\s*[(\[]`)
)

// Emitted is the set of native prelude helpers (bare names) the compiler
// can emit a call to, by the rules above.
func Emitted(root string) (map[string]bool, error) {
	lits := map[string]bool{}
	type tmpl struct{ pre, suf string }
	var tmpls []tmpl
	rts := map[string]bool{}
	walk := func(dir string, f func(src string)) error {
		// a fixture tree without the directory emits nothing from it
		if _, err := os.Stat(filepath.Join(root, dir)); os.IsNotExist(err) {
			return nil
		}
		return filepath.Walk(filepath.Join(root, dir), func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(p, ".xo") || strings.HasSuffix(p, "_test.xo") {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			f(string(data))
			return nil
		})
	}
	err := walk(filepath.Join("compiler", "ssa"), func(src string) {
		for _, m := range nameLitRe.FindAllStringSubmatch(src, -1) {
			lits[m[1]] = true
		}
		for _, m := range emitTmplRe.FindAllStringSubmatch(src, -1) {
			if m[1] != "" {
				tmpls = append(tmpls, tmpl{m[1], m[2]})
			}
		}
	})
	if err != nil {
		return nil, err
	}
	if err := walk("std", func(src string) {
		for _, m := range rtCallRe.FindAllStringSubmatch(src, -1) {
			rts[m[1]] = true
		}
	}); err != nil {
		return nil, err
	}
	ds, err := declared("prelude", filepath.Join(root, "std", "native", "prelude.xo"))
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, d := range ds {
		if d.Kind != "fn" {
			continue
		}
		n := d.bare
		x, isLlrt := strings.CutPrefix(n, "llrt_")
		switch {
		case lits[n], isLlrt && lits[x], isLlrt && rts[x]:
			out[n] = true
		case isLlrt:
			for _, t := range tmpls {
				mid, ok := strings.CutPrefix(x, t.pre)
				if ok && len(mid) > len(t.suf) && strings.HasSuffix(mid, t.suf) {
					out[n] = true
					break
				}
			}
		}
	}
	return out, nil
}
